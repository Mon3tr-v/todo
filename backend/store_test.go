package backend

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "todo-test.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func mustCreateTask(t *testing.T, store *Store, input CreateTaskInput) Task {
	t.Helper()
	task, err := store.CreateTask(context.Background(), input)
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	return task
}

func TestStoreSeedsListsAndSettings(t *testing.T) {
	store := newTestStore(t)
	bootstrap, err := store.GetBootstrap(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("GetBootstrap() error = %v", err)
	}
	if len(bootstrap.Lists) != 2 {
		t.Fatalf("lists length = %d, want 2", len(bootstrap.Lists))
	}
	if bootstrap.Lists[0].Name != "个人" || bootstrap.Lists[1].Name != "工作" {
		t.Fatalf("unexpected lists: %#v", bootstrap.Lists)
	}
	if bootstrap.Settings.Theme != "system" || bootstrap.Settings.StartupView != "today" {
		t.Fatalf("unexpected settings: %#v", bootstrap.Settings)
	}
	if bootstrap.Counts != (SmartCounts{}) {
		t.Fatalf("unexpected counts: %#v", bootstrap.Counts)
	}
	if len(bootstrap.ListCounts) != 0 || len(bootstrap.TagCounts) != 0 {
		t.Fatalf("unexpected collection counts: %#v, %#v", bootstrap.ListCounts, bootstrap.TagCounts)
	}
}

func TestTaskCRUDAndSmartViews(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	lists, _ := store.ListLists(ctx)
	tag, err := store.CreateTag(ctx, TagInput{Name: "重要", Color: "rose"})
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	overdue := now.AddDate(0, 0, -2).Format("2006-01-02")
	upcoming := now.AddDate(0, 0, 3).Format("2006-01-02")

	todayTask := mustCreateTask(t, store, CreateTaskInput{Title: "今天完成", ListID: lists[0].ID, Priority: "high", DueDate: today, TagIDs: []string{tag.ID}})
	overdueTask := mustCreateTask(t, store, CreateTaskInput{Title: "补交资料", Priority: "medium", DueDate: overdue})
	mustCreateTask(t, store, CreateTaskInput{Title: "准备下周", DueDate: upcoming})
	bootstrap, err := store.GetBootstrap(ctx, now)
	if err != nil {
		t.Fatalf("GetBootstrap() error = %v", err)
	}
	if bootstrap.Counts.All != 3 || bootstrap.ListCounts[lists[0].ID] != 1 || bootstrap.TagCounts[tag.ID] != 1 {
		t.Fatalf("unexpected collection counts: %#v", bootstrap)
	}

	todayTasks, err := store.ListTasks(ctx, TaskQuery{View: "today", Priority: "all", Sort: "due"}, now)
	if err != nil {
		t.Fatalf("ListTasks(today) error = %v", err)
	}
	if len(todayTasks) != 2 || todayTasks[0].ID != overdueTask.ID {
		t.Fatalf("today tasks = %#v", todayTasks)
	}
	upcomingTasks, _ := store.ListTasks(ctx, TaskQuery{View: "upcoming", Priority: "all", Sort: "due"}, now)
	if len(upcomingTasks) != 1 || upcomingTasks[0].Title != "准备下周" {
		t.Fatalf("unexpected upcoming tasks: %#v", upcomingTasks)
	}
	tagged, _ := store.ListTasks(ctx, TaskQuery{View: "all", TagID: tag.ID, Priority: "all", Sort: "due"}, now)
	if len(tagged) != 1 || tagged[0].ID != todayTask.ID {
		t.Fatalf("unexpected tagged tasks: %#v", tagged)
	}
	searched, _ := store.ListTasks(ctx, TaskQuery{View: "all", Search: "补交", Priority: "all", Sort: "due"}, now)
	if len(searched) != 1 || searched[0].ID != overdueTask.ID {
		t.Fatalf("unexpected search tasks: %#v", searched)
	}

	updated, err := store.UpdateTask(ctx, UpdateTaskInput{ID: todayTask.ID, Title: "今天完成更新", Notes: "详细备注", ListID: lists[1].ID, Priority: "low", DueDate: upcoming, TagIDs: []string{tag.ID}})
	if err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}
	if updated.Title != "今天完成更新" || updated.ListID != lists[1].ID || len(updated.Tags) != 1 {
		t.Fatalf("unexpected updated task: %#v", updated)
	}

	if _, err := store.SetTaskCompleted(ctx, updated.ID, true); err != nil {
		t.Fatalf("SetTaskCompleted() error = %v", err)
	}
	completed, _ := store.ListTasks(ctx, TaskQuery{View: "completed", Priority: "all", Sort: "created"}, now)
	if len(completed) != 1 || completed[0].ID != updated.ID {
		t.Fatalf("unexpected completed tasks: %#v", completed)
	}
	bootstrap, err = store.GetBootstrap(ctx, now)
	if err != nil {
		t.Fatalf("GetBootstrap() error = %v", err)
	}
	if bootstrap.Counts.All != 2 || bootstrap.ListCounts[lists[1].ID] != 0 || bootstrap.TagCounts[tag.ID] != 0 {
		t.Fatalf("completed task remained in collection counts: %#v", bootstrap)
	}
	allStatuses, _ := store.ListTasks(ctx, TaskQuery{View: "all", Status: "all", Priority: "all", Sort: "created"}, now)
	if len(allStatuses) != 3 {
		t.Fatalf("all statuses length = %d, want 3", len(allStatuses))
	}

	if err := store.DeleteTask(ctx, overdueTask.ID); err != nil {
		t.Fatalf("DeleteTask() error = %v", err)
	}
	inbox, _ := store.ListTasks(ctx, TaskQuery{View: "inbox", Priority: "all", Sort: "due"}, now)
	if len(inbox) != 1 || inbox[0].ID == overdueTask.ID {
		t.Fatalf("deleted task remained in inbox: %#v", inbox)
	}
	if _, err := store.RestoreTask(ctx, overdueTask.ID); err != nil {
		t.Fatalf("RestoreTask() error = %v", err)
	}
	inbox, _ = store.ListTasks(ctx, TaskQuery{View: "inbox", Priority: "all", Sort: "due"}, now)
	if len(inbox) != 2 || inbox[0].ID != overdueTask.ID {
		t.Fatalf("restored inbox = %#v", inbox)
	}
}

