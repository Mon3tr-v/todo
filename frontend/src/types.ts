export type Theme = 'system' | 'light' | 'dark'
export type Priority = 'none' | 'low' | 'medium' | 'high'
export type ViewKey = 'overview' | 'notes' | 'inbox' | 'today' | 'upcoming' | 'completed' | 'all' | 'list'
export type SortKey = 'due' | 'priority' | 'created'

export interface TodoList {
    id: string
    name: string
    position: number
    createdAt: string
    updatedAt: string
}

export interface Tag {
    id: string
    name: string
    color: string
    createdAt: string
    updatedAt: string
}

export interface Task {
    id: string
    title: string
    notes: string
    listId: string
    priority: Priority
    dueDate: string
    completedAt: string
    deletedAt: string
    createdAt: string
    updatedAt: string
    tags: Tag[]
}

export interface TaskQuery {
    view: ViewKey
    listId: string
    tagId: string
    priority: Priority | 'all'
    search: string
    sort: SortKey
    status: string
}

export interface CreateTaskInput {
    title: string
    notes: string
    listId: string
    priority: Priority
    dueDate: string
    tagIds: string[]
}

export interface UpdateTaskInput extends CreateTaskInput { id: string }

export interface AppSettings {
    theme: Theme
    startupView: Exclude<ViewKey, 'completed' | 'list'>
}

export type NotebookRole = 'owner' | 'editor' | 'viewer'

export interface Notebook {
    id: string
    name: string
    kind: 'personal' | 'shared'
    role: NotebookRole
    ownerId: string
    default: boolean
    deletedAt: string
    createdAt: string
    updatedAt: string
    serverVersion: number
}

export interface NoteTag {
    id: string
    notebookId: string
    name: string
    color: string
    deletedAt: string
    createdAt: string
    updatedAt: string
}

export interface TaskReference {
    id: string
    title: string
    dueDate: string
    completedAt: string
}

export interface Attachment {
    id: string
    notebookId: string
    fileName: string
    mimeType: string
    size: number
    sha256: string
    status: 'local' | 'pending' | 'uploading' | 'synced' | 'failed'
    remoteKey: string
    deletedAt: string
    createdAt: string
    updatedAt: string
}

export interface NoteSummary {
    id: string
    notebookId: string
    title: string
    excerpt: string
    pinned: boolean
    revision: number
    tags: NoteTag[]
    attachmentCount: number
    linkedTaskCount: number
    commentCount: number
    createdAt: string
    updatedAt: string
}

export interface Note {
    id: string
    notebookId: string
    title: string
    content: string
    pinned: boolean
    revision: number
    deletedAt: string
    tags: NoteTag[]
    tasks: TaskReference[]
    attachments: Attachment[]
    createdAt: string
    updatedAt: string
    serverVersion: number
}

export interface NoteQuery {
    notebookId: string
    tagId: string
    taskId: string
    search: string
    pinnedOnly: boolean
}

export interface CreateNoteInput {
    notebookId: string
    title: string
    content: string
    tagIds: string[]
    taskIds: string[]
}

export interface UpdateNoteInput extends CreateNoteInput {
    id: string
    baseRevision: number
}

export interface NoteVersion {
    id: string
    noteId: string
    title: string
    content: string
    tags: NoteTag[]
    attachments: Attachment[]
    actorId: string
    actorName: string
    sourceRevision: number
    createdAt: string
}

export interface NoteComment {
    id: string
    noteId: string
    parentId: string
    authorId: string
    authorName: string
    content: string
    deletedAt: string
    createdAt: string
    updatedAt: string
}

export interface NotebookMember {
    notebookId: string
    userId: string
    username: string
    role: NotebookRole
    createdAt: string
}

export interface AccountUser { id: string; username: string; mode: 'local' | 'sync' }

export interface SyncStatus {
    configured: boolean
    online: boolean
    syncing: boolean
    pending: number
    serverUrl: string
    lastSyncAt: string
    lastError: string
}

export interface LoginInput { serverUrl: string; username: string; password: string; migrateLocal: boolean }

export interface EditLease {
    noteId: string
    holderId: string
    holderName: string
    expiresAt: string
    editable: boolean
}

export interface AppNotification { id: string; kind: string; entityId: string; message: string; readAt: string; createdAt: string }

export interface Bootstrap {
    lists: TodoList[]
    tags: Tag[]
    settings: AppSettings
    notebooks: Notebook[]
    noteTags: NoteTag[]
    counts: {inbox: number; today: number; upcoming: number; all: number; completed: number; notes: number}
    listCounts: Record<string, number>
    tagCounts: Record<string, number>
    notebookCounts: Record<string, number>
    noteTagCounts: Record<string, number>
    currentUser: AccountUser
    sync: SyncStatus
    unreadCount: number
}

