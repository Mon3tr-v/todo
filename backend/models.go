package backend

type TodoList struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Tag struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Notes       string `json:"notes"`
	ListID      string `json:"listId"`
	Priority    string `json:"priority"`
	DueDate     string `json:"dueDate"`
	CompletedAt string `json:"completedAt"`
	DeletedAt   string `json:"deletedAt"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	Tags        []Tag  `json:"tags"`
}

type TaskQuery struct {
	View     string `json:"view"`
	ListID   string `json:"listId"`
	TagID    string `json:"tagId"`
	Priority string `json:"priority"`
	Search   string `json:"search"`
	Sort     string `json:"sort"`
	Status   string `json:"status"`
}

type CreateTaskInput struct {
	Title    string   `json:"title"`
	Notes    string   `json:"notes"`
	ListID   string   `json:"listId"`
	Priority string   `json:"priority"`
	DueDate  string   `json:"dueDate"`
	TagIDs   []string `json:"tagIds"`
}

type UpdateTaskInput struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Notes    string   `json:"notes"`
	ListID   string   `json:"listId"`
	Priority string   `json:"priority"`
	DueDate  string   `json:"dueDate"`
	TagIDs   []string `json:"tagIds"`
}

type ListInput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TagInput struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type AppSettings struct {
	Theme       string `json:"theme"`
	StartupView string `json:"startupView"`
}

type SmartCounts struct {
	Inbox     int `json:"inbox"`
	Today     int `json:"today"`
	Upcoming  int `json:"upcoming"`
	All       int `json:"all"`
	Completed int `json:"completed"`
	Notes     int `json:"notes"`
}

type Bootstrap struct {
	Lists          []TodoList     `json:"lists"`
	Tags           []Tag          `json:"tags"`
	Notebooks      []Notebook     `json:"notebooks"`
	NoteTags       []NoteTag      `json:"noteTags"`
	Settings       AppSettings    `json:"settings"`
	Counts         SmartCounts    `json:"counts"`
	ListCounts     map[string]int `json:"listCounts"`
	TagCounts      map[string]int `json:"tagCounts"`
	NotebookCounts map[string]int `json:"notebookCounts"`
	NoteTagCounts  map[string]int `json:"noteTagCounts"`
	CurrentUser    AccountUser    `json:"currentUser"`
	Sync           SyncStatus     `json:"sync"`
	UnreadCount    int            `json:"unreadCount"`
}

type Notebook struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Role          string `json:"role"`
	OwnerID       string `json:"ownerId"`
	Default       bool   `json:"default"`
	DeletedAt     string `json:"deletedAt"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	ServerVersion int64  `json:"serverVersion"`
}

type NotebookInput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type NoteTag struct {
	ID         string `json:"id"`
	NotebookID string `json:"notebookId"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	DeletedAt  string `json:"deletedAt"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type NoteTagInput struct {
	ID         string `json:"id"`
	NotebookID string `json:"notebookId"`
	Name       string `json:"name"`
	Color      string `json:"color"`
}

type TaskReference struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	DueDate     string `json:"dueDate"`
	CompletedAt string `json:"completedAt"`
}

type Attachment struct {
	ID         string `json:"id"`
	NotebookID string `json:"notebookId"`
	FileName   string `json:"fileName"`
	MimeType   string `json:"mimeType"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Status     string `json:"status"`
	RemoteKey  string `json:"remoteKey"`
	DeletedAt  string `json:"deletedAt"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type NoteSummary struct {
	ID              string    `json:"id"`
	NotebookID      string    `json:"notebookId"`
	Title           string    `json:"title"`
	Excerpt         string    `json:"excerpt"`
	Pinned          bool      `json:"pinned"`
	Revision        int64     `json:"revision"`
	Tags            []NoteTag `json:"tags"`
	AttachmentCount int       `json:"attachmentCount"`
	LinkedTaskCount int       `json:"linkedTaskCount"`
	CommentCount    int       `json:"commentCount"`
	CreatedAt       string    `json:"createdAt"`
	UpdatedAt       string    `json:"updatedAt"`
}

type Note struct {
	ID            string          `json:"id"`
	NotebookID    string          `json:"notebookId"`
	Title         string          `json:"title"`
	Content       string          `json:"content"`
	Pinned        bool            `json:"pinned"`
	Revision      int64           `json:"revision"`
	DeletedAt     string          `json:"deletedAt"`
	Tags          []NoteTag       `json:"tags"`
	Tasks         []TaskReference `json:"tasks"`
	Attachments   []Attachment    `json:"attachments"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt"`
	ServerVersion int64           `json:"serverVersion"`
}

type NoteQuery struct {
	NotebookID string `json:"notebookId"`
	TagID      string `json:"tagId"`
	TaskID     string `json:"taskId"`
	Search     string `json:"search"`
	PinnedOnly bool   `json:"pinnedOnly"`
}

type CreateNoteInput struct {
	NotebookID string   `json:"notebookId"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	TagIDs     []string `json:"tagIds"`
	TaskIDs    []string `json:"taskIds"`
}

type UpdateNoteInput struct {
	ID           string   `json:"id"`
	NotebookID   string   `json:"notebookId"`
	Title        string   `json:"title"`
	Content      string   `json:"content"`
	TagIDs       []string `json:"tagIds"`
	TaskIDs      []string `json:"taskIds"`
	BaseRevision int64    `json:"baseRevision"`
}

type NoteVersion struct {
	ID             string       `json:"id"`
	NoteID         string       `json:"noteId"`
	Title          string       `json:"title"`
	Content        string       `json:"content"`
	Tags           []NoteTag    `json:"tags"`
	Attachments    []Attachment `json:"attachments"`
	ActorID        string       `json:"actorId"`
	ActorName      string       `json:"actorName"`
	SourceRevision int64        `json:"sourceRevision"`
	CreatedAt      string       `json:"createdAt"`
}

type Comment struct {
	ID         string `json:"id"`
	NoteID     string `json:"noteId"`
	ParentID   string `json:"parentId"`
	AuthorID   string `json:"authorId"`
	AuthorName string `json:"authorName"`
	Content    string `json:"content"`
	DeletedAt  string `json:"deletedAt"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type CommentInput struct {
	ID       string `json:"id"`
	NoteID   string `json:"noteId"`
	ParentID string `json:"parentId"`
	Content  string `json:"content"`
}

type NotebookMember struct {
	NotebookID string `json:"notebookId"`
	UserID     string `json:"userId"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	CreatedAt  string `json:"createdAt"`
}

type MemberUpdateInput struct {
	NotebookID string `json:"notebookId"`
	UserID     string `json:"userId"`
	Role       string `json:"role"`
}

type AccountUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Mode     string `json:"mode"`
}

type SyncStatus struct {
	Configured bool   `json:"configured"`
	Online     bool   `json:"online"`
	Syncing    bool   `json:"syncing"`
	Pending    int    `json:"pending"`
	ServerURL  string `json:"serverUrl"`
	LastSyncAt string `json:"lastSyncAt"`
	LastError  string `json:"lastError"`
}

type LoginInput struct {
	ServerURL    string `json:"serverUrl"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	MigrateLocal bool   `json:"migrateLocal"`
}

type LoginResult struct {
	User AccountUser `json:"user"`
	Sync SyncStatus  `json:"sync"`
}

type Notification struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	EntityID  string `json:"entityId"`
	Message   string `json:"message"`
	ReadAt    string `json:"readAt"`
	CreatedAt string `json:"createdAt"`
}

type InvitationResult struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	ExpiresIn int    `json:"expiresIn"`
}

