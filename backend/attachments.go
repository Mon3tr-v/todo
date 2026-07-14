package backend

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const maxAttachmentBytes int64 = 25 * 1024 * 1024

var allowedAttachmentExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".txt": true, ".md": true, ".csv": true,
	".json": true, ".xml": true, ".yaml": true, ".yml": true, ".zip": true,
	".7z": true, ".rar": true,
}

var blockedAttachmentExtensions = map[string]bool{
	".exe": true, ".dll": true, ".msi": true, ".bat": true, ".cmd": true,
	".com": true, ".scr": true, ".ps1": true, ".js": true, ".vbs": true,
	".html": true, ".htm": true, ".svg": true,
}

func validateAttachment(fileName, mimeType string, size int64) error {
	if size <= 0 {
		return errors.New("附件内容为空")
	}
	if size > maxAttachmentBytes {
		return errors.New("附件不能超过 25MB")
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	if blockedAttachmentExtensions[ext] || !allowedAttachmentExtensions[ext] {
		return errors.New("不支持这种附件类型")
	}
	if strings.Contains(strings.ToLower(mimeType), "svg") || strings.Contains(strings.ToLower(mimeType), "html") {
		return errors.New("不允许上传可执行或主动内容")
	}
	return nil
}

func attachmentMime(fileName string, sniffed []byte) string {
	if detected := http.DetectContentType(sniffed); detected != "application/octet-stream" {
		return detected
	}
	if byExtension := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName))); byExtension != "" {
		return byExtension
	}
	return "application/octet-stream"
}

func scanAttachment(scanner rowScanner) (Attachment, string, error) {
	var item Attachment
	var localPath string
	err := scanner.Scan(&item.ID, &item.NotebookID, &item.FileName, &item.MimeType, &item.Size, &item.SHA256,
		&localPath, &item.RemoteKey, &item.Status, &item.DeletedAt, &item.CreatedAt, &item.UpdatedAt)
	return item, localPath, err
}

const attachmentColumns = `id, notebook_id, file_name, mime_type, size_bytes, sha256, local_path, remote_key, status,
	COALESCE(deleted_at, ''), created_at, updated_at`

func (s *Store) attachmentsForNote(ctx context.Context, noteID string) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.notebook_id, a.file_name, a.mime_type, a.size_bytes, a.sha256,
			a.local_path, a.remote_key, a.status, COALESCE(a.deleted_at, ''), a.created_at, a.updated_at
			FROM attachments a JOIN note_attachments l ON l.attachment_id = a.id
			WHERE l.note_id = ? AND a.deleted_at IS NULL ORDER BY l.position, a.created_at`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Attachment{}
	for rows.Next() {
		item, _, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) attachmentsByIDs(ctx context.Context, ids []string) ([]Attachment, error) {
	items := []Attachment{}
	for _, id := range ids {
		item, _, err := scanAttachment(s.db.QueryRowContext(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE id = ?`, id))
		if err == nil {
			items = append(items, item)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return items, nil
}