export interface Overview {
    open: number
    overdue: number
    completedToday: number
    week: Array<{date: string; count: number}>
}

export interface FileResult { path: string; cancelled: boolean; count: number; tasks: number; notes: number; attachments: number }

export interface ImportResult {
    path: string
    backupPath: string
    cancelled: boolean
    tasks: number
    lists: number
    tags: number
    notes: number
    notebooks: number
    attachments: number
}

export interface AppBridge {
	GetCompactMode(): Promise<boolean>
	SetCompactMode(enabled: boolean): Promise<void>
    GetBootstrap(): Promise<Bootstrap>
    ListTasks(query: TaskQuery): Promise<Task[]>
    CreateTask(input: CreateTaskInput): Promise<Task>
    UpdateTask(input: UpdateTaskInput): Promise<Task>
    SetTaskCompleted(id: string, completed: boolean): Promise<Task>
    DeleteTask(id: string): Promise<void>
    RestoreTask(id: string): Promise<Task>
    ListNotebooks(): Promise<Notebook[]>
    CreateNotebook(name: string): Promise<Notebook>
    UpdateNotebook(input: {id: string; name: string}): Promise<Notebook>
    DeleteNotebook(id: string): Promise<void>
    ListNoteTags(notebookId: string): Promise<NoteTag[]>
    CreateNoteTag(input: {id: string; notebookId: string; name: string; color: string}): Promise<NoteTag>
    UpdateNoteTag(input: {id: string; notebookId: string; name: string; color: string}): Promise<NoteTag>
    DeleteNoteTag(id: string): Promise<void>
    ListNotes(query: NoteQuery): Promise<NoteSummary[]>
    GetNote(id: string): Promise<Note>
    CreateNote(input: CreateNoteInput): Promise<Note>
    UpdateNote(input: UpdateNoteInput): Promise<Note>
    SetNotePinned(id: string, pinned: boolean): Promise<Note>
    DeleteNote(id: string): Promise<void>
    RestoreNote(id: string): Promise<Note>
    ListNoteVersions(noteId: string): Promise<NoteVersion[]>
    RestoreNoteVersion(versionId: string): Promise<Note>
    ListComments(noteId: string): Promise<NoteComment[]>
    CreateComment(input: {id: string; noteId: string; parentId: string; content: string}): Promise<NoteComment>
    UpdateComment(input: {id: string; noteId: string; parentId: string; content: string}): Promise<NoteComment>
    DeleteComment(id: string): Promise<void>
    ListNotebookMembers(notebookId: string): Promise<NotebookMember[]>
    CreateNotebookInvite(input: {notebookId: string; username: string; role: 'editor' | 'viewer'}): Promise<{id: string; token: string; expiresIn: number}>
    UpdateNotebookMember(input: {notebookId: string; userId: string; role: 'editor' | 'viewer'}): Promise<void>
    RemoveNotebookMember(notebookId: string, userId: string): Promise<void>
    TransferNotebookOwnership(notebookId: string, userId: string): Promise<void>
    AcceptInvitation(token: string): Promise<void>
    AcquireEditLease(noteId: string, takeover: boolean): Promise<EditLease>
    ReleaseEditLease(noteId: string): Promise<void>
    AddNoteAttachments(noteId: string): Promise<Attachment[]>
    SavePastedAttachment(input: {noteId: string; fileName: string; mimeType: string; data: string}): Promise<Attachment>
    ReadAttachment(id: string): Promise<{mimeType: string; data: string}>
    OpenAttachment(id: string): Promise<void>
    DeleteAttachment(noteId: string, id: string): Promise<void>
    Login(input: LoginInput): Promise<{user: AccountUser; sync: SyncStatus}>
    Logout(): Promise<void>
    SyncNow(): Promise<SyncStatus>
    GetSyncStatus(): Promise<SyncStatus>
    ListNotifications(): Promise<AppNotification[]>
    MarkNotificationRead(id: string): Promise<void>
    CreateList(name: string): Promise<TodoList>
    UpdateList(input: {id: string; name: string}): Promise<TodoList>
    DeleteList(id: string): Promise<void>
    CreateTag(input: {id: string; name: string; color: string}): Promise<Tag>
    UpdateTag(input: {id: string; name: string; color: string}): Promise<Tag>
    DeleteTag(id: string): Promise<void>
    GetOverview(): Promise<Overview>
    UpdateSettings(settings: AppSettings): Promise<AppSettings>
    ExportBackup(): Promise<FileResult>
    ImportBackup(): Promise<ImportResult>
}
