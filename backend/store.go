package backend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const schemaVersion = 2

type migration struct {
	version    int
	statements []string
}

type Store struct {
	db            *sql.DB
	path          string
	attachmentDir string
	temporaryDir  bool
}

func OpenStore(path string) (*Store, error) {
	dsn := "file:" + filepath.ToSlash(path)
	if path == ":memory:" {
		dsn = "file:todo-memory?mode=memory&cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure database: %w", err)
		}
	}
	attachmentDir := filepath.Join(filepath.Dir(path), "attachments")
	temporaryDir := false
	if path == ":memory:" {
		attachmentDir, err = os.MkdirTemp("", "todo-attachments-")
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("create attachment directory: %w", err)
		}
		temporaryDir = true
	}
	if err := os.MkdirAll(attachmentDir, 0o700); err != nil {
		db.Close()
		return nil, fmt.Errorf("create attachment directory: %w", err)
	}
	s := &Store{db: db, path: path, attachmentDir: attachmentDir, temporaryDir: temporaryDir}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.seed(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	err := s.db.Close()
	if s.temporaryDir {
		_ = os.RemoveAll(s.attachmentDir)
	}
	return err
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	migrations := []migration{
		{version: 1, statements: []string{
			`CREATE TABLE IF NOT EXISTS lists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL COLLATE NOCASE UNIQUE,
			position INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
			`CREATE TABLE IF NOT EXISTS tags (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL COLLATE NOCASE UNIQUE,
			color TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
			`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			notes TEXT NOT NULL DEFAULT '',
			list_id TEXT REFERENCES lists(id) ON DELETE SET NULL,
			priority TEXT NOT NULL DEFAULT 'none' CHECK(priority IN ('none','low','medium','high')),
			due_date TEXT,
			completed_at TEXT,
			deleted_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
			`CREATE TABLE IF NOT EXISTS task_tags (
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			PRIMARY KEY(task_id, tag_id)
		)`,
			`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_list ON tasks(list_id)`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(due_date)`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_completed ON tasks(completed_at)`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_deleted ON tasks(deleted_at)`,
		}},
		{version: 2, statements: []string{
			`CREATE TABLE IF NOT EXISTS notebooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT 'personal' CHECK(kind IN ('personal','shared')),
			role TEXT NOT NULL DEFAULT 'owner' CHECK(role IN ('owner','editor','viewer')),
			owner_id TEXT NOT NULL DEFAULT 'local',
			is_default INTEGER NOT NULL DEFAULT 0 CHECK(is_default IN (0,1)),
			deleted_at TEXT,
			server_version INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_notebooks_default ON notebooks(is_default) WHERE is_default = 1 AND deleted_at IS NULL`,
			`CREATE INDEX IF NOT EXISTS idx_notebooks_deleted ON notebooks(deleted_at)`,
			`CREATE TABLE IF NOT EXISTS notes (
			id TEXT PRIMARY KEY,
			notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE RESTRICT,
			title TEXT NOT NULL DEFAULT '无标题笔记',
			content TEXT NOT NULL DEFAULT '',
			pinned INTEGER NOT NULL DEFAULT 0 CHECK(pinned IN (0,1)),
			revision INTEGER NOT NULL DEFAULT 1,
			deleted_at TEXT,
			server_version INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
			`CREATE INDEX IF NOT EXISTS idx_notes_notebook ON notes(notebook_id, deleted_at, pinned, updated_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_notes_deleted ON notes(deleted_at)`,
			`CREATE TABLE IF NOT EXISTS note_tags (
			id TEXT PRIMARY KEY,
			notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE CASCADE,
			name TEXT NOT NULL COLLATE NOCASE,
			color TEXT NOT NULL DEFAULT 'coral',
			deleted_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(notebook_id, name)
		)`,
			`CREATE INDEX IF NOT EXISTS idx_note_tags_notebook ON note_tags(notebook_id, deleted_at)`,
			`CREATE TABLE IF NOT EXISTS note_tag_links (
			note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			tag_id TEXT NOT NULL REFERENCES note_tags(id) ON DELETE CASCADE,
			PRIMARY KEY(note_id, tag_id)
		)`,
			`CREATE TABLE IF NOT EXISTS note_task_links (
			note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL DEFAULT 'local',
			PRIMARY KEY(note_id, task_id, user_id)
		)`,
			`CREATE INDEX IF NOT EXISTS idx_note_task_links_task ON note_task_links(task_id, user_id)`,
			`CREATE TABLE IF NOT EXISTS attachments (
			id TEXT PRIMARY KEY,
			notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE CASCADE,
			file_name TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			local_path TEXT NOT NULL,
			remote_key TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'local' CHECK(status IN ('local','pending','uploading','synced','failed')),
			deleted_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
			`CREATE INDEX IF NOT EXISTS idx_attachments_hash ON attachments(notebook_id, sha256)`,
			`CREATE INDEX IF NOT EXISTS idx_attachments_deleted ON attachments(deleted_at)`,
			`CREATE TABLE IF NOT EXISTS note_attachments (
			note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			attachment_id TEXT NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
			position INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(note_id, attachment_id)
		)`,
			`CREATE TABLE IF NOT EXISTS note_versions (
			id TEXT PRIMARY KEY,
			note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			tag_ids TEXT NOT NULL DEFAULT '[]',
			attachment_ids TEXT NOT NULL DEFAULT '[]',
			actor_id TEXT NOT NULL,
			actor_name TEXT NOT NULL,
			source_revision INTEGER NOT NULL,
			created_at TEXT NOT NULL
		)`,
			`CREATE INDEX IF NOT EXISTS idx_note_versions_note ON note_versions(note_id, created_at DESC)`,
			`CREATE TABLE IF NOT EXISTS comments (
			id TEXT PRIMARY KEY,
			note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			parent_id TEXT REFERENCES comments(id) ON DELETE CASCADE,
			author_id TEXT NOT NULL,
			author_name TEXT NOT NULL,
			content TEXT NOT NULL,
			deleted_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
			`CREATE INDEX IF NOT EXISTS idx_comments_note ON comments(note_id, deleted_at, created_at)`,
			`CREATE TABLE IF NOT EXISTS mentions (
			comment_id TEXT NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
			username TEXT NOT NULL,
			read_at TEXT,
			PRIMARY KEY(comment_id, username)
		)`,
			`CREATE TABLE IF NOT EXISTS memberships (
			notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL,
			username TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('owner','editor','viewer')),
			created_at TEXT NOT NULL,
			PRIMARY KEY(notebook_id, user_id)
		)`,
			`CREATE TABLE IF NOT EXISTS notifications (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			message TEXT NOT NULL,
			read_at TEXT,
			created_at TEXT NOT NULL
		)`,
			`CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, read_at, created_at DESC)`,
			`CREATE TABLE IF NOT EXISTS outbox (
			id TEXT PRIMARY KEY,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			action TEXT NOT NULL,
			base_version INTEGER NOT NULL DEFAULT 0,
			payload TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
			`CREATE INDEX IF NOT EXISTS idx_outbox_created ON outbox(created_at)`,
			`CREATE TABLE IF NOT EXISTS sync_state (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS tombstones (
			id TEXT PRIMARY KEY,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			deleted_at TEXT NOT NULL
		)`,
			`CREATE INDEX IF NOT EXISTS idx_tombstones_deleted ON tombstones(deleted_at)`,
			`CREATE TABLE IF NOT EXISTS edit_leases (
			note_id TEXT PRIMARY KEY REFERENCES notes(id) ON DELETE CASCADE,
			holder_id TEXT NOT NULL,
			holder_name TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`,
		}},
	}
	for _, item := range migrations {
		if item.version <= current {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, statement := range item.statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				tx.Rollback()
				return fmt.Errorf("apply schema version %d: %w", item.version, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, item.version, nowString()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seed(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lists`).Scan(&count); err != nil {
		return err
	}
	now := nowString()
	if count == 0 {
		for position, name := range []string{"个人", "工作"} {
			if _, err := s.db.ExecContext(ctx,
				`INSERT INTO lists(id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
				uuid.NewString(), name, position, now, now); err != nil {
				return err
			}
		}
	}
	var notebookCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notebooks WHERE deleted_at IS NULL`).Scan(&notebookCount); err != nil {
		return err
	}
	if notebookCount == 0 {
		notebookID := uuid.NewString()
		if _, err := s.db.ExecContext(ctx, `INSERT INTO notebooks
			(id, name, kind, role, owner_id, is_default, created_at, updated_at)
			VALUES (?, '我的笔记', 'personal', 'owner', 'local', 1, ?, ?)`, notebookID, now, now); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO memberships(notebook_id, user_id, username, role, created_at)
			VALUES (?, 'local', '本地用户', 'owner', ?)`, notebookID, now); err != nil {
			return err
		}
	}
	defaults := map[string]string{"theme": "system", "startupView": "today"}
	for key, value := range defaults {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key, value) VALUES (?, ?)`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func validPriority(priority string) bool {
	return priority == "none" || priority == "low" || priority == "medium" || priority == "high"
}

func validateDate(value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return errors.New("截止日期格式无效")
	}
	return nil
}

func normalizeTaskInput(title, priority, dueDate string) (string, string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", errors.New("任务标题不能为空")
	}
	if len([]rune(title)) > 240 {
		return "", "", errors.New("任务标题不能超过 240 个字符")
	}
	if priority == "" {
		priority = "none"
	}
	if !validPriority(priority) {
		return "", "", errors.New("优先级无效")
	}
	if err := validateDate(dueDate); err != nil {
		return "", "", err
	}
	return title, priority, nil
}

func (s *Store) GetBootstrap(ctx context.Context, now time.Time) (Bootstrap, error) {
	lists, err := s.ListLists(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	tags, err := s.ListTags(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	counts, err := s.GetSmartCounts(ctx, now)
	if err != nil {
		return Bootstrap{}, err
	}
	listCounts, tagCounts, err := s.GetCollectionCounts(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	notebooks, err := s.ListNotebooks(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	noteTags, err := s.ListNoteTags(ctx, "")
	if err != nil {
		return Bootstrap{}, err
	}
	notebookCounts, noteTagCounts, err := s.GetNoteCollectionCounts(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	syncStatus, _ := s.GetSyncStatus(ctx)
	currentUser := AccountUser{ID: localUserID, Username: "本地用户", Mode: "local"}
	if syncStatus.Configured {
		state, _ := (&SyncClient{store: s}).syncState(ctx)
		if state["user_id"] != "" {
			currentUser = AccountUser{ID: state["user_id"], Username: state["username"], Mode: "sync"}
		}
	}
	var unread int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = 'local' AND read_at IS NULL`).Scan(&unread)
	return Bootstrap{
		Lists: lists, Tags: tags, Notebooks: notebooks, NoteTags: noteTags, Settings: settings,
		Counts: counts, ListCounts: listCounts, TagCounts: tagCounts, NotebookCounts: notebookCounts,
		NoteTagCounts: noteTagCounts, CurrentUser: currentUser,
		Sync: syncStatus, UnreadCount: unread,
	}, nil
}

func (s *Store) ListLists(ctx context.Context) ([]TodoList, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, position, created_at, updated_at FROM lists ORDER BY position, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TodoList{}
	for rows.Next() {
		var item TodoList
		if err := rows.Scan(&item.ID, &item.Name, &item.Position, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, created_at, updated_at FROM tags ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Tag{}
	for rows.Next() {
		var item Tag
		if err := rows.Scan(&item.ID, &item.Name, &item.Color, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner rowScanner) (Task, error) {
	var task Task
	err := scanner.Scan(
		&task.ID, &task.Title, &task.Notes, &task.ListID, &task.Priority,
		&task.DueDate, &task.CompletedAt, &task.DeletedAt, &task.CreatedAt, &task.UpdatedAt,
	)
	task.Tags = []Tag{}
	return task, err
}

const taskColumns = `id, title, notes, COALESCE(list_id, ''), priority, COALESCE(due_date, ''),
	COALESCE(completed_at, ''), COALESCE(deleted_at, ''), created_at, updated_at`

func (s *Store) ListTasks(ctx context.Context, query TaskQuery, now time.Time) ([]Task, error) {
	clauses := []string{"deleted_at IS NULL"}
	args := []any{}
	today := now.In(time.Local).Format("2006-01-02")

	switch query.View {
	case "inbox":
		clauses = append(clauses, "list_id IS NULL")
	case "today":
		clauses = append(clauses, "due_date <= ?")
		args = append(args, today)
	case "upcoming":
		clauses = append(clauses, "due_date > ?")
		args = append(args, today)
	case "list":
		clauses = append(clauses, "list_id = ?")
		args = append(args, query.ListID)
	}
	if query.View == "completed" || query.Status == "completed" {
		clauses = append(clauses, "completed_at IS NOT NULL")
	} else if query.Status != "all" {
		clauses = append(clauses, "completed_at IS NULL")
	}
	if query.Priority != "" && query.Priority != "all" {
		if !validPriority(query.Priority) {
			return nil, errors.New("优先级筛选无效")
		}
		clauses = append(clauses, "priority = ?")
		args = append(args, query.Priority)
	}
	if query.TagID != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM task_tags tt WHERE tt.task_id = tasks.id AND tt.tag_id = ?)`)
		args = append(args, query.TagID)
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		clauses = append(clauses, "(LOWER(title) LIKE LOWER(?) OR LOWER(notes) LIKE LOWER(?))")
		value := "%" + search + "%"
		args = append(args, value, value)
	}
	orderBy := `CASE WHEN due_date IS NULL THEN 1 ELSE 0 END, due_date ASC,
		CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 WHEN 'low' THEN 2 ELSE 3 END, created_at DESC`
	switch query.Sort {
	case "priority":
		orderBy = `CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 WHEN 'low' THEN 2 ELSE 3 END,
			CASE WHEN due_date IS NULL THEN 1 ELSE 0 END, due_date ASC, created_at DESC`
	case "created":
		orderBy = "created_at DESC"
	case "due", "":
	default:
		return nil, errors.New("排序方式无效")
	}
	sqlQuery := `SELECT ` + taskColumns + ` FROM tasks WHERE ` + strings.Join(clauses, " AND ") + ` ORDER BY ` + orderBy
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	result := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, task)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range result {
		tags, err := s.tagsForTask(ctx, result[i].ID)
		if err != nil {
			return nil, err
		}
		result[i].Tags = tags
	}
	return result, nil
}

func (s *Store) tagsForTask(ctx context.Context, taskID string) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tags.id, tags.name, tags.color, tags.created_at, tags.updated_at
		FROM tags JOIN task_tags ON task_tags.tag_id = tags.id
		WHERE task_tags.task_id = ? ORDER BY tags.name COLLATE NOCASE`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Tag{}
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Color, &tag.CreatedAt, &tag.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, tag)
	}
	return result, rows.Err()
}

func (s *Store) getTask(ctx context.Context, id string) (Task, error) {
	task, err := scanTask(s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, errors.New("任务不存在")
	}
	if err != nil {
		return Task{}, err
	}
	task.Tags, err = s.tagsForTask(ctx, id)
	return task, err
}

func (s *Store) validateReferences(ctx context.Context, listID string, tagIDs []string) error {
	if listID != "" {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lists WHERE id = ?`, listID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return errors.New("所选清单不存在")
		}
	}
	seen := map[string]bool{}
	for _, id := range tagIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE id = ?`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return errors.New("所选标签不存在")
		}
	}
	return nil
}

func (s *Store) CreateTask(ctx context.Context, input CreateTaskInput) (Task, error) {
	title, priority, err := normalizeTaskInput(input.Title, input.Priority, input.DueDate)
	if err != nil {
		return Task{}, err
	}
	if err := s.validateReferences(ctx, input.ListID, input.TagIDs); err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	id, now := uuid.NewString(), nowString()
	var listID, dueDate any
	if input.ListID != "" {
		listID = input.ListID
	}
	if input.DueDate != "" {
		dueDate = input.DueDate
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tasks
		(id, title, notes, list_id, priority, due_date, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, title, input.Notes, listID, priority, dueDate, now, now); err != nil {
		return Task{}, err
	}
	if err := replaceTaskTags(ctx, tx, id, input.TagIDs); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.getTask(ctx, id)
}

func replaceTaskTags(ctx context.Context, tx *sql.Tx, taskID string, tagIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, tagID := range tagIDs {
		if tagID == "" || seen[tagID] {
			continue
		}
		seen[tagID] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_tags(task_id, tag_id) VALUES (?, ?)`, taskID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateTask(ctx context.Context, input UpdateTaskInput) (Task, error) {
	title, priority, err := normalizeTaskInput(input.Title, input.Priority, input.DueDate)
	if err != nil {
		return Task{}, err
	}
	if input.ID == "" {
		return Task{}, errors.New("任务 ID 不能为空")
	}
	if err := s.validateReferences(ctx, input.ListID, input.TagIDs); err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	var listID, dueDate any
	if input.ListID != "" {
		listID = input.ListID
	}
	if input.DueDate != "" {
		dueDate = input.DueDate
	}
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET title = ?, notes = ?, list_id = ?, priority = ?,
		due_date = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		title, input.Notes, listID, priority, dueDate, nowString(), input.ID)
	if err != nil {
		return Task{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Task{}, errors.New("任务不存在")
	}
	if err := replaceTaskTags(ctx, tx, input.ID, input.TagIDs); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.getTask(ctx, input.ID)
}

func (s *Store) SetTaskCompleted(ctx context.Context, id string, completed bool) (Task, error) {
	var value any
	if completed {
		value = nowString()
	}
	result, err := s.db.ExecContext(ctx, `UPDATE tasks SET completed_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, value, nowString(), id)
	if err != nil {
		return Task{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Task{}, errors.New("任务不存在")
	}
	return s.getTask(ctx, id)
}

func (s *Store) DeleteTask(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, nowString(), nowString(), id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("任务不存在")
	}
	return nil
}

func (s *Store) RestoreTask(ctx context.Context, id string) (Task, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE tasks SET deleted_at = NULL, updated_at = ? WHERE id = ? AND deleted_at IS NOT NULL`, nowString(), id)
	if err != nil {
		return Task{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Task{}, errors.New("任务无法恢复")
	}
	return s.getTask(ctx, id)
}

func (s *Store) PurgeDeleted(ctx context.Context, before time.Time) error {
	cutoff := before.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`DELETE FROM notes WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
		`DELETE FROM note_tags WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
		`DELETE FROM notebooks WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
		`DELETE FROM tasks WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, cutoff); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tombstones WHERE deleted_at < ?`, before.AddDate(0, -2, 0).UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateList(ctx context.Context, name string) (TodoList, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return TodoList{}, errors.New("清单名称不能为空")
	}
	if len([]rune(name)) > 40 {
		return TodoList{}, errors.New("清单名称不能超过 40 个字符")
	}
	var position int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM lists`).Scan(&position); err != nil {
		return TodoList{}, err
	}
	item := TodoList{ID: uuid.NewString(), Name: name, Position: position, CreatedAt: nowString()}
	item.UpdatedAt = item.CreatedAt
	_, err := s.db.ExecContext(ctx, `INSERT INTO lists(id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, item.ID, item.Name, item.Position, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return TodoList{}, errors.New("清单名称已存在")
		}
		return TodoList{}, err
	}
	return item, nil
}

func (s *Store) UpdateList(ctx context.Context, input ListInput) (TodoList, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return TodoList{}, errors.New("清单名称不能为空")
	}
	if len([]rune(name)) > 40 {
		return TodoList{}, errors.New("清单名称不能超过 40 个字符")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE lists SET name = ?, updated_at = ? WHERE id = ?`, name, nowString(), input.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return TodoList{}, errors.New("清单名称已存在")
		}
		return TodoList{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return TodoList{}, errors.New("清单不存在")
	}
	var item TodoList
	err = s.db.QueryRowContext(ctx, `SELECT id, name, position, created_at, updated_at FROM lists WHERE id = ?`, input.ID).
		Scan(&item.ID, &item.Name, &item.Position, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) DeleteList(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET list_id = NULL, updated_at = ? WHERE list_id = ?`, nowString(), id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM lists WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("清单不存在")
	}
	return tx.Commit()
}

