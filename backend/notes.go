package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const localUserID = "local"

func currentActorTx(ctx context.Context, tx *sql.Tx) (string, string) {
	var id, username string
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM sync_state WHERE key='user_id'),'local'),
		COALESCE((SELECT value FROM sync_state WHERE key='username'),'本地用户')`).Scan(&id, &username)
	if id == "" {
		id = localUserID
	}
	if username == "" {
		username = "本地用户"
	}
	return id, username
}

func (s *Store) currentActor(ctx context.Context) (string, string) {
	var id, username string
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM sync_state WHERE key='user_id'),'local'),
		COALESCE((SELECT value FROM sync_state WHERE key='username'),'本地用户')`).Scan(&id, &username)
	if id == "" {
		id = localUserID
	}
	if username == "" {
		username = "本地用户"
	}
	return id, username
}

func normalizeNoteTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "无标题笔记"
	}
	if len([]rune(title)) > 240 {
		return "", errors.New("笔记标题不能超过 240 个字符")
	}
	return title, nil
}

func normalizeEntityName(name, label string, limit int) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%s名称不能为空", label)
	}
	if len([]rune(name)) > limit {
		return "", fmt.Errorf("%s名称不能超过 %d 个字符", label, limit)
	}
	return name, nil
}

func (s *Store) defaultNotebookID(ctx context.Context) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM notebooks WHERE is_default = 1 AND deleted_at IS NULL LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("默认笔记本不存在")
	}
	return id, err
}

func scanNotebook(scanner rowScanner) (Notebook, error) {
	var item Notebook
	var isDefault int
	err := scanner.Scan(&item.ID, &item.Name, &item.Kind, &item.Role, &item.OwnerID, &isDefault,
		&item.DeletedAt, &item.CreatedAt, &item.UpdatedAt, &item.ServerVersion)
	item.Default = isDefault == 1
	return item, err
}

const notebookColumns = `id, name, kind, role, owner_id, is_default, COALESCE(deleted_at, ''), created_at, updated_at, server_version`

func (s *Store) ListNotebooks(ctx context.Context) ([]Notebook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+notebookColumns+` FROM notebooks WHERE deleted_at IS NULL ORDER BY is_default DESC, kind, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Notebook{}
	for rows.Next() {
		item, err := scanNotebook(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) getNotebook(ctx context.Context, id string) (Notebook, error) {
	item, err := scanNotebook(s.db.QueryRowContext(ctx, `SELECT `+notebookColumns+` FROM notebooks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Notebook{}, errors.New("笔记本不存在")
	}
	return item, err
}