func TestListAndTagDeletionPreservesTasks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	list, _ := store.CreateList(ctx, "学习")
	tag, _ := store.CreateTag(ctx, TagInput{Name: "阅读", Color: "blue"})
	task := mustCreateTask(t, store, CreateTaskInput{Title: "读一章", ListID: list.ID, Priority: "none", TagIDs: []string{tag.ID}})
	if err := store.DeleteList(ctx, list.ID); err != nil {
		t.Fatalf("DeleteList() error = %v", err)
	}
	if err := store.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag() error = %v", err)
	}
	loaded, err := store.getTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("getTask() error = %v", err)
	}
	if loaded.ListID != "" || len(loaded.Tags) != 0 {
		t.Fatalf("task references were not cleared: %#v", loaded)
	}
}

func TestOverviewAndCounts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	mustCreateTask(t, store, CreateTaskInput{Title: "逾期", Priority: "none", DueDate: now.AddDate(0, 0, -1).Format("2006-01-02")})
	done := mustCreateTask(t, store, CreateTaskInput{Title: "完成", Priority: "none", DueDate: now.Format("2006-01-02")})
	if _, err := store.SetTaskCompleted(ctx, done.ID, true); err != nil {
		t.Fatal(err)
	}
	overview, err := store.GetOverview(ctx, now)
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if overview.Open != 1 || overview.Overdue != 1 || overview.CompletedToday != 1 {
		t.Fatalf("unexpected overview: %#v", overview)
	}
	if len(overview.Week) != 7 || overview.Week[6].Count != 1 {
		t.Fatalf("unexpected week: %#v", overview.Week)
	}
	counts, err := store.GetSmartCounts(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Today != 1 || counts.All != 1 || counts.Completed != 1 || counts.Inbox != 1 {
		t.Fatalf("unexpected counts: %#v", counts)
	}
}