func normalizeTagColor(color string) string {
	allowed := map[string]bool{"coral": true, "amber": true, "mint": true, "blue": true, "violet": true, "rose": true}
	if !allowed[color] {
		return "coral"
	}
	return color
}

func (s *Store) CreateTag(ctx context.Context, input TagInput) (Tag, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Tag{}, errors.New("标签名称不能为空")
	}
	if len([]rune(name)) > 28 {
		return Tag{}, errors.New("标签名称不能超过 28 个字符")
	}
	item := Tag{ID: uuid.NewString(), Name: name, Color: normalizeTagColor(input.Color), CreatedAt: nowString()}
	item.UpdatedAt = item.CreatedAt
	_, err := s.db.ExecContext(ctx, `INSERT INTO tags(id, name, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, item.ID, item.Name, item.Color, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Tag{}, errors.New("标签名称已存在")
		}
		return Tag{}, err
	}
	return item, nil
}

func (s *Store) UpdateTag(ctx context.Context, input TagInput) (Tag, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Tag{}, errors.New("标签名称不能为空")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE tags SET name = ?, color = ?, updated_at = ? WHERE id = ?`, name, normalizeTagColor(input.Color), nowString(), input.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Tag{}, errors.New("标签名称已存在")
		}
		return Tag{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Tag{}, errors.New("标签不存在")
	}
	var item Tag
	err = s.db.QueryRowContext(ctx, `SELECT id, name, color, created_at, updated_at FROM tags WHERE id = ?`, input.ID).
		Scan(&item.ID, &item.Name, &item.Color, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) DeleteTag(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("标签不存在")
	}
	return nil
}

