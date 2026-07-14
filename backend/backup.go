package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const backupSchemaVersion = 2

func (s *Store) ExportSnapshot(ctx context.Context) (BackupEnvelope, error) {
	lists, err := s.ListLists(ctx)
	if err != nil {
		return BackupEnvelope{}, err
	}
	tags, err := s.ListTags(ctx)
	if err != nil {
		return BackupEnvelope{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE deleted_at IS NULL ORDER BY created_at`)
	if err != nil {
		return BackupEnvelope{}, err
	}
	tasks := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			rows.Close()
			return BackupEnvelope{}, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Close(); err != nil {
		return BackupEnvelope{}, err
	}
	for i := range tasks {
		tasks[i].Tags, err = s.tagsForTask(ctx, tasks[i].ID)
		if err != nil {
			return BackupEnvelope{}, err
		}
	}

	notebooks, err := s.ListNotebooks(ctx)
	if err != nil {
		return BackupEnvelope{}, err
	}
	noteTags, err := s.ListNoteTags(ctx, "")
	if err != nil {
		return BackupEnvelope{}, err
	}
	noteSummaries, err := s.ListNotes(ctx, NoteQuery{})
	if err != nil {
		return BackupEnvelope{}, err
	}
	notes := make([]BackupNote, 0, len(noteSummaries))
	attachmentMap := map[string]Attachment{}
	versions := []BackupNoteVersion{}
	comments := []Comment{}
	for _, summary := range noteSummaries {
		note, err := s.GetNote(ctx, summary.ID)
		if err != nil {
			return BackupEnvelope{}, err
		}
		backupNote := BackupNote{
			ID: note.ID, NotebookID: note.NotebookID, Title: note.Title, Content: note.Content, Pinned: note.Pinned,
			Revision: note.Revision, CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt,
		}
		for _, tag := range note.Tags {
			backupNote.TagIDs = append(backupNote.TagIDs, tag.ID)
		}
		for _, task := range note.Tasks {
			backupNote.TaskIDs = append(backupNote.TaskIDs, task.ID)
		}
		for _, attachment := range note.Attachments {
			backupNote.AttachmentIDs = append(backupNote.AttachmentIDs, attachment.ID)
			attachmentMap[attachment.ID] = attachment
		}
		notes = append(notes, backupNote)

		versionRows, err := s.db.QueryContext(ctx, `SELECT id, note_id, title, content, tag_ids, attachment_ids, actor_id, actor_name, source_revision, created_at
			FROM note_versions WHERE note_id = ? ORDER BY created_at`, note.ID)
		if err != nil {
			return BackupEnvelope{}, err
		}
		for versionRows.Next() {
			var item BackupNoteVersion
			item.NotebookID = note.NotebookID
			var tagJSON, attachmentJSON string
			if err := versionRows.Scan(&item.ID, &item.NoteID, &item.Title, &item.Content, &tagJSON, &attachmentJSON,
				&item.ActorID, &item.ActorName, &item.SourceRevision, &item.CreatedAt); err != nil {
				versionRows.Close()
				return BackupEnvelope{}, err
			}
			_ = json.Unmarshal([]byte(tagJSON), &item.TagIDs)
			_ = json.Unmarshal([]byte(attachmentJSON), &item.AttachmentIDs)
			versions = append(versions, item)
		}
		if err := versionRows.Close(); err != nil {
			return BackupEnvelope{}, err
		}
		noteComments, err := s.ListComments(ctx, note.ID)
		if err != nil {
			return BackupEnvelope{}, err
		}
		for _, comment := range noteComments {
			if comment.DeletedAt == "" {
				comments = append(comments, comment)
			}
		}
	}
	attachments := make([]Attachment, 0, len(attachmentMap))
	for _, item := range attachmentMap {
		attachments = append(attachments, item)
	}
	memberships := []NotebookMember{}
	for _, notebook := range notebooks {
		items, err := s.ListNotebookMembers(ctx, notebook.ID)
		if err != nil {
			return BackupEnvelope{}, err
		}
		memberships = append(memberships, items...)
	}

	settings := map[string]string{}
	settingRows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return BackupEnvelope{}, err
	}
	for settingRows.Next() {
		var key, value string
		if err := settingRows.Scan(&key, &value); err != nil {
			settingRows.Close()
			return BackupEnvelope{}, err
		}
		settings[key] = value
	}
	if err := settingRows.Close(); err != nil {
		return BackupEnvelope{}, err
	}
	return BackupEnvelope{
		SchemaVersion: backupSchemaVersion, ExportedAt: nowString(), Lists: lists, Tags: tags, Tasks: tasks,
		Notebooks: notebooks, NoteTags: noteTags, Notes: notes, Attachments: attachments, Versions: versions,
		Comments: comments, Memberships: memberships, Settings: settings,
	}, nil
}

func validateTimestamp(value, label string, optional bool) error {
	if value == "" && optional {
		return nil
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return fmt.Errorf("%s时间格式无效", label)
	}
	return nil
}

func validateSnapshot(backup BackupEnvelope) error {
	if backup.SchemaVersion != 1 && backup.SchemaVersion != backupSchemaVersion {
		return fmt.Errorf("不支持的备份版本: %d", backup.SchemaVersion)
	}
	listIDs, listNames := map[string]bool{}, map[string]bool{}
	for _, list := range backup.Lists {
		name := strings.TrimSpace(list.Name)
		if list.ID == "" || name == "" || listIDs[list.ID] || listNames[strings.ToLower(name)] {
			return errors.New("备份中包含无效或重复清单")
		}
		listIDs[list.ID], listNames[strings.ToLower(name)] = true, true
		if err := validateTimestamp(list.CreatedAt, "清单创建", false); err != nil {
			return err
		}
		if err := validateTimestamp(list.UpdatedAt, "清单更新", false); err != nil {
			return err
		}
	}
	tagIDs, tagNames := map[string]bool{}, map[string]bool{}
	for _, tag := range backup.Tags {
		name := strings.TrimSpace(tag.Name)
		if tag.ID == "" || name == "" || tagIDs[tag.ID] || tagNames[strings.ToLower(name)] {
			return errors.New("备份中包含无效或重复标签")
		}
		tagIDs[tag.ID], tagNames[strings.ToLower(name)] = true, true
		if err := validateTimestamp(tag.CreatedAt, "标签创建", false); err != nil {
			return err
		}
		if err := validateTimestamp(tag.UpdatedAt, "标签更新", false); err != nil {
			return err
		}
	}
	taskIDs := map[string]bool{}
	for _, task := range backup.Tasks {
		if task.ID == "" || strings.TrimSpace(task.Title) == "" || taskIDs[task.ID] || !validPriority(task.Priority) {
			return errors.New("备份中包含无效或重复任务")
		}
		taskIDs[task.ID] = true
		if err := validateDate(task.DueDate); err != nil {
			return err
		}
		if task.ListID != "" && !listIDs[task.ListID] {
			return errors.New("备份任务引用了不存在的清单")
		}
		for _, value := range []struct{ value, label string }{{task.CreatedAt, "任务创建"}, {task.UpdatedAt, "任务更新"}} {
			if err := validateTimestamp(value.value, value.label, false); err != nil {
				return err
			}
		}
		if err := validateTimestamp(task.CompletedAt, "任务完成", true); err != nil {
			return err
		}
		seenTags := map[string]bool{}
		for _, tag := range task.Tags {
			if !tagIDs[tag.ID] || seenTags[tag.ID] {
				return errors.New("备份任务引用了不存在或重复的标签")
			}
			seenTags[tag.ID] = true
		}
	}
	if backup.SchemaVersion == 1 {
		return validateBackupSettings(backup.Settings)
	}
	notebookIDs := map[string]bool{}
	defaultCount := 0
	for _, notebook := range backup.Notebooks {
		if notebook.ID == "" || strings.TrimSpace(notebook.Name) == "" || notebookIDs[notebook.ID] {
			return errors.New("备份中包含无效或重复笔记本")
		}
		notebookIDs[notebook.ID] = true
		if notebook.Default {
			defaultCount++
		}
		if notebook.Role != "owner" && notebook.Role != "editor" && notebook.Role != "viewer" {
			return errors.New("备份中包含无效笔记本权限")
		}
		if err := validateTimestamp(notebook.CreatedAt, "笔记本创建", false); err != nil {
			return err
		}
		if err := validateTimestamp(notebook.UpdatedAt, "笔记本更新", false); err != nil {
			return err
		}
	}
	if len(backup.Notebooks) > 0 && defaultCount != 1 {
		return errors.New("备份必须包含一个默认笔记本")
	}
	noteTagIDs := map[string]string{}
	for _, tag := range backup.NoteTags {
		if tag.ID == "" || !notebookIDs[tag.NotebookID] || noteTagIDs[tag.ID] != "" {
			return errors.New("备份中包含无效笔记标签")
		}
		noteTagIDs[tag.ID] = tag.NotebookID
	}
	attachmentIDs := map[string]bool{}
	for _, attachment := range backup.Attachments {
		if attachment.ID == "" || !notebookIDs[attachment.NotebookID] || attachmentIDs[attachment.ID] || attachment.Size <= 0 || attachment.Size > maxAttachmentBytes {
			return errors.New("备份中包含无效附件")
		}
		if err := validateAttachment(attachment.FileName, attachment.MimeType, attachment.Size); err != nil {
			return err
		}
		attachmentIDs[attachment.ID] = true
	}
	noteIDs := map[string]bool{}
	for _, note := range backup.Notes {
		if note.ID == "" || !notebookIDs[note.NotebookID] || noteIDs[note.ID] {
			return errors.New("备份中包含无效或重复笔记")
		}
		if _, err := normalizeNoteTitle(note.Title); err != nil {
			return err
		}
		noteIDs[note.ID] = true
		for _, id := range uniqueStrings(note.TagIDs) {
			if noteTagIDs[id] != note.NotebookID {
				return errors.New("备份笔记引用了其他笔记本的标签")
			}
		}
		for _, id := range uniqueStrings(note.TaskIDs) {
			if !taskIDs[id] {
				return errors.New("备份笔记引用了不存在的任务")
			}
		}
		for _, id := range uniqueStrings(note.AttachmentIDs) {
			if !attachmentIDs[id] {
				return errors.New("备份笔记引用了不存在的附件")
			}
		}
		if err := validateTimestamp(note.CreatedAt, "笔记创建", false); err != nil {
			return err
		}
		if err := validateTimestamp(note.UpdatedAt, "笔记更新", false); err != nil {
			return err
		}
	}
	for _, version := range backup.Versions {
		if version.ID == "" || !noteIDs[version.NoteID] {
			return errors.New("备份中包含无效历史版本")
		}
		if err := validateTimestamp(version.CreatedAt, "版本创建", false); err != nil {
			return err
		}
	}
	for _, comment := range backup.Comments {
		if comment.ID == "" || !noteIDs[comment.NoteID] || strings.TrimSpace(comment.Content) == "" {
			return errors.New("备份中包含无效评论")
		}
	}
	return validateBackupSettings(backup.Settings)
}

func validateBackupSettings(settings map[string]string) error {
	if theme := settings["theme"]; theme != "" && theme != "system" && theme != "light" && theme != "dark" {
		return errors.New("备份中的主题设置无效")
	}
	return nil
}

func (s *Store) ImportSnapshot(ctx context.Context, backup BackupEnvelope) error {
	return s.ImportSnapshotWithAttachments(ctx, backup, nil)
}

func (s *Store) ImportSnapshotWithAttachments(ctx context.Context, backup BackupEnvelope, attachmentPaths map[string]string) error {
	if err := validateSnapshot(backup); err != nil {
		return err
	}
	if len(backup.Attachments) > 0 && len(attachmentPaths) == 0 {
		return errors.New("备份缺少附件文件")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{
		"edit_leases", "mentions", "comments", "note_versions", "note_attachments", "attachments", "note_task_links", "note_tag_links",
		"notes", "memberships", "note_tags", "notebooks", "task_tags", "tasks", "tags", "lists", "settings", "outbox", "tombstones", "notifications",
	} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return err
		}
	}
	for _, list := range backup.Lists {
		if _, err := tx.ExecContext(ctx, `INSERT INTO lists(id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			list.ID, strings.TrimSpace(list.Name), list.Position, list.CreatedAt, list.UpdatedAt); err != nil {
			return err
		}
	}
	for _, tag := range backup.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags(id, name, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			tag.ID, strings.TrimSpace(tag.Name), normalizeTagColor(tag.Color), tag.CreatedAt, tag.UpdatedAt); err != nil {
			return err
		}
	}
	for _, task := range backup.Tasks {
		var listID, dueDate, completedAt any
		if task.ListID != "" {
			listID = task.ListID
		}
		if task.DueDate != "" {
			dueDate = task.DueDate
		}
		if task.CompletedAt != "" {
			completedAt = task.CompletedAt
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tasks(id, title, notes, list_id, priority, due_date, completed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, task.ID, strings.TrimSpace(task.Title), task.Notes, listID, task.Priority, dueDate, completedAt, task.CreatedAt, task.UpdatedAt); err != nil {
			return err
		}
		for _, tag := range task.Tags {
			if _, err := tx.ExecContext(ctx, `INSERT INTO task_tags(task_id, tag_id) VALUES (?, ?)`, task.ID, tag.ID); err != nil {
				return err
			}
		}
	}
	if backup.SchemaVersion == backupSchemaVersion {
		if err := importNotesTx(ctx, tx, backup, attachmentPaths); err != nil {
			return err
		}
	}
	settings := map[string]string{"theme": "system", "startupView": "today"}
	for key, value := range backup.Settings {
		settings[key] = value
	}
	for key, value := range settings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, ?)`, key, value); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if backup.SchemaVersion == 1 || len(backup.Notebooks) == 0 {
		return s.seed(ctx)
	}
	return nil
}

func importNotesTx(ctx context.Context, tx *sql.Tx, backup BackupEnvelope, attachmentPaths map[string]string) error {
	for _, notebook := range backup.Notebooks {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notebooks(id, name, kind, role, owner_id, is_default, server_version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, notebook.ID, notebook.Name, notebook.Kind, notebook.Role, notebook.OwnerID, boolInt(notebook.Default), notebook.ServerVersion, notebook.CreatedAt, notebook.UpdatedAt); err != nil {
			return err
		}
	}
	for _, tag := range backup.NoteTags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_tags(id, notebook_id, name, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			tag.ID, tag.NotebookID, tag.Name, normalizeTagColor(tag.Color), tag.CreatedAt, tag.UpdatedAt); err != nil {
			return err
		}
	}
	for _, attachment := range backup.Attachments {
		path := attachmentPaths[attachment.ID]
		if filepath.Clean(path) == "." || path == "" {
			return errors.New("备份附件路径无效")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO attachments(id, notebook_id, file_name, mime_type, size_bytes, sha256, local_path, remote_key, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'local', ?, ?)`, attachment.ID, attachment.NotebookID, attachment.FileName, attachment.MimeType,
			attachment.Size, attachment.SHA256, path, attachment.RemoteKey, attachment.CreatedAt, attachment.UpdatedAt); err != nil {
			return err
		}
	}
	for _, note := range backup.Notes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notes(id, notebook_id, title, content, pinned, revision, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, note.ID, note.NotebookID, note.Title, note.Content, boolInt(note.Pinned), maxInt64(note.Revision, 1), note.CreatedAt, note.UpdatedAt); err != nil {
			return err
		}
		for _, id := range uniqueStrings(note.TagIDs) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO note_tag_links(note_id, tag_id) VALUES (?, ?)`, note.ID, id); err != nil {
				return err
			}
		}
		for _, id := range uniqueStrings(note.TaskIDs) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO note_task_links(note_id, task_id, user_id) VALUES (?, ?, ?)`, note.ID, id, localUserID); err != nil {
				return err
			}
		}
		for position, id := range note.AttachmentIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO note_attachments(note_id, attachment_id, position) VALUES (?, ?, ?)`, note.ID, id, position); err != nil {
				return err
			}
		}
	}
	for _, version := range backup.Versions {
		tags, _ := json.Marshal(version.TagIDs)
		attachments, _ := json.Marshal(version.AttachmentIDs)
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_versions(id, note_id, title, content, tag_ids, attachment_ids, actor_id, actor_name, source_revision, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, version.ID, version.NoteID, version.Title, version.Content, string(tags), string(attachments),
			version.ActorID, version.ActorName, version.SourceRevision, version.CreatedAt); err != nil {
			return err
		}
	}
	for _, comment := range backup.Comments {
		var parent any
		if comment.ParentID != "" {
			parent = comment.ParentID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO comments(id, note_id, parent_id, author_id, author_name, content, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, comment.ID, comment.NoteID, parent, comment.AuthorID, comment.AuthorName, comment.Content, comment.CreatedAt, comment.UpdatedAt); err != nil {
			return err
		}
	}
	for _, member := range backup.Memberships {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO memberships(notebook_id, user_id, username, role, created_at) VALUES (?, ?, ?, ?, ?)`,
			member.NotebookID, member.UserID, member.Username, member.Role, member.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}
