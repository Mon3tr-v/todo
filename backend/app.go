package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx        context.Context
	store      *Store
	startupErr error
	dataDir    string
	mu         sync.RWMutex
	syncClient *SyncClient
	syncCancel context.CancelFunc
	windowMu   sync.Mutex
	window     windowState
}

type windowState struct {
	compact      bool
	width        int
	height       int
	x            int
	y            int
	wasMaximised bool
}

func NewApp() *App { return &App{} }

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	configDir, err := os.UserConfigDir()
	if err != nil {
		a.startupErr = fmt.Errorf("获取数据目录失败: %w", err)
		return
	}
	a.dataDir = filepath.Join(configDir, "TODO")
	if err := os.MkdirAll(a.dataDir, 0o755); err != nil {
		a.startupErr = fmt.Errorf("创建数据目录失败: %w", err)
		return
	}
	a.store, err = OpenStore(filepath.Join(a.dataDir, "todo.db"))
	if err != nil {
		a.startupErr = fmt.Errorf("打开任务数据库失败: %w", err)
		return
	}
	_ = a.store.PurgeDeleted(ctx, time.Now().AddDate(0, 0, -30))
	_ = a.store.PurgeOrphanAttachments(ctx)
	a.syncClient = NewSyncClient(a.store, func(status SyncStatus) {
		runtime.EventsEmit(a.ctx, "sync:status", status)
	})
	_ = a.syncClient.RestoreSession(ctx)
	syncCtx, cancel := context.WithCancel(context.Background())
	a.syncCancel = cancel
	go a.syncClient.Run(syncCtx)
	go a.syncClient.RunWebSocket(syncCtx)
}

func (a *App) Shutdown(context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.syncCancel != nil {
		a.syncCancel()
	}
	if a.store != nil {
		_ = a.store.Close()
	}
}

// SetCompactMode switches the main window between its full workspace and a
// small always-on-top Today view. Wails v2 has one application window, so the
// previous window geometry is retained here and restored when compact mode ends.
func (a *App) SetCompactMode(enabled bool) error {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()
	if a.ctx == nil {
		return errors.New("应用窗口尚未完成初始化")
	}
	if a.window.compact == enabled {
		return nil
	}

	if enabled {
		a.window.wasMaximised = runtime.WindowIsMaximised(a.ctx)
		if a.window.wasMaximised {
			runtime.WindowUnmaximise(a.ctx)
		}
		a.window.width, a.window.height = runtime.WindowGetSize(a.ctx)
		a.window.x, a.window.y = runtime.WindowGetPosition(a.ctx)
		runtime.WindowSetMinSize(a.ctx, 360, 440)
		runtime.WindowSetSize(a.ctx, 380, 540)
		x := a.window.x
		if a.window.width > 380 {
			x += a.window.width - 380
		}
		runtime.WindowSetPosition(a.ctx, x, a.window.y)
		runtime.WindowSetAlwaysOnTop(a.ctx, true)
		a.window.compact = true
		return nil
	}

	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	runtime.WindowSetMinSize(a.ctx, 900, 620)
	width, height := a.window.width, a.window.height
	if width < 900 {
		width = 900
	}
	if height < 620 {
		height = 620
	}
	runtime.WindowSetSize(a.ctx, width, height)
	runtime.WindowSetPosition(a.ctx, a.window.x, a.window.y)
	if a.window.wasMaximised {
		runtime.WindowMaximise(a.ctx)
	}
	a.window.compact = false
	return nil
}

func (a *App) GetCompactMode() bool {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()
	return a.window.compact
}