func (s *Store) GetSettings(ctx context.Context) (AppSettings, error) {
	settings := AppSettings{Theme: "system", StartupView: "today"}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return settings, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return settings, err
		}
		switch key {
		case "theme":
			settings.Theme = value
		case "startupView":
			settings.StartupView = value
		}
	}
	return settings, rows.Err()
}

func (s *Store) UpdateSettings(ctx context.Context, settings AppSettings) (AppSettings, error) {
	if settings.Theme != "system" && settings.Theme != "light" && settings.Theme != "dark" {
		return AppSettings{}, errors.New("主题设置无效")
	}
	allowedViews := map[string]bool{"overview": true, "inbox": true, "today": true, "upcoming": true, "all": true, "notes": true}
	if !allowedViews[settings.StartupView] {
		settings.StartupView = "today"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppSettings{}, err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{"theme": settings.Theme, "startupView": settings.StartupView} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return AppSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AppSettings{}, err
	}
	return settings, nil
}

func (s *Store) GetSmartCounts(ctx context.Context, now time.Time) (SmartCounts, error) {
	today := now.In(time.Local).Format("2006-01-02")
	var counts SmartCounts
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN list_id IS NULL AND completed_at IS NULL THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN due_date <= ? AND completed_at IS NULL THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN due_date > ? AND completed_at IS NULL THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN completed_at IS NULL THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN completed_at IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM tasks WHERE deleted_at IS NULL`, today, today).Scan(&counts.Inbox, &counts.Today, &counts.Upcoming, &counts.All, &counts.Completed)
	if err == nil {
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes WHERE deleted_at IS NULL`).Scan(&counts.Notes)
	}
	return counts, err
}