func TestBackupRoundTripAndInvalidRollback(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	lists, _ := store.ListLists(ctx)
	tag, _ := store.CreateTag(ctx, TagInput{Name: "备份", Color: "mint"})
	original := mustCreateTask(t, store, CreateTaskInput{Title: "保留我", ListID: lists[0].ID, Priority: "high", TagIDs: []string{tag.ID}})
	backup, err := store.ExportSnapshot(ctx)
	if err != nil {
		t.Fatalf("ExportSnapshot() error = %v", err)
	}
	if len(backup.Tasks) != 1 || backup.SchemaVersion != 2 {
		t.Fatalf("unexpected backup: %#v", backup)
	}
	if err := store.DeleteTask(ctx, original.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ImportSnapshot(ctx, backup); err != nil {
		t.Fatalf("ImportSnapshot() error = %v", err)
	}
	restored, err := store.getTask(ctx, original.ID)
	if err != nil || restored.Title != original.Title || len(restored.Tags) != 1 {
		t.Fatalf("restored task = %#v, err = %v", restored, err)
	}

	invalid := backup
	invalid.Tasks = append(invalid.Tasks, invalid.Tasks[0])
	if err := store.ImportSnapshot(ctx, invalid); err == nil {
		t.Fatal("ImportSnapshot() accepted duplicate task")
	}
	stillThere, err := store.getTask(ctx, original.ID)
	if err != nil || stillThere.Title != original.Title {
		t.Fatalf("invalid import changed data: %#v, %v", stillThere, err)
	}
}

func TestValidation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateTask(ctx, CreateTaskInput{Title: "  ", Priority: "none"}); err == nil {
		t.Fatal("empty task title was accepted")
	}
	if _, err := store.CreateTask(ctx, CreateTaskInput{Title: "任务", Priority: "urgent"}); err == nil {
		t.Fatal("invalid priority was accepted")
	}
	if _, err := store.CreateTask(ctx, CreateTaskInput{Title: "任务", Priority: "none", DueDate: "13-07-2026"}); err == nil {
		t.Fatal("invalid date was accepted")
	}
	if _, err := store.UpdateSettings(ctx, AppSettings{Theme: "neon", StartupView: "today"}); err == nil {
		t.Fatal("invalid theme was accepted")
	}
}

func TestMigrationFromVersionOnePreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo-v1.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`INSERT INTO schema_migrations(version,applied_at) VALUES(1,'2026-01-01T00:00:00Z')`,
		`CREATE TABLE lists(id TEXT PRIMARY KEY,name TEXT NOT NULL,position INTEGER NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE tags(id TEXT PRIMARY KEY,name TEXT NOT NULL,color TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE tasks(id TEXT PRIMARY KEY,title TEXT NOT NULL,notes TEXT NOT NULL DEFAULT '',list_id TEXT REFERENCES lists(id) ON DELETE SET NULL,priority TEXT NOT NULL,due_date TEXT,completed_at TEXT,deleted_at TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE task_tags(task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,PRIMARY KEY(task_id,tag_id))`,
		`CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL)`,
		`INSERT INTO settings(key,value) VALUES('theme','dark')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	db.Close()
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore(v1) error = %v", err)
	}
	defer store.Close()
	var version int
	if err := store.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version = %d, err = %v", version, err)
	}
	bootstrap, err := store.GetBootstrap(context.Background(), time.Now())
	if err != nil || bootstrap.Settings.Theme != "dark" || len(bootstrap.Notebooks) != 1 || !bootstrap.Notebooks[0].Default {
		t.Fatalf("unexpected migrated bootstrap: %#v, err = %v", bootstrap, err)
	}
}

func TestNotesAttachmentsVersionsCommentsAndArchiveRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	bootstrap, _ := store.GetBootstrap(ctx, time.Now())
	notebookID := bootstrap.Notebooks[0].ID
	tag, err := store.CreateNoteTag(ctx, NoteTagInput{NotebookID: notebookID, Name: "方案", Color: "blue"})
	if err != nil {
		t.Fatal(err)
	}
	task := mustCreateTask(t, store, CreateTaskInput{Title: "关联任务", Priority: "none"})
	note, err := store.CreateNote(ctx, CreateNoteInput{NotebookID: notebookID, Title: "同步设计", Content: "初稿", TagIDs: []string{tag.ID}, TaskIDs: []string{task.ID}})
	if err != nil {
		t.Fatal(err)
	}
	note, err = store.UpdateNote(ctx, UpdateNoteInput{ID: note.ID, NotebookID: notebookID, Title: "同步设计", Content: "第二稿", TagIDs: []string{tag.ID}, TaskIDs: []string{task.ID}, BaseRevision: note.Revision})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := store.ListNoteVersions(ctx, note.ID)
	if err != nil || len(versions) != 1 || versions[0].Content != "第二稿" {
		t.Fatalf("coalesced versions = %#v, err = %v", versions, err)
	}
	attachment, err := store.AddAttachmentData(ctx, AttachmentUploadInput{NoteID: note.ID, FileName: "说明.txt", MimeType: "text/plain", Data: base64.StdEncoding.EncodeToString([]byte("archive attachment"))})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := store.CreateComment(ctx, CommentInput{NoteID: note.ID, Content: "请 @teammate 检查"})
	if err != nil || comment.ID == "" {
		t.Fatalf("CreateComment() = %#v, %v", comment, err)
	}
	if _, err := store.SetNotePinned(ctx, note.ID, true); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListNotes(ctx, NoteQuery{NotebookID: notebookID, Search: "第二稿", PinnedOnly: true})
	if err != nil || len(items) != 1 || items[0].AttachmentCount != 1 || items[0].LinkedTaskCount != 1 {
		t.Fatalf("ListNotes() = %#v, %v", items, err)
	}
	backup, err := store.ExportSnapshot(ctx)
	if err != nil || len(backup.Notes) != 1 || len(backup.Attachments) != 1 || len(backup.Versions) < 2 || len(backup.Comments) != 1 {
		t.Fatalf("ExportSnapshot() = %#v, %v", backup, err)
	}
	archivePath := filepath.Join(t.TempDir(), "notes.todo-backup.zip")
	if err := writeBackupArchive(archivePath, backup, store); err != nil {
		t.Fatal(err)
	}
	restored := newTestStore(t)
	incoming, paths, err := readBackupArchive(archivePath, restored)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.ImportSnapshotWithAttachments(ctx, incoming, paths); err != nil {
		t.Fatal(err)
	}
	content, err := restored.ReadAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := base64.StdEncoding.DecodeString(content.Data)
	if string(decoded) != "archive attachment" {
		t.Fatalf("restored attachment = %q", decoded)
	}
}

func TestRemoteNoteSnapshotKeepsPrivateTaskLinks(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	bootstrap, _ := store.GetBootstrap(ctx, time.Now())
	notebookID := bootstrap.Notebooks[0].ID
	task := mustCreateTask(t, store, CreateTaskInput{Title: "私有任务", Priority: "none"})
	note, err := store.CreateNote(ctx, CreateNoteInput{NotebookID: notebookID, Title: "共享内容", Content: "本地", TaskIDs: []string{task.ID}})
	if err != nil {
		t.Fatal(err)
	}
	attachment := Attachment{ID: "remote-attachment", NotebookID: notebookID, FileName: "远端.txt", MimeType: "text/plain", Size: 12, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CreatedAt: nowString(), UpdatedAt: nowString()}
	attachmentJSON, _ := json.Marshal(attachment)
	noteJSON, _ := json.Marshal(BackupNote{ID: note.ID, NotebookID: notebookID, Title: "共享内容", Content: "远端正文", Revision: note.Revision + 1, AttachmentIDs: []string{attachment.ID}, CreatedAt: note.CreatedAt, UpdatedAt: nowString()})
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyRemoteChangeTx(ctx, tx, "attachment", attachment.ID, "upsert", 1, attachmentJSON, store.attachmentDir); err != nil {
		t.Fatal(err)
	}
	if err := applyRemoteChangeTx(ctx, tx, "note", note.ID, "upsert", 2, noteJSON, store.attachmentDir); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetNote(ctx, note.ID)
	if err != nil || updated.Content != "远端正文" || len(updated.Attachments) != 1 || len(updated.Tasks) != 1 || updated.Tasks[0].ID != task.ID {
		t.Fatalf("remote note = %#v, err = %v", updated, err)
	}
	if _, err := os.Stat(filepath.Join(store.attachmentDir, notebookID)); !os.IsNotExist(err) {
		t.Fatalf("remote metadata unexpectedly created attachment bytes: %v", err)
	}
}