func (a *App) Login(input LoginInput) (LoginResult, error) {
	s, err := a.ready()
	if err != nil {
		return LoginResult{}, err
	}
	if !input.MigrateLocal {
		return LoginResult{}, errors.New("连接同步账号前需要确认迁移当前本地数据")
	}
	serverURL, err := validateServerURL(input.ServerURL)
	if err != nil {
		return LoginResult{}, err
	}
	input.ServerURL = serverURL
	identity := strings.ToLower(strings.TrimSpace(input.Username)) + "@" + strings.ToLower(strings.TrimRight(strings.TrimSpace(input.ServerURL), "/"))
	var archivedIdentity string
	_ = s.db.QueryRowContext(a.ctx, `SELECT value FROM settings WHERE key='syncArchiveIdentity'`).Scan(&archivedIdentity)
	if archivedIdentity != "" && archivedIdentity != identity {
		return LoginResult{}, errors.New("当前本地档案已绑定其他同步账号，不能直接切换账号")
	}
	backup, err := s.ExportSnapshot(a.ctx)
	if err != nil {
		return LoginResult{}, err
	}
	backupDir := filepath.Join(a.dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return LoginResult{}, err
	}
	archivePath := filepath.Join(backupDir, "before-sync-"+time.Now().Format("20060102-150405")+".todo-backup.zip")
	if err := writeBackupArchive(archivePath, backup, s); err != nil {
		return LoginResult{}, fmt.Errorf("创建登录前本地档案失败: %w", err)
	}
	result, err := a.syncClient.Login(a.ctx, input)
	if err != nil {
		return LoginResult{}, err
	}
	_, _ = s.db.ExecContext(a.ctx, `INSERT INTO settings(key,value) VALUES('syncArchiveIdentity',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, identity)
	return result, nil
}

func (a *App) Logout() error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.syncClient.Logout(a.ctx)
}

func (a *App) SyncNow() (SyncStatus, error) {
	if _, err := a.ready(); err != nil {
		return SyncStatus{}, err
	}
	err := a.syncClient.SyncNow(a.ctx)
	status, statusErr := a.store.GetSyncStatus(a.ctx)
	if err != nil {
		return status, err
	}
	return status, statusErr
}

func (a *App) GetSyncStatus() (SyncStatus, error) {
	s, err := a.ready()
	if err != nil {
		return SyncStatus{}, err
	}
	return s.GetSyncStatus(a.ctx)
}

func (a *App) ListNotifications() ([]Notification, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListNotifications(a.ctx)
}

func (a *App) MarkNotificationRead(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	status, _ := s.GetSyncStatus(a.ctx)
	if status.Configured {
		return a.syncClient.MarkNotificationRead(a.ctx, id)
	}
	return s.MarkNotificationRead(a.ctx, id)
}

func (a *App) ready() (*Store, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.store == nil {
		return nil, errors.New("应用尚未完成初始化")
	}
	return a.store, nil
}

func (a *App) GetBootstrap() (Bootstrap, error) {
	s, err := a.ready()
	if err != nil {
		return Bootstrap{}, err
	}
	return s.GetBootstrap(a.ctx, time.Now())
}

func (a *App) ListTasks(query TaskQuery) ([]Task, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListTasks(a.ctx, query, time.Now())
}

func (a *App) CreateTask(input CreateTaskInput) (Task, error) {
	s, err := a.ready()
	if err != nil {
		return Task{}, err
	}
	item, err := s.CreateTask(a.ctx, input)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "task", item.ID, "upsert", 0, item)
	}
	return item, err
}

func (a *App) UpdateTask(input UpdateTaskInput) (Task, error) {
	s, err := a.ready()
	if err != nil {
		return Task{}, err
	}
	item, err := s.UpdateTask(a.ctx, input)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "task", item.ID, "upsert", 0, item)
	}
	return item, err
}

func (a *App) SetTaskCompleted(id string, completed bool) (Task, error) {
	s, err := a.ready()
	if err != nil {
		return Task{}, err
	}
	item, err := s.SetTaskCompleted(a.ctx, id, completed)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "task", item.ID, "upsert", 0, item)
	}
	return item, err
}

func (a *App) DeleteTask(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	if err := s.DeleteTask(a.ctx, id); err != nil {
		return err
	}
	return s.enqueueOutbox(a.ctx, "task", id, "delete", 0, map[string]string{"deletedAt": nowString()})
}

func (a *App) RestoreTask(id string) (Task, error) {
	s, err := a.ready()
	if err != nil {
		return Task{}, err
	}
	item, err := s.RestoreTask(a.ctx, id)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "task", item.ID, "restore", 0, item)
	}
	return item, err
}

func (a *App) ListNotebooks() ([]Notebook, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListNotebooks(a.ctx)
}

func (a *App) CreateNotebook(name string) (Notebook, error) {
	s, err := a.ready()
	if err != nil {
		return Notebook{}, err
	}
	return s.CreateNotebook(a.ctx, name)
}

func (a *App) UpdateNotebook(input NotebookInput) (Notebook, error) {
	s, err := a.ready()
	if err != nil {
		return Notebook{}, err
	}
	return s.UpdateNotebook(a.ctx, input)
}

func (a *App) DeleteNotebook(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.DeleteNotebook(a.ctx, id)
}

func (a *App) ListNoteTags(notebookID string) ([]NoteTag, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListNoteTags(a.ctx, notebookID)
}

func (a *App) CreateNoteTag(input NoteTagInput) (NoteTag, error) {
	s, err := a.ready()
	if err != nil {
		return NoteTag{}, err
	}
	return s.CreateNoteTag(a.ctx, input)
}

func (a *App) UpdateNoteTag(input NoteTagInput) (NoteTag, error) {
	s, err := a.ready()
	if err != nil {
		return NoteTag{}, err
	}
	return s.UpdateNoteTag(a.ctx, input)
}

func (a *App) DeleteNoteTag(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.DeleteNoteTag(a.ctx, id)
}

func (a *App) ListNotes(query NoteQuery) ([]NoteSummary, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListNotes(a.ctx, query)
}

func (a *App) GetNote(id string) (Note, error) {
	s, err := a.ready()
	if err != nil {
		return Note{}, err
	}
	return s.GetNote(a.ctx, id)
}

func (a *App) CreateNote(input CreateNoteInput) (Note, error) {
	s, err := a.ready()
	if err != nil {
		return Note{}, err
	}
	return s.CreateNote(a.ctx, input)
}

func (a *App) UpdateNote(input UpdateNoteInput) (Note, error) {
	s, err := a.ready()
	if err != nil {
		return Note{}, err
	}
	return s.UpdateNote(a.ctx, input)
}

func (a *App) SetNotePinned(id string, pinned bool) (Note, error) {
	s, err := a.ready()
	if err != nil {
		return Note{}, err
	}
	return s.SetNotePinned(a.ctx, id, pinned)
}

func (a *App) DeleteNote(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.DeleteNote(a.ctx, id)
}

func (a *App) RestoreNote(id string) (Note, error) {
	s, err := a.ready()
	if err != nil {
		return Note{}, err
	}
	return s.RestoreNote(a.ctx, id)
}

func (a *App) ListNoteVersions(noteID string) ([]NoteVersion, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListNoteVersions(a.ctx, noteID)
}

func (a *App) RestoreNoteVersion(versionID string) (Note, error) {
	s, err := a.ready()
	if err != nil {
		return Note{}, err
	}
	return s.RestoreNoteVersion(a.ctx, versionID)
}

func (a *App) ListComments(noteID string) ([]Comment, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	return s.ListComments(a.ctx, noteID)
}

func (a *App) CreateComment(input CommentInput) (Comment, error) {
	s, err := a.ready()
	if err != nil {
		return Comment{}, err
	}
	return s.CreateComment(a.ctx, input)
}

func (a *App) UpdateComment(input CommentInput) (Comment, error) {
	s, err := a.ready()
	if err != nil {
		return Comment{}, err
	}
	return s.UpdateComment(a.ctx, input)
}

func (a *App) DeleteComment(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.DeleteComment(a.ctx, id)
}

func (a *App) ListNotebookMembers(notebookID string) ([]NotebookMember, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	status, _ := s.GetSyncStatus(a.ctx)
	if status.Configured {
		return a.syncClient.ListRemoteMembers(a.ctx, notebookID)
	}
	return s.ListNotebookMembers(a.ctx, notebookID)
}

func (a *App) CreateNotebookInvite(input InvitationInput) (InvitationResult, error) {
	if _, err := a.ready(); err != nil {
		return InvitationResult{}, err
	}
	return a.syncClient.CreateInvitation(a.ctx, input)
}

func (a *App) UpdateNotebookMember(input MemberUpdateInput) error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.syncClient.UpdateRemoteMember(a.ctx, input)
}

func (a *App) RemoveNotebookMember(notebookID, userID string) error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.syncClient.RemoveRemoteMember(a.ctx, notebookID, userID)
}

func (a *App) TransferNotebookOwnership(notebookID, userID string) error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.syncClient.TransferRemoteOwnership(a.ctx, notebookID, userID)
}

func (a *App) AcceptInvitation(token string) error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.syncClient.AcceptInvitation(a.ctx, token)
}

func (a *App) AcquireEditLease(noteID string, takeover bool) (EditLease, error) {
	s, err := a.ready()
	if err != nil {
		return EditLease{}, err
	}
	status, _ := s.GetSyncStatus(a.ctx)
	if status.Configured {
		note, err := s.GetNote(a.ctx, noteID)
		if err != nil {
			return EditLease{}, err
		}
		return a.syncClient.AcquireRemoteLease(a.ctx, noteID, note.NotebookID, takeover)
	}
	return s.AcquireEditLease(a.ctx, noteID, takeover)
}

func (a *App) ReleaseEditLease(noteID string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	status, _ := s.GetSyncStatus(a.ctx)
	if status.Configured {
		return a.syncClient.ReleaseRemoteLease(a.ctx, noteID)
	}
	return s.ReleaseEditLease(a.ctx, noteID)
}

func (a *App) AddNoteAttachments(noteID string) ([]Attachment, error) {
	s, err := a.ready()
	if err != nil {
		return nil, err
	}
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "添加笔记附件",
		Filters: []runtime.FileFilter{
			{DisplayName: "支持的文件", Pattern: "*.jpg;*.jpeg;*.png;*.webp;*.gif;*.pdf;*.doc;*.docx;*.xls;*.xlsx;*.ppt;*.pptx;*.txt;*.md;*.csv;*.json;*.xml;*.yaml;*.yml;*.zip;*.7z;*.rar"},
		},
	})
	if err != nil {
		return nil, err
	}
	items := make([]Attachment, 0, len(paths))
	for _, path := range paths {
		item, err := s.AddAttachmentFromPath(a.ctx, noteID, path)
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (a *App) SavePastedAttachment(input AttachmentUploadInput) (Attachment, error) {
	s, err := a.ready()
	if err != nil {
		return Attachment{}, err
	}
	return s.AddAttachmentData(a.ctx, input)
}

func (a *App) ReadAttachment(id string) (AttachmentContent, error) {
	s, err := a.ready()
	if err != nil {
		return AttachmentContent{}, err
	}
	if err := a.syncClient.EnsureAttachmentCached(a.ctx, id); err != nil {
		return AttachmentContent{}, err
	}
	return s.ReadAttachment(a.ctx, id)
}

func (a *App) OpenAttachment(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	if err := a.syncClient.EnsureAttachmentCached(a.ctx, id); err != nil {
		return err
	}
	_, path, err := s.attachmentPath(a.ctx, id)
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, "file:///"+filepath.ToSlash(path))
	return nil
}

func (a *App) DeleteAttachment(noteID, id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	return s.DeleteAttachment(a.ctx, noteID, id)
}

func (a *App) CreateList(name string) (TodoList, error) {
	s, err := a.ready()
	if err != nil {
		return TodoList{}, err
	}
	item, err := s.CreateList(a.ctx, name)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "list", item.ID, "upsert", 0, item)
	}
	return item, err
}

func (a *App) UpdateList(input ListInput) (TodoList, error) {
	s, err := a.ready()
	if err != nil {
		return TodoList{}, err
	}
	item, err := s.UpdateList(a.ctx, input)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "list", item.ID, "upsert", 0, item)
	}
	return item, err
}

func (a *App) DeleteList(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	if err := s.DeleteList(a.ctx, id); err != nil {
		return err
	}
	return s.enqueueOutbox(a.ctx, "list", id, "delete", 0, map[string]string{"deletedAt": nowString()})
}

func (a *App) CreateTag(input TagInput) (Tag, error) {
	s, err := a.ready()
	if err != nil {
		return Tag{}, err
	}
	item, err := s.CreateTag(a.ctx, input)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "tag", item.ID, "upsert", 0, item)
	}
	return item, err
}

func (a *App) UpdateTag(input TagInput) (Tag, error) {
	s, err := a.ready()
	if err != nil {
		return Tag{}, err
	}
	item, err := s.UpdateTag(a.ctx, input)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "tag", item.ID, "upsert", 0, item)
	}
	return item, err
}

func (a *App) DeleteTag(id string) error {
	s, err := a.ready()
	if err != nil {
		return err
	}
	if err := s.DeleteTag(a.ctx, id); err != nil {
		return err
	}
	return s.enqueueOutbox(a.ctx, "tag", id, "delete", 0, map[string]string{"deletedAt": nowString()})
}

func (a *App) GetOverview() (Overview, error) {
	s, err := a.ready()
	if err != nil {
		return Overview{}, err
	}
	return s.GetOverview(a.ctx, time.Now())
}

func (a *App) UpdateSettings(settings AppSettings) (AppSettings, error) {
	s, err := a.ready()
	if err != nil {
		return AppSettings{}, err
	}
	item, err := s.UpdateSettings(a.ctx, settings)
	if err == nil {
		_ = s.enqueueOutbox(a.ctx, "settings", localUserID, "upsert", 0, item)
	}
	return item, err
}

func (a *App) ExportBackup() (FileResult, error) {
	s, err := a.ready()
	if err != nil {
		return FileResult{}, err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出 TODO 备份",
		DefaultFilename: "TODO-backup-" + time.Now().Format("2006-01-02") + ".todo-backup.zip",
		Filters:         []runtime.FileFilter{{DisplayName: "TODO 备份 (*.todo-backup.zip)", Pattern: "*.todo-backup.zip"}},
	})
	if err != nil {
		return FileResult{}, err
	}
	if path == "" {
		return FileResult{Cancelled: true}, nil
	}
	backup, err := s.ExportSnapshot(a.ctx)
	if err != nil {
		return FileResult{}, err
	}
	if err := writeBackupArchive(path, backup, s); err != nil {
		return FileResult{}, err
	}
	return FileResult{Path: path, Count: len(backup.Tasks) + len(backup.Notes), Tasks: len(backup.Tasks), Notes: len(backup.Notes), Attachments: len(backup.Attachments)}, nil
}

func (a *App) ImportBackup() (ImportResult, error) {
	s, err := a.ready()
	if err != nil {
		return ImportResult{}, err
	}
	status, _ := s.GetSyncStatus(a.ctx)
	if status.Configured {
		return ImportResult{}, errors.New("同步账号连接期间不能恢复本地备份，请先退出同步账号")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "恢复 TODO 备份",
		Filters: []runtime.FileFilter{
			{DisplayName: "TODO 备份 (*.todo-backup.zip)", Pattern: "*.todo-backup.zip"},
			{DisplayName: "旧版 JSON 备份 (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return ImportResult{}, err
	}
	if path == "" {
		return ImportResult{Cancelled: true}, nil
	}
	incoming, attachmentPaths, err := readBackupArchive(path, s)
	if err != nil {
		return ImportResult{}, err
	}
	current, err := s.ExportSnapshot(a.ctx)
	if err != nil {
		return ImportResult{}, err
	}
	backupDir := filepath.Join(a.dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return ImportResult{}, err
	}
	backupPath := filepath.Join(backupDir, "before-import-"+time.Now().Format("20060102-150405")+".todo-backup.zip")
	if err := writeBackupArchive(backupPath, current, s); err != nil {
		return ImportResult{}, err
	}
	if err := s.ImportSnapshotWithAttachments(a.ctx, incoming, attachmentPaths); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Path: path, BackupPath: backupPath, Tasks: len(incoming.Tasks), Lists: len(incoming.Lists), Tags: len(incoming.Tags), Notes: len(incoming.Notes), Notebooks: len(incoming.Notebooks), Attachments: len(incoming.Attachments)}, nil
}