func (s *Store) attachmentPath(ctx context.Context, id string) (Attachment, string, error) {
	item, path, err := scanAttachment(s.db.QueryRowContext(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE id = ? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, "", errors.New("附件不存在")
	}
	return item, path, err
}

func (s *Store) AddAttachmentFromPath(ctx context.Context, noteID, sourcePath string) (Attachment, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return Attachment{}, fmt.Errorf("读取附件失败: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Attachment{}, err
	}
	if info.IsDir() {
		return Attachment{}, errors.New("不能将文件夹作为附件")
	}
	return s.addAttachment(ctx, noteID, filepath.Base(sourcePath), file, info.Size())
}

func (s *Store) AddAttachmentData(ctx context.Context, input AttachmentUploadInput) (Attachment, error) {
	data, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil {
		return Attachment{}, errors.New("附件数据无效")
	}
	return s.addAttachment(ctx, input.NoteID, filepath.Base(input.FileName), strings.NewReader(string(data)), int64(len(data)))
}

func (s *Store) addAttachment(ctx context.Context, noteID, fileName string, source io.Reader, size int64) (Attachment, error) {
	note, err := s.GetNote(ctx, noteID)
	if err != nil {
		return Attachment{}, err
	}
	if _, err := s.editableNotebook(ctx, note.NotebookID); err != nil {
		return Attachment{}, err
	}
	if size <= 0 || size > maxAttachmentBytes {
		return Attachment{}, validateAttachment(fileName, "", size)
	}
	temp, err := os.CreateTemp(s.attachmentDir, "incoming-*")
	if err != nil {
		return Attachment{}, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	hash := sha256.New()
	limited := io.LimitReader(source, maxAttachmentBytes+1)
	written, copyErr := io.Copy(io.MultiWriter(temp, hash), limited)
	closeErr := temp.Close()
	if copyErr != nil {
		return Attachment{}, copyErr
	}
	if closeErr != nil {
		return Attachment{}, closeErr
	}
	if written != size || written > maxAttachmentBytes {
		return Attachment{}, errors.New("附件读取不完整或超过 25MB")
	}
	probe, err := os.Open(tempPath)
	if err != nil {
		return Attachment{}, err
	}
	buffer := make([]byte, 512)
	n, _ := probe.Read(buffer)
	probe.Close()
	mimeType := attachmentMime(fileName, buffer[:n])
	if err := validateAttachment(fileName, mimeType, written); err != nil {
		return Attachment{}, err
	}
	hashValue := fmt.Sprintf("%x", hash.Sum(nil))
	ext := strings.ToLower(filepath.Ext(fileName))
	directory := filepath.Join(s.attachmentDir, note.NotebookID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Attachment{}, err
	}
	destination := filepath.Join(directory, hashValue+ext)
	if _, err := os.Stat(destination); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(tempPath, destination); err != nil {
			return Attachment{}, err
		}
	}
	now := nowString()
	item := Attachment{ID: uuid.NewString(), NotebookID: note.NotebookID, FileName: fileName, MimeType: mimeType, Size: written, SHA256: hashValue, Status: "local", CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Attachment{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO attachments(id, notebook_id, file_name, mime_type, size_bytes, sha256, local_path, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'local', ?, ?)`, item.ID, item.NotebookID, item.FileName, item.MimeType, item.Size, item.SHA256, destination, now, now); err != nil {
		return Attachment{}, err
	}
	var position int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM note_attachments WHERE note_id = ?`, noteID).Scan(&position); err != nil {
		return Attachment{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO note_attachments(note_id, attachment_id, position) VALUES (?, ?, ?)`, noteID, item.ID, position); err != nil {
		return Attachment{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET revision = revision + 1, updated_at = ? WHERE id = ?`, now, noteID); err != nil {
		return Attachment{}, err
	}
	if err := saveNoteVersionTx(ctx, tx, noteID, note.Title, note.Content, note.Revision+1, true); err != nil {
		return Attachment{}, err
	}
	if err := enqueueOutboxTx(ctx, tx, "attachment", item.ID, "upsert", 0, item); err != nil {
		return Attachment{}, err
	}
	notePayload, err := noteSyncPayloadTx(ctx, tx, noteID)
	if err != nil {
		return Attachment{}, err
	}
	if err := enqueueOutboxTx(ctx, tx, "note", noteID, "upsert", note.ServerVersion, notePayload); err != nil {
		return Attachment{}, err
	}
	if err := enqueueLatestNoteVersionTx(ctx, tx, noteID); err != nil {
		return Attachment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Attachment{}, err
	}
	return item, nil
}

func (s *Store) ReadAttachment(ctx context.Context, id string) (AttachmentContent, error) {
	item, path, err := s.attachmentPath(ctx, id)
	if err != nil {
		return AttachmentContent{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AttachmentContent{}, fmt.Errorf("读取附件失败: %w", err)
	}
	return AttachmentContent{MimeType: item.MimeType, Data: base64.StdEncoding.EncodeToString(data)}, nil
}

func (s *Store) DeleteAttachment(ctx context.Context, noteID, id string) error {
	note, err := s.GetNote(ctx, noteID)
	if err != nil {
		return err
	}
	attachment, _, err := s.attachmentPath(ctx, id)
	if err != nil {
		return err
	}
	now := nowString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM note_attachments WHERE note_id = ? AND attachment_id = ?`, noteID, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("附件不属于这篇笔记")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE attachments SET deleted_at = ?, updated_at = ? WHERE id = ? AND NOT EXISTS
		(SELECT 1 FROM note_attachments WHERE attachment_id = ?)`, now, now, id, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET revision = revision + 1, updated_at = ? WHERE id = ?`, now, noteID); err != nil {
		return err
	}
	if err := saveNoteVersionTx(ctx, tx, noteID, note.Title, note.Content, note.Revision+1, true); err != nil {
		return err
	}
	attachment.DeletedAt, attachment.UpdatedAt = now, now
	if err := enqueueOutboxTx(ctx, tx, "attachment", id, "delete", 0, attachment); err != nil {
		return err
	}
	notePayload, err := noteSyncPayloadTx(ctx, tx, noteID)
	if err != nil {
		return err
	}
	if err := enqueueOutboxTx(ctx, tx, "note", noteID, "upsert", note.ServerVersion, notePayload); err != nil {
		return err
	}
	if err := enqueueLatestNoteVersionTx(ctx, tx, noteID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PurgeOrphanAttachments(ctx context.Context) error {
	cutoff := nowString()
	rows, err := s.db.QueryContext(ctx, `SELECT id, local_path FROM attachments a WHERE a.deleted_at IS NOT NULL AND a.deleted_at < datetime(?, '-7 days')
		AND NOT EXISTS (SELECT 1 FROM note_attachments na WHERE na.attachment_id = a.id)
		AND NOT EXISTS (SELECT 1 FROM note_versions nv WHERE nv.attachment_ids LIKE '%' || a.id || '%')`, cutoff)
	if err != nil {
		return err
	}
	type orphan struct{ id, path string }
	items := []orphan{}
	for rows.Next() {
		var item orphan
		if err := rows.Scan(&item.id, &item.path); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, item.id); err != nil {
			return err
		}
		_ = os.Remove(item.path)
	}
	return nil
}