type InvitationInput struct {
	NotebookID string `json:"notebookId"`
	Username   string `json:"username"`
	Role       string `json:"role"`
}

type AttachmentUploadInput struct {
	NoteID   string `json:"noteId"`
	FileName string `json:"fileName"`
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type AttachmentContent struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type EditLease struct {
	NoteID     string `json:"noteId"`
	HolderID   string `json:"holderId"`
	HolderName string `json:"holderName"`
	ExpiresAt  string `json:"expiresAt"`
	Editable   bool   `json:"editable"`
}

type DayStat struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type Overview struct {
	Open           int       `json:"open"`
	Overdue        int       `json:"overdue"`
	CompletedToday int       `json:"completedToday"`
	Week           []DayStat `json:"week"`
}

type FileResult struct {
	Path        string `json:"path"`
	Cancelled   bool   `json:"cancelled"`
	Count       int    `json:"count"`
	Tasks       int    `json:"tasks"`
	Notes       int    `json:"notes"`
	Attachments int    `json:"attachments"`
}

type ImportResult struct {
	Path        string `json:"path"`
	BackupPath  string `json:"backupPath"`
	Cancelled   bool   `json:"cancelled"`
	Tasks       int    `json:"tasks"`
	Lists       int    `json:"lists"`
	Tags        int    `json:"tags"`
	Notes       int    `json:"notes"`
	Notebooks   int    `json:"notebooks"`
	Attachments int    `json:"attachments"`
}

type BackupNote struct {
	ID            string   `json:"id"`
	NotebookID    string   `json:"notebookId"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Pinned        bool     `json:"pinned"`
	Revision      int64    `json:"revision"`
	TagIDs        []string `json:"tagIds"`
	TaskIDs       []string `json:"taskIds"`
	AttachmentIDs []string `json:"attachmentIds"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

type BackupNoteVersion struct {
	ID             string   `json:"id"`
	NoteID         string   `json:"noteId"`
	NotebookID     string   `json:"notebookId"`
	Title          string   `json:"title"`
	Content        string   `json:"content"`
	TagIDs         []string `json:"tagIds"`
	AttachmentIDs  []string `json:"attachmentIds"`
	ActorID        string   `json:"actorId"`
	ActorName      string   `json:"actorName"`
	SourceRevision int64    `json:"sourceRevision"`
	CreatedAt      string   `json:"createdAt"`
}

type BackupEnvelope struct {
	SchemaVersion int                 `json:"schemaVersion"`
	ExportedAt    string              `json:"exportedAt"`
	Lists         []TodoList          `json:"lists"`
	Tags          []Tag               `json:"tags"`
	Tasks         []Task              `json:"tasks"`
	Notebooks     []Notebook          `json:"notebooks,omitempty"`
	NoteTags      []NoteTag           `json:"noteTags,omitempty"`
	Notes         []BackupNote        `json:"notes,omitempty"`
	Attachments   []Attachment        `json:"attachments,omitempty"`
	Versions      []BackupNoteVersion `json:"versions,omitempty"`
	Comments      []Comment           `json:"comments,omitempty"`
	Memberships   []NotebookMember    `json:"memberships,omitempty"`
	Settings      map[string]string   `json:"settings"`
}