func (s *Store) CreateNotebook(ctx context.Context, name string) (Notebook, error) {
	name, err := normalizeEntityName(name, "笔记本", 60)
	if err != nil {
		return Notebook{}, err
	}
	now := nowString()
	item := Notebook{ID: uuid.NewString(), Name: name, Kind: "personal", Role: "owner", OwnerID: localUserID, CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Notebook{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO notebooks(id, name, kind, role, owner_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Name, item.Kind, item.Role, item.OwnerID, now, now); err != nil {
		return Notebook{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO memberships(notebook_id, user_id, username, role, created_at) VALUES (?, ?, '本地用户', 'owner', ?)`, item.ID, localUserID, now); err != nil {
		return Notebook{}, err
	}
	if err := enqueueOutboxTx(ctx, tx, "notebook", item.ID, "upsert", 0, item); err != nil {
		return Notebook{}, err
	}
	if err := tx.Commit(); err != nil {
		return Notebook{}, err
	}
	return item, nil
}

func (s *Store) UpdateNotebook(ctx context.Context, input NotebookInput) (Notebook, error) {
	name, err := normalizeEntityName(input.Name, "笔记本", 60)
	if err != nil {
		return Notebook{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE notebooks SET name = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL AND role IN ('owner','editor')`, name, nowString(), input.ID)
	if err != nil {
		return Notebook{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Notebook{}, errors.New("笔记本不存在或没有编辑权限")
	}
	item, err := s.getNotebook(ctx, input.ID)
	if err == nil {
		_ = s.enqueueOutbox(ctx, "notebook", item.ID, "upsert", item.ServerVersion, item)
	}
	return item, err
}

func (s *Store) DeleteNotebook(ctx context.Context, id string) error {
	item, err := s.getNotebook(ctx, id)
	if err != nil {
		return err
	}
	if item.Default {
		return errors.New("默认笔记本不能删除")
	}
	if item.Role != "owner" {
		return errors.New("只有所有者可以删除笔记本")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := nowString()
	if item.Kind == "shared" {
		if _, err := tx.ExecContext(ctx, `UPDATE notes SET deleted_at = ?, updated_at = ? WHERE notebook_id = ? AND deleted_at IS NULL`, now, now, id); err != nil {
			return err
		}
	} else {
		var defaultID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM notebooks WHERE is_default = 1 AND deleted_at IS NULL`).Scan(&defaultID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE notes SET notebook_id = ?, updated_at = ?, revision = revision + 1 WHERE notebook_id = ? AND deleted_at IS NULL`, defaultID, now, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notebooks SET deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, id); err != nil {
		return err
	}
	if err := addTombstoneTx(ctx, tx, "notebook", id, now); err != nil {
		return err
	}
	item.DeletedAt, item.UpdatedAt = now, now
	if err := enqueueOutboxTx(ctx, tx, "notebook", id, "delete", item.ServerVersion, item); err != nil {
		return err
	}
	return tx.Commit()
}

func scanNoteTag(scanner rowScanner) (NoteTag, error) {
	var item NoteTag
	err := scanner.Scan(&item.ID, &item.NotebookID, &item.Name, &item.Color, &item.DeletedAt, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

const noteTagColumns = `id, notebook_id, name, color, COALESCE(deleted_at, ''), created_at, updated_at`

func (s *Store) ListNoteTags(ctx context.Context, notebookID string) ([]NoteTag, error) {
	query := `SELECT ` + noteTagColumns + ` FROM note_tags WHERE deleted_at IS NULL`
	args := []any{}
	if notebookID != "" {
		query += ` AND notebook_id = ?`
		args = append(args, notebookID)
	}
	query += ` ORDER BY name COLLATE NOCASE`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []NoteTag{}
	for rows.Next() {
		item, err := scanNoteTag(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateNoteTag(ctx context.Context, input NoteTagInput) (NoteTag, error) {
	name, err := normalizeEntityName(input.Name, "笔记标签", 28)
	if err != nil {
		return NoteTag{}, err
	}
	if _, err := s.editableNotebook(ctx, input.NotebookID); err != nil {
		return NoteTag{}, err
	}
	now := nowString()
	item := NoteTag{ID: uuid.NewString(), NotebookID: input.NotebookID, Name: name, Color: normalizeTagColor(input.Color), CreatedAt: now, UpdatedAt: now}
	_, err = s.db.ExecContext(ctx, `INSERT INTO note_tags(id, notebook_id, name, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, item.ID, item.NotebookID, item.Name, item.Color, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return NoteTag{}, errors.New("这个笔记本中已存在同名标签")
		}
		return NoteTag{}, err
	}
	_ = s.enqueueOutbox(ctx, "note_tag", item.ID, "upsert", 0, item)
	return item, nil
}

func (s *Store) UpdateNoteTag(ctx context.Context, input NoteTagInput) (NoteTag, error) {
	name, err := normalizeEntityName(input.Name, "笔记标签", 28)
	if err != nil {
		return NoteTag{}, err
	}
	var notebookID string
	if err := s.db.QueryRowContext(ctx, `SELECT notebook_id FROM note_tags WHERE id = ? AND deleted_at IS NULL`, input.ID).Scan(&notebookID); err != nil {
		return NoteTag{}, errors.New("笔记标签不存在")
	}
	if _, err := s.editableNotebook(ctx, notebookID); err != nil {
		return NoteTag{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE note_tags SET name = ?, color = ?, updated_at = ? WHERE id = ?`, name, normalizeTagColor(input.Color), nowString(), input.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return NoteTag{}, errors.New("这个笔记本中已存在同名标签")
		}
		return NoteTag{}, err
	}
	item, err := scanNoteTag(s.db.QueryRowContext(ctx, `SELECT `+noteTagColumns+` FROM note_tags WHERE id = ?`, input.ID))
	if err == nil {
		_ = s.enqueueOutbox(ctx, "note_tag", item.ID, "upsert", 0, item)
	}
	return item, err
}

func (s *Store) DeleteNoteTag(ctx context.Context, id string) error {
	item, scanErr := scanNoteTag(s.db.QueryRowContext(ctx, `SELECT `+noteTagColumns+` FROM note_tags WHERE id = ? AND deleted_at IS NULL`, id))
	if scanErr != nil {
		return errors.New("笔记标签不存在")
	}
	now := nowString()
	result, err := s.db.ExecContext(ctx, `UPDATE note_tags SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("笔记标签不存在")
	}
	item.DeletedAt, item.UpdatedAt = now, now
	_ = s.enqueueOutbox(ctx, "note_tag", id, "delete", 0, item)
	return nil
}

func (s *Store) editableNotebook(ctx context.Context, notebookID string) (Notebook, error) {
	item, err := s.getNotebook(ctx, notebookID)
	if err != nil || item.DeletedAt != "" {
		return Notebook{}, errors.New("笔记本不存在")
	}
	if item.Role == "viewer" {
		return Notebook{}, errors.New("当前笔记本为只读")
	}
	return item, nil
}

func plainExcerpt(content string) string {
	text := strings.NewReplacer("#", "", "*", "", "_", "", "`", "", ">", "", "[", "", "]", "", "(", "", ")", "").Replace(content)
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 150 {
		return string(runes[:150]) + "..."
	}
	return text
}

func (s *Store) ListNotes(ctx context.Context, query NoteQuery) ([]NoteSummary, error) {
	clauses := []string{"n.deleted_at IS NULL", "nb.deleted_at IS NULL"}
	args := []any{}
	if query.NotebookID != "" {
		clauses = append(clauses, "n.notebook_id = ?")
		args = append(args, query.NotebookID)
	}
	if query.TagID != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM note_tag_links ntl WHERE ntl.note_id = n.id AND ntl.tag_id = ?)`)
		args = append(args, query.TagID)
	}
	if query.TaskID != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM note_task_links ntk WHERE ntk.note_id = n.id AND ntk.task_id = ? AND ntk.user_id = ?)`)
		args = append(args, query.TaskID, localUserID)
	}
	if query.PinnedOnly {
		clauses = append(clauses, "n.pinned = 1")
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		value := "%" + search + "%"
		clauses = append(clauses, `(LOWER(n.title) LIKE LOWER(?) OR LOWER(n.content) LIKE LOWER(?))`)
		args = append(args, value, value)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT n.id, n.notebook_id, n.title, n.content, n.pinned, n.revision, n.created_at, n.updated_at,
		(SELECT COUNT(*) FROM note_attachments na JOIN attachments a ON a.id = na.attachment_id WHERE na.note_id = n.id AND a.deleted_at IS NULL),
		(SELECT COUNT(*) FROM note_task_links ntk JOIN tasks t ON t.id = ntk.task_id WHERE ntk.note_id = n.id AND ntk.user_id = ? AND t.deleted_at IS NULL),
		(SELECT COUNT(*) FROM comments c WHERE c.note_id = n.id AND c.deleted_at IS NULL)
		FROM notes n JOIN notebooks nb ON nb.id = n.notebook_id WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY n.pinned DESC, n.updated_at DESC`, append([]any{localUserID}, args...)...)
	if err != nil {
		return nil, err
	}
	items := []NoteSummary{}
	for rows.Next() {
		var item NoteSummary
		var content string
		var pinned int
		if err := rows.Scan(&item.ID, &item.NotebookID, &item.Title, &content, &pinned, &item.Revision, &item.CreatedAt, &item.UpdatedAt,
			&item.AttachmentCount, &item.LinkedTaskCount, &item.CommentCount); err != nil {
			return nil, err
		}
		item.Pinned = pinned == 1
		item.Excerpt = plainExcerpt(content)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range items {
		items[index].Tags, err = s.tagsForNote(ctx, items[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func scanNote(scanner rowScanner) (Note, error) {
	var item Note
	var pinned int
	err := scanner.Scan(&item.ID, &item.NotebookID, &item.Title, &item.Content, &pinned, &item.Revision,
		&item.DeletedAt, &item.CreatedAt, &item.UpdatedAt, &item.ServerVersion)
	item.Pinned = pinned == 1
	item.Tags = []NoteTag{}
	item.Tasks = []TaskReference{}
	item.Attachments = []Attachment{}
	return item, err
}

const noteColumns = `id, notebook_id, title, content, pinned, revision, COALESCE(deleted_at, ''), created_at, updated_at, server_version`

func (s *Store) GetNote(ctx context.Context, id string) (Note, error) {
	item, err := scanNote(s.db.QueryRowContext(ctx, `SELECT `+noteColumns+` FROM notes WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, errors.New("笔记不存在")
	}
	if err != nil {
		return Note{}, err
	}
	item.Tags, err = s.tagsForNote(ctx, id)
	if err != nil {
		return Note{}, err
	}
	item.Tasks, err = s.tasksForNote(ctx, id)
	if err != nil {
		return Note{}, err
	}
	item.Attachments, err = s.attachmentsForNote(ctx, id)
	return item, err
}

func (s *Store) CreateNote(ctx context.Context, input CreateNoteInput) (Note, error) {
	if input.NotebookID == "" {
		var err error
		input.NotebookID, err = s.defaultNotebookID(ctx)
		if err != nil {
			return Note{}, err
		}
	}
	if _, err := s.editableNotebook(ctx, input.NotebookID); err != nil {
		return Note{}, err
	}
	title, err := normalizeNoteTitle(input.Title)
	if err != nil {
		return Note{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, err
	}
	defer tx.Rollback()
	id, now := uuid.NewString(), nowString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO notes(id, notebook_id, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, id, input.NotebookID, title, input.Content, now, now); err != nil {
		return Note{}, err
	}
	if err := replaceNoteTagsTx(ctx, tx, id, input.NotebookID, input.TagIDs); err != nil {
		return Note{}, err
	}
	if err := replaceNoteTasksTx(ctx, tx, id, input.TaskIDs); err != nil {
		return Note{}, err
	}
	if err := saveNoteVersionTx(ctx, tx, id, title, input.Content, 1, true); err != nil {
		return Note{}, err
	}
	payload, err := noteSyncPayloadTx(ctx, tx, id)
	if err != nil {
		return Note{}, err
	}
	if err := enqueueOutboxTx(ctx, tx, "note", id, "upsert", 0, payload); err != nil {
		return Note{}, err
	}
	if err := enqueueLatestNoteVersionTx(ctx, tx, id); err != nil {
		return Note{}, err
	}
	if err := tx.Commit(); err != nil {
		return Note{}, err
	}
	return s.GetNote(ctx, id)
}

func (s *Store) UpdateNote(ctx context.Context, input UpdateNoteInput) (Note, error) {
	if input.ID == "" {
		return Note{}, errors.New("笔记 ID 不能为空")
	}
	title, err := normalizeNoteTitle(input.Title)
	if err != nil {
		return Note{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, err
	}
	defer tx.Rollback()
	var currentRevision int64
	var currentNotebook string
	var serverVersion int64
	if err := tx.QueryRowContext(ctx, `SELECT revision, notebook_id, server_version FROM notes WHERE id = ? AND deleted_at IS NULL`, input.ID).Scan(&currentRevision, &currentNotebook, &serverVersion); err != nil {
		return Note{}, errors.New("笔记不存在")
	}
	if input.BaseRevision > 0 && input.BaseRevision != currentRevision {
		return Note{}, fmt.Errorf("笔记已在其他位置更新，请重新加载: 当前版本 %d", currentRevision)
	}
	if input.NotebookID == "" {
		input.NotebookID = currentNotebook
	}
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM notebooks WHERE id = ? AND deleted_at IS NULL`, input.NotebookID).Scan(&role); err != nil || role == "viewer" {
		return Note{}, errors.New("目标笔记本不存在或为只读")
	}
	now := nowString()
	nextRevision := currentRevision + 1
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET notebook_id = ?, title = ?, content = ?, revision = ?, updated_at = ? WHERE id = ?`,
		input.NotebookID, title, input.Content, nextRevision, now, input.ID); err != nil {
		return Note{}, err
	}
	if err := replaceNoteTagsTx(ctx, tx, input.ID, input.NotebookID, input.TagIDs); err != nil {
		return Note{}, err
	}
	if err := replaceNoteTasksTx(ctx, tx, input.ID, input.TaskIDs); err != nil {
		return Note{}, err
	}
	if err := saveNoteVersionTx(ctx, tx, input.ID, title, input.Content, nextRevision, false); err != nil {
		return Note{}, err
	}
	payload, err := noteSyncPayloadTx(ctx, tx, input.ID)
	if err != nil {
		return Note{}, err
	}
	if err := enqueueOutboxTx(ctx, tx, "note", input.ID, "upsert", serverVersion, payload); err != nil {
		return Note{}, err
	}
	if err := enqueueLatestNoteVersionTx(ctx, tx, input.ID); err != nil {
		return Note{}, err
	}
	if err := tx.Commit(); err != nil {
		return Note{}, err
	}
	return s.GetNote(ctx, input.ID)
}

func (s *Store) SetNotePinned(ctx context.Context, id string, pinned bool) (Note, error) {
	value := 0
	if pinned {
		value = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE notes SET pinned = ?, updated_at = ?, revision = revision + 1 WHERE id = ? AND deleted_at IS NULL`, value, nowString(), id)
	if err != nil {
		return Note{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Note{}, errors.New("笔记不存在")
	}
	item, err := s.GetNote(ctx, id)
	if err == nil {
		_ = s.enqueueOutbox(ctx, "note", id, "pin", item.ServerVersion, backupNoteFromNote(item))
	}
	return item, err
}

func (s *Store) DeleteNote(ctx context.Context, id string) error {
	item, err := s.GetNote(ctx, id)
	if err != nil {
		return err
	}
	now := nowString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE notes SET deleted_at = ?, updated_at = ?, revision = revision + 1 WHERE id = ? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("笔记不存在")
	}
	if err := addTombstoneTx(ctx, tx, "note", id, now); err != nil {
		return err
	}
	payload := backupNoteFromNote(item)
	payload.UpdatedAt = now
	if err := enqueueOutboxTx(ctx, tx, "note", id, "delete", item.ServerVersion, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RestoreNote(ctx context.Context, id string) (Note, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE notes SET deleted_at = NULL, updated_at = ?, revision = revision + 1 WHERE id = ? AND deleted_at IS NOT NULL`, nowString(), id)
	if err != nil {
		return Note{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Note{}, errors.New("笔记无法恢复")
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM tombstones WHERE entity_type = 'note' AND entity_id = ?`, id)
	item, err := s.GetNote(ctx, id)
	if err == nil {
		_ = s.enqueueOutbox(ctx, "note", id, "restore", item.ServerVersion, backupNoteFromNote(item))
	}
	return item, err
}

func (s *Store) tagsForNote(ctx context.Context, noteID string) ([]NoteTag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.notebook_id, t.name, t.color, COALESCE(t.deleted_at, ''), t.created_at, t.updated_at FROM note_tags t
		JOIN note_tag_links l ON l.tag_id = t.id WHERE l.note_id = ? AND t.deleted_at IS NULL ORDER BY t.name COLLATE NOCASE`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []NoteTag{}
	for rows.Next() {
		item, err := scanNoteTag(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) tasksForNote(ctx context.Context, noteID string) ([]TaskReference, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.title, COALESCE(t.due_date, ''), COALESCE(t.completed_at, '')
		FROM tasks t JOIN note_task_links l ON l.task_id = t.id
		WHERE l.note_id = ? AND l.user_id = ? AND t.deleted_at IS NULL ORDER BY t.completed_at IS NOT NULL, t.created_at DESC`, noteID, localUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TaskReference{}
	for rows.Next() {
		var item TaskReference
		if err := rows.Scan(&item.ID, &item.Title, &item.DueDate, &item.CompletedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func replaceNoteTagsTx(ctx context.Context, tx *sql.Tx, noteID, notebookID string, tagIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_tag_links WHERE note_id = ?`, noteID); err != nil {
		return err
	}
	for _, id := range uniqueStrings(tagIDs) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM note_tags WHERE id = ? AND notebook_id = ? AND deleted_at IS NULL`, id, notebookID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return errors.New("笔记标签不存在或不属于目标笔记本")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_tag_links(note_id, tag_id) VALUES (?, ?)`, noteID, id); err != nil {
			return err
		}
	}
	return nil
}

func replaceNoteTasksTx(ctx context.Context, tx *sql.Tx, noteID string, taskIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_task_links WHERE note_id = ? AND user_id = ?`, noteID, localUserID); err != nil {
		return err
	}
	for _, id := range uniqueStrings(taskIDs) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE id = ? AND deleted_at IS NULL`, id).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return errors.New("关联任务不存在")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_task_links(note_id, task_id, user_id) VALUES (?, ?, ?)`, noteID, id, localUserID); err != nil {
			return err
		}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func backupNoteFromNote(item Note) BackupNote {
	tagIDs := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tagIDs = append(tagIDs, tag.ID)
	}
	attachmentIDs := make([]string, 0, len(item.Attachments))
	for _, attachment := range item.Attachments {
		attachmentIDs = append(attachmentIDs, attachment.ID)
	}
	return BackupNote{ID: item.ID, NotebookID: item.NotebookID, Title: item.Title, Content: item.Content, Pinned: item.Pinned,
		Revision: item.Revision, TagIDs: uniqueStrings(tagIDs), AttachmentIDs: uniqueStrings(attachmentIDs), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func noteSyncPayloadTx(ctx context.Context, tx *sql.Tx, noteID string) (BackupNote, error) {
	var item BackupNote
	var pinned int
	if err := tx.QueryRowContext(ctx, `SELECT id,notebook_id,title,content,pinned,revision,created_at,updated_at FROM notes WHERE id=?`, noteID).
		Scan(&item.ID, &item.NotebookID, &item.Title, &item.Content, &pinned, &item.Revision, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return BackupNote{}, err
	}
	item.Pinned = pinned == 1
	for _, query := range []struct {
		sql  string
		dest *[]string
	}{
		{`SELECT tag_id FROM note_tag_links WHERE note_id=? ORDER BY tag_id`, &item.TagIDs},
		{`SELECT attachment_id FROM note_attachments WHERE note_id=? ORDER BY position,attachment_id`, &item.AttachmentIDs},
	} {
		rows, err := tx.QueryContext(ctx, query.sql, noteID)
		if err != nil {
			return BackupNote{}, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return BackupNote{}, err
			}
			*query.dest = append(*query.dest, id)
		}
		if err := rows.Close(); err != nil {
			return BackupNote{}, err
		}
	}
	return item, nil
}

func saveNoteVersionTx(ctx context.Context, tx *sql.Tx, noteID, title, content string, revision int64, force bool) error {
	var tagIDs, attachmentIDs []string
	for _, query := range []struct {
		sql  string
		dest *[]string
	}{
		{`SELECT tag_id FROM note_tag_links WHERE note_id = ? ORDER BY tag_id`, &tagIDs},
		{`SELECT attachment_id FROM note_attachments WHERE note_id = ? ORDER BY position, attachment_id`, &attachmentIDs},
	} {
		rows, err := tx.QueryContext(ctx, query.sql, noteID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			*query.dest = append(*query.dest, id)
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	tagJSON, _ := json.Marshal(tagIDs)
	attachmentJSON, _ := json.Marshal(attachmentIDs)
	now := time.Now().UTC()
	actorID, actorName := currentActorTx(ctx, tx)
	var latestID, latestAt string
	err := tx.QueryRowContext(ctx, `SELECT id, created_at FROM note_versions WHERE note_id = ? AND actor_id IN (?,?) ORDER BY created_at DESC LIMIT 1`, noteID, actorID, localUserID).Scan(&latestID, &latestAt)
	coalesce := false
	if err == nil && !force {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, latestAt); parseErr == nil && now.Sub(parsed) <= 5*time.Minute {
			coalesce = true
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if coalesce {
		_, err = tx.ExecContext(ctx, `UPDATE note_versions SET title = ?, content = ?, tag_ids = ?, attachment_ids = ?, actor_id = ?, actor_name = ?, source_revision = ?, created_at = ? WHERE id = ?`,
			title, content, string(tagJSON), string(attachmentJSON), actorID, actorName, revision, now.Format(time.RFC3339Nano), latestID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO note_versions(id, note_id, title, content, tag_ids, attachment_ids, actor_id, actor_name, source_revision, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), noteID, title, content, string(tagJSON), string(attachmentJSON), actorID, actorName, revision, now.Format(time.RFC3339Nano))
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_versions WHERE note_id = ? AND created_at < ?`, noteID, now.AddDate(0, 0, -90).Format(time.RFC3339Nano)); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM note_versions WHERE note_id = ? AND id NOT IN
		(SELECT id FROM note_versions WHERE note_id = ? ORDER BY created_at DESC LIMIT 100)`, noteID, noteID)
	return err
}

func enqueueLatestNoteVersionTx(ctx context.Context, tx *sql.Tx, noteID string) error {
	var item BackupNoteVersion
	var tagJSON, attachmentJSON string
	if err := tx.QueryRowContext(ctx, `SELECT v.id,v.note_id,n.notebook_id,v.title,v.content,v.tag_ids,v.attachment_ids,
		v.actor_id,v.actor_name,v.source_revision,v.created_at FROM note_versions v JOIN notes n ON n.id=v.note_id
		WHERE v.note_id=? ORDER BY v.created_at DESC LIMIT 1`, noteID).Scan(&item.ID, &item.NoteID, &item.NotebookID, &item.Title,
		&item.Content, &tagJSON, &attachmentJSON, &item.ActorID, &item.ActorName, &item.SourceRevision, &item.CreatedAt); err != nil {
		return err
	}
	_ = json.Unmarshal([]byte(tagJSON), &item.TagIDs)
	_ = json.Unmarshal([]byte(attachmentJSON), &item.AttachmentIDs)
	return enqueueOutboxTx(ctx, tx, "note_version", item.ID, "upsert", 0, item)
}

func (s *Store) ListNoteVersions(ctx context.Context, noteID string) ([]NoteVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, note_id, title, content, tag_ids, attachment_ids, actor_id, actor_name, source_revision, created_at
		FROM note_versions WHERE note_id = ? ORDER BY created_at DESC`, noteID)
	if err != nil {
		return nil, err
	}
	type rawVersion struct {
		item                  NoteVersion
		tagIDs, attachmentIDs []string
	}
	rawItems := []rawVersion{}
	for rows.Next() {
		var raw rawVersion
		var tagJSON, attachmentJSON string
		if err := rows.Scan(&raw.item.ID, &raw.item.NoteID, &raw.item.Title, &raw.item.Content, &tagJSON, &attachmentJSON,
			&raw.item.ActorID, &raw.item.ActorName, &raw.item.SourceRevision, &raw.item.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagJSON), &raw.tagIDs)
		_ = json.Unmarshal([]byte(attachmentJSON), &raw.attachmentIDs)
		rawItems = append(rawItems, raw)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]NoteVersion, 0, len(rawItems))
	for _, raw := range rawItems {
		raw.item.Tags, _ = s.noteTagsByIDs(ctx, raw.tagIDs)
		raw.item.Attachments, _ = s.attachmentsByIDs(ctx, raw.attachmentIDs)
		items = append(items, raw.item)
	}
	return items, nil
}

func (s *Store) RestoreNoteVersion(ctx context.Context, versionID string) (Note, error) {
	var noteID, title, content, tagJSON, attachmentJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT note_id, title, content, tag_ids, attachment_ids FROM note_versions WHERE id = ?`, versionID).
		Scan(&noteID, &title, &content, &tagJSON, &attachmentJSON); err != nil {
		return Note{}, errors.New("历史版本不存在")
	}
	current, err := s.GetNote(ctx, noteID)
	if err != nil {
		return Note{}, err
	}
	var tagIDs, attachmentIDs []string
	_ = json.Unmarshal([]byte(tagJSON), &tagIDs)
	_ = json.Unmarshal([]byte(attachmentJSON), &attachmentIDs)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, err
	}
	defer tx.Rollback()
	nextRevision := current.Revision + 1
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET title = ?, content = ?, revision = ?, updated_at = ? WHERE id = ?`, title, content, nextRevision, nowString(), noteID); err != nil {
		return Note{}, err
	}
	if err := replaceNoteTagsTx(ctx, tx, noteID, current.NotebookID, tagIDs); err != nil {
		return Note{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_attachments WHERE note_id = ?`, noteID); err != nil {
		return Note{}, err
	}
	for position, id := range uniqueStrings(attachmentIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO note_attachments(note_id, attachment_id, position) SELECT ?, id, ? FROM attachments WHERE id = ? AND deleted_at IS NULL`, noteID, position, id); err != nil {
			return Note{}, err
		}
	}
	if err := saveNoteVersionTx(ctx, tx, noteID, title, content, nextRevision, true); err != nil {
		return Note{}, err
	}
	payload, err := noteSyncPayloadTx(ctx, tx, noteID)
	if err != nil {
		return Note{}, err
	}
	if err := enqueueOutboxTx(ctx, tx, "note", noteID, "restore_version", current.ServerVersion, payload); err != nil {
		return Note{}, err
	}
	if err := enqueueLatestNoteVersionTx(ctx, tx, noteID); err != nil {
		return Note{}, err
	}
	if err := tx.Commit(); err != nil {
		return Note{}, err
	}
	return s.GetNote(ctx, noteID)
}

func (s *Store) noteTagsByIDs(ctx context.Context, ids []string) ([]NoteTag, error) {
	items := []NoteTag{}
	for _, id := range ids {
		item, err := scanNoteTag(s.db.QueryRowContext(ctx, `SELECT `+noteTagColumns+` FROM note_tags WHERE id = ?`, id))
		if err == nil {
			items = append(items, item)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return items, nil
}

var mentionPattern = regexp.MustCompile(`@([A-Za-z0-9_.-]{2,32})`)

func (s *Store) ListComments(ctx context.Context, noteID string) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, note_id, COALESCE(parent_id, ''), author_id, author_name, content, COALESCE(deleted_at, ''), created_at, updated_at
		FROM comments WHERE note_id = ? ORDER BY created_at`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Comment{}
	for rows.Next() {
		var item Comment
		if err := rows.Scan(&item.ID, &item.NoteID, &item.ParentID, &item.AuthorID, &item.AuthorName, &item.Content, &item.DeletedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateComment(ctx context.Context, input CommentInput) (Comment, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return Comment{}, errors.New("评论内容不能为空")
	}
	if len([]rune(content)) > 4000 {
		return Comment{}, errors.New("评论不能超过 4000 个字符")
	}
	if _, err := s.GetNote(ctx, input.NoteID); err != nil {
		return Comment{}, err
	}
	now := nowString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Comment{}, err
	}
	defer tx.Rollback()
	actorID, actorName := currentActorTx(ctx, tx)
	item := Comment{ID: uuid.NewString(), NoteID: input.NoteID, ParentID: input.ParentID, AuthorID: actorID, AuthorName: actorName, Content: content, CreatedAt: now, UpdatedAt: now}
	var parent any
	if input.ParentID != "" {
		parent = input.ParentID
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM comments WHERE id = ? AND note_id = ?`, input.ParentID, input.NoteID).Scan(&count); err != nil || count == 0 {
			return Comment{}, errors.New("回复的评论不存在")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO comments(id, note_id, parent_id, author_id, author_name, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.NoteID, parent, item.AuthorID, item.AuthorName, item.Content, now, now); err != nil {
		return Comment{}, err
	}
	for _, match := range mentionPattern.FindAllStringSubmatch(content, -1) {
		username := match[1]
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO mentions(comment_id, username) VALUES (?, ?)`, item.ID, username); err != nil {
			return Comment{}, err
		}
	}
	if err := enqueueOutboxTx(ctx, tx, "comment", item.ID, "upsert", 0, item); err != nil {
		return Comment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Comment{}, err
	}
	return item, nil
}

func (s *Store) UpdateComment(ctx context.Context, input CommentInput) (Comment, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return Comment{}, errors.New("评论内容不能为空")
	}
	actorID, _ := s.currentActor(ctx)
	result, err := s.db.ExecContext(ctx, `UPDATE comments SET content = ?, updated_at = ? WHERE id = ? AND author_id IN (?, ?) AND deleted_at IS NULL`, content, nowString(), input.ID, actorID, localUserID)
	if err != nil {
		return Comment{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Comment{}, errors.New("评论不存在或没有编辑权限")
	}
	comments, err := s.ListComments(ctx, input.NoteID)
	if err != nil {
		return Comment{}, err
	}
	for _, item := range comments {
		if item.ID == input.ID {
			_ = s.enqueueOutbox(ctx, "comment", item.ID, "upsert", 0, item)
			return item, nil
		}
	}
	return Comment{}, errors.New("评论不存在")
}

func (s *Store) DeleteComment(ctx context.Context, id string) error {
	var item Comment
	if err := s.db.QueryRowContext(ctx, `SELECT id,note_id,COALESCE(parent_id,''),author_id,author_name,content,COALESCE(deleted_at,''),created_at,updated_at FROM comments WHERE id=?`, id).
		Scan(&item.ID, &item.NoteID, &item.ParentID, &item.AuthorID, &item.AuthorName, &item.Content, &item.DeletedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return errors.New("评论不存在或没有删除权限")
	}
	now := nowString()
	actorID, _ := s.currentActor(ctx)
	result, err := s.db.ExecContext(ctx, `UPDATE comments SET deleted_at = ?, updated_at = ? WHERE id = ? AND author_id IN (?, ?) AND deleted_at IS NULL`, now, now, id, actorID, localUserID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("评论不存在或没有删除权限")
	}
	item.DeletedAt, item.UpdatedAt = now, now
	_ = s.enqueueOutbox(ctx, "comment", id, "delete", 0, item)
	return nil
}

func (s *Store) ListNotebookMembers(ctx context.Context, notebookID string) ([]NotebookMember, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT notebook_id, user_id, username, role, created_at FROM memberships WHERE notebook_id = ? ORDER BY CASE role WHEN 'owner' THEN 0 WHEN 'editor' THEN 1 ELSE 2 END, username`, notebookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []NotebookMember{}
	for rows.Next() {
		var item NotebookMember
		if err := rows.Scan(&item.NotebookID, &item.UserID, &item.Username, &item.Role, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) AcquireEditLease(ctx context.Context, noteID string, takeover bool) (EditLease, error) {
	if _, err := s.GetNote(ctx, noteID); err != nil {
		return EditLease{}, err
	}
	now := time.Now().UTC()
	var holderID, holderName, expires string
	err := s.db.QueryRowContext(ctx, `SELECT holder_id, holder_name, expires_at FROM edit_leases WHERE note_id = ?`, noteID).Scan(&holderID, &holderName, &expires)
	if err == nil {
		expiresAt, _ := time.Parse(time.RFC3339Nano, expires)
		if holderID != localUserID && expiresAt.After(now) && !takeover {
			return EditLease{NoteID: noteID, HolderID: holderID, HolderName: holderName, ExpiresAt: expires, Editable: false}, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return EditLease{}, err
	}
	expiresAt := now.Add(2 * time.Minute).Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT INTO edit_leases(note_id, holder_id, holder_name, expires_at) VALUES (?, ?, '本地用户', ?)
		ON CONFLICT(note_id) DO UPDATE SET holder_id = excluded.holder_id, holder_name = excluded.holder_name, expires_at = excluded.expires_at`, noteID, localUserID, expiresAt)
	return EditLease{NoteID: noteID, HolderID: localUserID, HolderName: "本地用户", ExpiresAt: expiresAt, Editable: true}, err
}

func (s *Store) ReleaseEditLease(ctx context.Context, noteID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM edit_leases WHERE note_id = ? AND holder_id = ?`, noteID, localUserID)
	return err
}

func (s *Store) GetNoteCollectionCounts(ctx context.Context) (map[string]int, map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT notebook_id, COUNT(*) FROM notes WHERE deleted_at IS NULL GROUP BY notebook_id`)
	if err != nil {
		return nil, nil, err
	}
	notebookCounts, err := scanCollectionCounts(rows)
	if err != nil {
		return nil, nil, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT l.tag_id, COUNT(*) FROM note_tag_links l JOIN notes n ON n.id = l.note_id
		JOIN note_tags t ON t.id = l.tag_id WHERE n.deleted_at IS NULL AND t.deleted_at IS NULL GROUP BY l.tag_id`)
	if err != nil {
		return nil, nil, err
	}
	tagCounts, err := scanCollectionCounts(rows)
	return notebookCounts, tagCounts, err
}

func addTombstoneTx(ctx context.Context, tx *sql.Tx, entityType, entityID, deletedAt string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO tombstones(id, entity_type, entity_id, deleted_at) VALUES (?, ?, ?, ?)`, uuid.NewString(), entityType, entityID, deletedAt)
	return err
}

func enqueueOutboxTx(ctx context.Context, tx *sql.Tx, entityType, entityID, action string, baseVersion int64, payload any) error {
	var configured int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_state WHERE key = 'server_url' AND value <> ''`).Scan(&configured); err != nil {
		return err
	}
	if configured == 0 {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox(id, entity_type, entity_id, action, base_version, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), entityType, entityID, action, baseVersion, string(data), nowString())
	return err
}

func (s *Store) enqueueOutbox(ctx context.Context, entityType, entityID, action string, baseVersion int64, payload any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := enqueueOutboxTx(ctx, tx, entityType, entityID, action, baseVersion, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetSyncStatus(ctx context.Context) (SyncStatus, error) {
	status := SyncStatus{}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM sync_state`)
	if err != nil {
		return status, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return status, err
		}
		switch key {
		case "server_url":
			status.ServerURL, status.Configured = value, value != ""
		case "last_sync_at":
			status.LastSyncAt = value
		case "last_error":
			status.LastError = value
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox`).Scan(&status.Pending); err != nil {
		return status, err
	}
	status.Online = status.Configured && status.LastError == ""
	return status, rows.Err()
}