func scanCollectionCounts(rows *sql.Rows) (map[string]int, error) {
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func (s *Store) GetCollectionCounts(ctx context.Context) (map[string]int, map[string]int, error) {
	listRows, err := s.db.QueryContext(ctx, `SELECT list_id, COUNT(*)
		FROM tasks
		WHERE deleted_at IS NULL AND completed_at IS NULL AND list_id IS NOT NULL
		GROUP BY list_id`)
	if err != nil {
		return nil, nil, err
	}
	listCounts, err := scanCollectionCounts(listRows)
	if err != nil {
		return nil, nil, err
	}

	tagRows, err := s.db.QueryContext(ctx, `SELECT task_tags.tag_id, COUNT(*)
		FROM task_tags
		JOIN tasks ON tasks.id = task_tags.task_id
		WHERE tasks.deleted_at IS NULL AND tasks.completed_at IS NULL
		GROUP BY task_tags.tag_id`)
	if err != nil {
		return nil, nil, err
	}
	tagCounts, err := scanCollectionCounts(tagRows)
	if err != nil {
		return nil, nil, err
	}
	return listCounts, tagCounts, nil
}

func (s *Store) GetOverview(ctx context.Context, now time.Time) (Overview, error) {
	localNow := now.In(time.Local)
	today := localNow.Format("2006-01-02")
	startLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	endLocal := startLocal.AddDate(0, 0, 1)
	var overview Overview
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN completed_at IS NULL THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN completed_at IS NULL AND due_date < ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN completed_at >= ? AND completed_at < ? THEN 1 ELSE 0 END), 0)
		FROM tasks WHERE deleted_at IS NULL`, today, startLocal.UTC().Format(time.RFC3339Nano), endLocal.UTC().Format(time.RFC3339Nano)).
		Scan(&overview.Open, &overview.Overdue, &overview.CompletedToday)
	if err != nil {
		return Overview{}, err
	}
	overview.Week = make([]DayStat, 0, 7)
	for offset := -6; offset <= 0; offset++ {
		dayStart := startLocal.AddDate(0, 0, offset)
		dayEnd := dayStart.AddDate(0, 0, 1)
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE deleted_at IS NULL AND completed_at >= ? AND completed_at < ?`, dayStart.UTC().Format(time.RFC3339Nano), dayEnd.UTC().Format(time.RFC3339Nano)).Scan(&count); err != nil {
			return Overview{}, err
		}
		overview.Week = append(overview.Week, DayStat{Date: dayStart.Format("2006-01-02"), Count: count})
	}
	return overview, nil
}
