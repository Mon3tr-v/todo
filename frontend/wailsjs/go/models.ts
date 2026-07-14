export namespace backend {
	
	export class AccountUser {
	    id: string;
	    username: string;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new AccountUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.mode = source["mode"];
	    }
	}
	export class AppSettings {
	    theme: string;
	    startupView: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.startupView = source["startupView"];
	    }
	}
	export class Attachment {
	    id: string;
	    notebookId: string;
	    fileName: string;
	    mimeType: string;
	    size: number;
	    sha256: string;
	    status: string;
	    remoteKey: string;
	    deletedAt: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.notebookId = source["notebookId"];
	        this.fileName = source["fileName"];
	        this.mimeType = source["mimeType"];
	        this.size = source["size"];
	        this.sha256 = source["sha256"];
	        this.status = source["status"];
	        this.remoteKey = source["remoteKey"];
	        this.deletedAt = source["deletedAt"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class AttachmentContent {
	    mimeType: string;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new AttachmentContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mimeType = source["mimeType"];
	        this.data = source["data"];
	    }
	}
	export class AttachmentUploadInput {
	    noteId: string;
	    fileName: string;
	    mimeType: string;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new AttachmentUploadInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.noteId = source["noteId"];
	        this.fileName = source["fileName"];
	        this.mimeType = source["mimeType"];
	        this.data = source["data"];
	    }
	}
	export class SyncStatus {
	    configured: boolean;
	    online: boolean;
	    syncing: boolean;
	    pending: number;
	    serverUrl: string;
	    lastSyncAt: string;
	    lastError: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.online = source["online"];
	        this.syncing = source["syncing"];
	        this.pending = source["pending"];
	        this.serverUrl = source["serverUrl"];
	        this.lastSyncAt = source["lastSyncAt"];
	        this.lastError = source["lastError"];
	    }
	}
	export class SmartCounts {
	    inbox: number;
	    today: number;
	    upcoming: number;
	    all: number;
	    completed: number;
	    notes: number;
	
	    static createFrom(source: any = {}) {
	        return new SmartCounts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inbox = source["inbox"];
	        this.today = source["today"];
	        this.upcoming = source["upcoming"];
	        this.all = source["all"];
	        this.completed = source["completed"];
	        this.notes = source["notes"];
	    }
	}
	export class NoteTag {
	    id: string;
	    notebookId: string;
	    name: string;
	    color: string;
	    deletedAt: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new NoteTag(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.notebookId = source["notebookId"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.deletedAt = source["deletedAt"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class Notebook {
	    id: string;
	    name: string;
	    kind: string;
	    role: string;
	    ownerId: string;
	    default: boolean;
	    deletedAt: string;
	    createdAt: string;
	    updatedAt: string;
	    serverVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new Notebook(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.role = source["role"];
	        this.ownerId = source["ownerId"];
	        this.default = source["default"];
	        this.deletedAt = source["deletedAt"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.serverVersion = source["serverVersion"];
	    }
	}
	export class Tag {
	    id: string;
	    name: string;
	    color: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Tag(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class TodoList {
	    id: string;
	    name: string;
	    position: number;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new TodoList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.position = source["position"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class Bootstrap {
	    lists: TodoList[];
	    tags: Tag[];
	    notebooks: Notebook[];
	    noteTags: NoteTag[];
	    settings: AppSettings;
	    counts: SmartCounts;
	    listCounts: Record<string, number>;
	    tagCounts: Record<string, number>;
	    notebookCounts: Record<string, number>;
	    noteTagCounts: Record<string, number>;
	    currentUser: AccountUser;
	    sync: SyncStatus;
	    unreadCount: number;
	
	    static createFrom(source: any = {}) {
	        return new Bootstrap(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lists = this.convertValues(source["lists"], TodoList);
	        this.tags = this.convertValues(source["tags"], Tag);
	        this.notebooks = this.convertValues(source["notebooks"], Notebook);
	        this.noteTags = this.convertValues(source["noteTags"], NoteTag);
	        this.settings = this.convertValues(source["settings"], AppSettings);
	        this.counts = this.convertValues(source["counts"], SmartCounts);
	        this.listCounts = source["listCounts"];
	        this.tagCounts = source["tagCounts"];
	        this.notebookCounts = source["notebookCounts"];
	        this.noteTagCounts = source["noteTagCounts"];
	        this.currentUser = this.convertValues(source["currentUser"], AccountUser);
	        this.sync = this.convertValues(source["sync"], SyncStatus);
	        this.unreadCount = source["unreadCount"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Comment {
	    id: string;
	    noteId: string;
	    parentId: string;
	    authorId: string;
	    authorName: string;
	    content: string;
	    deletedAt: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Comment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.noteId = source["noteId"];
	        this.parentId = source["parentId"];
	        this.authorId = source["authorId"];
	        this.authorName = source["authorName"];
	        this.content = source["content"];
	        this.deletedAt = source["deletedAt"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class CommentInput {
	    id: string;
	    noteId: string;
	    parentId: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new CommentInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.noteId = source["noteId"];
	        this.parentId = source["parentId"];
	        this.content = source["content"];
	    }
	}
	export class CreateNoteInput {
	    notebookId: string;
	    title: string;
	    content: string;
	    tagIds: string[];
	    taskIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new CreateNoteInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.notebookId = source["notebookId"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.tagIds = source["tagIds"];
	        this.taskIds = source["taskIds"];
	    }
	}
	export class CreateTaskInput {
	    title: string;
	    notes: string;
	    listId: string;
	    priority: string;
	    dueDate: string;
	    tagIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new CreateTaskInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.notes = source["notes"];
	        this.listId = source["listId"];
	        this.priority = source["priority"];
	        this.dueDate = source["dueDate"];
	        this.tagIds = source["tagIds"];
	    }
	}
	export class DayStat {
	    date: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new DayStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.count = source["count"];
	    }
	}
	export class EditLease {
	    noteId: string;
	    holderId: string;
	    holderName: string;
	    expiresAt: string;
	    editable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EditLease(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.noteId = source["noteId"];
	        this.holderId = source["holderId"];
	        this.holderName = source["holderName"];
	        this.expiresAt = source["expiresAt"];
	        this.editable = source["editable"];
	    }
	}
	export class FileResult {
	    path: string;
	    cancelled: boolean;
	    count: number;
	    tasks: number;
	    notes: number;
	    attachments: number;
	
	    static createFrom(source: any = {}) {
	        return new FileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.cancelled = source["cancelled"];
	        this.count = source["count"];
	        this.tasks = source["tasks"];
	        this.notes = source["notes"];
	        this.attachments = source["attachments"];
	    }
	}
	export class ImportResult {
	    path: string;
	    backupPath: string;
	    cancelled: boolean;
	    tasks: number;
	    lists: number;
	    tags: number;
	    notes: number;
	    notebooks: number;
	    attachments: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.backupPath = source["backupPath"];
	        this.cancelled = source["cancelled"];
	        this.tasks = source["tasks"];
	        this.lists = source["lists"];
	        this.tags = source["tags"];
	        this.notes = source["notes"];
	        this.notebooks = source["notebooks"];
	        this.attachments = source["attachments"];
	    }
	}
	export class InvitationInput {
	    notebookId: string;
	    username: string;
	    role: string;
	
	    static createFrom(source: any = {}) {
	        return new InvitationInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.notebookId = source["notebookId"];
	        this.username = source["username"];
	        this.role = source["role"];
	    }
	}
	export class InvitationResult {
	    id: string;
	    token: string;
	    expiresIn: number;
	
	    static createFrom(source: any = {}) {
	        return new InvitationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.token = source["token"];
	        this.expiresIn = source["expiresIn"];
	    }
	}
	export class ListInput {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ListInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class LoginInput {
	    serverUrl: string;
	    username: string;
	    password: string;
	    migrateLocal: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LoginInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverUrl = source["serverUrl"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.migrateLocal = source["migrateLocal"];
	    }
	}
	export class LoginResult {
	    user: AccountUser;
	    sync: SyncStatus;
	
	    static createFrom(source: any = {}) {
	        return new LoginResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = this.convertValues(source["user"], AccountUser);
	        this.sync = this.convertValues(source["sync"], SyncStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MemberUpdateInput {
	    notebookId: string;
	    userId: string;
	    role: string;
	
	    static createFrom(source: any = {}) {
	        return new MemberUpdateInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.notebookId = source["notebookId"];
	        this.userId = source["userId"];
	        this.role = source["role"];
	    }
	}
	export class TaskReference {
	    id: string;
	    title: string;
	    dueDate: string;
	    completedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.dueDate = source["dueDate"];
	        this.completedAt = source["completedAt"];
	    }
	}
	export class Note {
	    id: string;
	    notebookId: string;
	    title: string;
	    content: string;
	    pinned: boolean;
	    revision: number;
	    deletedAt: string;
	    tags: NoteTag[];
	    tasks: TaskReference[];
	    attachments: Attachment[];
	    createdAt: string;
	    updatedAt: string;
	    serverVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new Note(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.notebookId = source["notebookId"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.pinned = source["pinned"];
	        this.revision = source["revision"];
	        this.deletedAt = source["deletedAt"];
	        this.tags = this.convertValues(source["tags"], NoteTag);
	        this.tasks = this.convertValues(source["tasks"], TaskReference);
	        this.attachments = this.convertValues(source["attachments"], Attachment);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.serverVersion = source["serverVersion"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NoteQuery {
	    notebookId: string;
	    tagId: string;
	    taskId: string;
	    search: string;
	    pinnedOnly: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NoteQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.notebookId = source["notebookId"];
	        this.tagId = source["tagId"];
	        this.taskId = source["taskId"];
	        this.search = source["search"];
	        this.pinnedOnly = source["pinnedOnly"];
	    }
	}
	export class NoteSummary {
	    id: string;
	    notebookId: string;
	    title: string;
	    excerpt: string;
	    pinned: boolean;
	    revision: number;
	    tags: NoteTag[];
	    attachmentCount: number;
	    linkedTaskCount: number;
	    commentCount: number;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new NoteSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.notebookId = source["notebookId"];
	        this.title = source["title"];
	        this.excerpt = source["excerpt"];
	        this.pinned = source["pinned"];
	        this.revision = source["revision"];
	        this.tags = this.convertValues(source["tags"], NoteTag);
	        this.attachmentCount = source["attachmentCount"];
	        this.linkedTaskCount = source["linkedTaskCount"];
	        this.commentCount = source["commentCount"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class NoteTagInput {
	    id: string;
	    notebookId: string;
	    name: string;
	    color: string;
	
	    static createFrom(source: any = {}) {
	        return new NoteTagInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.notebookId = source["notebookId"];
	        this.name = source["name"];
	        this.color = source["color"];
	    }
	}
	export class NoteVersion {
	    id: string;
	    noteId: string;
	    title: string;
	    content: string;
	    tags: NoteTag[];
	    attachments: Attachment[];
	    actorId: string;
	    actorName: string;
	    sourceRevision: number;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new NoteVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.noteId = source["noteId"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.tags = this.convertValues(source["tags"], NoteTag);
	        this.attachments = this.convertValues(source["attachments"], Attachment);
	        this.actorId = source["actorId"];
	        this.actorName = source["actorName"];
	        this.sourceRevision = source["sourceRevision"];
	        this.createdAt = source["createdAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class NotebookInput {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new NotebookInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class NotebookMember {
	    notebookId: string;
	    userId: string;
	    username: string;
	    role: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new NotebookMember(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.notebookId = source["notebookId"];
	        this.userId = source["userId"];
	        this.username = source["username"];
	        this.role = source["role"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Notification {
	    id: string;
	    kind: string;
	    entityId: string;
	    message: string;
	    readAt: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Notification(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.entityId = source["entityId"];
	        this.message = source["message"];
	        this.readAt = source["readAt"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Overview {
	    open: number;
	    overdue: number;
	    completedToday: number;
	    week: DayStat[];
	
	    static createFrom(source: any = {}) {
	        return new Overview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.open = source["open"];
	        this.overdue = source["overdue"];
	        this.completedToday = source["completedToday"];
	        this.week = this.convertValues(source["week"], DayStat);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class TagInput {
	    id: string;
	    name: string;
	    color: string;
	
	    static createFrom(source: any = {}) {
	        return new TagInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	    }
	}
	export class Task {
	    id: string;
	    title: string;
	    notes: string;
	    listId: string;
	    priority: string;
	    dueDate: string;
	    completedAt: string;
	    deletedAt: string;
	    createdAt: string;
	    updatedAt: string;
	    tags: Tag[];
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.notes = source["notes"];
	        this.listId = source["listId"];
	        this.priority = source["priority"];
	        this.dueDate = source["dueDate"];
	        this.completedAt = source["completedAt"];
	        this.deletedAt = source["deletedAt"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.tags = this.convertValues(source["tags"], Tag);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TaskQuery {
	    view: string;
	    listId: string;
	    tagId: string;
	    priority: string;
	    search: string;
	    sort: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.view = source["view"];
	        this.listId = source["listId"];
	        this.tagId = source["tagId"];
	        this.priority = source["priority"];
	        this.search = source["search"];
	        this.sort = source["sort"];
	        this.status = source["status"];
	    }
	}
	
	
	export class UpdateNoteInput {
	    id: string;
	    notebookId: string;
	    title: string;
	    content: string;
	    tagIds: string[];
	    taskIds: string[];
	    baseRevision: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateNoteInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.notebookId = source["notebookId"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.tagIds = source["tagIds"];
	        this.taskIds = source["taskIds"];
	        this.baseRevision = source["baseRevision"];
	    }
	}
	export class UpdateTaskInput {
	    id: string;
	    title: string;
	    notes: string;
	    listId: string;
	    priority: string;
	    dueDate: string;
	    tagIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new UpdateTaskInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.notes = source["notes"];
	        this.listId = source["listId"];
	        this.priority = source["priority"];
	        this.dueDate = source["dueDate"];
	        this.tagIds = source["tagIds"];
	    }
	}

}

