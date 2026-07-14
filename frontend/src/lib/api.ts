import type {
    Attachment,
    AppBridge,
    AppSettings,
    CreateTaskInput,
    ImportResult,
    Note,
    NoteComment,
    NoteQuery,
    NoteSummary,
    NoteTag,
    NoteVersion,
    Notebook,
    Overview,
    Task,
    TaskQuery,
    TodoList,
} from '../types'

const STORAGE_KEY = 'todo-browser-preview-v1'
const COMPACT_MODE_KEY = 'todo-browser-compact-mode'

interface MockState {
    lists: TodoList[]
    tags: Array<{id: string; name: string; color: string; createdAt: string; updatedAt: string}>
    tasks: Task[]
    notebooks: Notebook[]
    noteTags: NoteTag[]
    notes: Note[]
    versions: NoteVersion[]
    comments: NoteComment[]
    attachmentData: Record<string, string>
    settings: AppSettings
}

const iso = () => new Date().toISOString()
const uid = () => crypto.randomUUID()
const today = () => new Date().toLocaleDateString('sv-SE')

function initialState(): MockState {
    const createdAt = iso()
    const notebookId = uid()
    return {
        lists: [
            {id: uid(), name: '个人', position: 0, createdAt, updatedAt: createdAt},
            {id: uid(), name: '工作', position: 1, createdAt, updatedAt: createdAt},
        ],
        tags: [],
        tasks: [],
        notebooks: [{id: notebookId, name: '我的笔记', kind: 'personal', role: 'owner', ownerId: 'local', default: true, deletedAt: '', createdAt, updatedAt: createdAt, serverVersion: 0}],
        noteTags: [],
        notes: [],
        versions: [],
        comments: [],
        attachmentData: {},
        settings: {theme: 'system', startupView: 'today'},
    }
}

function save(state: MockState) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
}

function load(): MockState {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
        try {
            const parsed = JSON.parse(raw) as Partial<MockState>
            const fallback = initialState()
            return {
                ...fallback,
                ...parsed,
                notebooks: parsed.notebooks?.length ? parsed.notebooks : fallback.notebooks,
                noteTags: parsed.noteTags ?? [], notes: parsed.notes ?? [], versions: parsed.versions ?? [],
                comments: parsed.comments ?? [], attachmentData: parsed.attachmentData ?? {},
            }
        } catch { /* reset invalid preview data */ }
    }
    const state = initialState()
    save(state)
    return state
}

function noteSummary(state: MockState, note: Note): NoteSummary {
    const comments = state.comments.filter((comment) => comment.noteId === note.id && !comment.deletedAt)
    return {
        id: note.id, notebookId: note.notebookId, title: note.title,
        excerpt: note.content.replace(/[#*_`>\[\]()]/g, '').replace(/\s+/g, ' ').trim().slice(0, 150),
        pinned: note.pinned, revision: note.revision, tags: note.tags,
        attachmentCount: note.attachments.length, linkedTaskCount: note.tasks.length, commentCount: comments.length,
        createdAt: note.createdAt, updatedAt: note.updatedAt,
    }
}

function filterNotes(state: MockState, query: NoteQuery): NoteSummary[] {
    const needle = query.search.trim().toLocaleLowerCase()
    return state.notes
        .filter((note) => !note.deletedAt)
        .filter((note) => !query.notebookId || note.notebookId === query.notebookId)
        .filter((note) => !query.tagId || note.tags.some((tag) => tag.id === query.tagId))
        .filter((note) => !query.taskId || note.tasks.some((task) => task.id === query.taskId))
        .filter((note) => !query.pinnedOnly || note.pinned)
        .filter((note) => !needle || `${note.title} ${note.content}`.toLocaleLowerCase().includes(needle))
        .sort((a, b) => Number(b.pinned) - Number(a.pinned) || b.updatedAt.localeCompare(a.updatedAt))
        .map((note) => noteSummary(state, note))
}

function saveMockVersion(state: MockState, note: Note) {
    const latest = state.versions.find((version) => version.noteId === note.id)
    const timestamp = Date.now()
    const canMerge = latest && timestamp - new Date(latest.createdAt).getTime() <= 5 * 60_000
    const version: NoteVersion = {
        id: canMerge ? latest.id : uid(), noteId: note.id, title: note.title, content: note.content,
        tags: note.tags, attachments: note.attachments, actorId: 'local', actorName: '本地用户',
        sourceRevision: note.revision, createdAt: iso(),
    }
    state.versions = canMerge ? state.versions.map((item) => item.id === latest.id ? version : item) : [version, ...state.versions]
    state.versions = state.versions.filter((item, index) => item.noteId !== note.id || index < 100)
}

function mockOverview(tasks: Task[]): Overview {
    const active = tasks.filter((task) => !task.deletedAt)
    const dates = Array.from({length: 7}, (_, index) => {
        const date = new Date()
        date.setDate(date.getDate() - (6 - index))
        return date.toLocaleDateString('sv-SE')
    })
    return {
        open: active.filter((task) => !task.completedAt).length,
        overdue: active.filter((task) => !task.completedAt && task.dueDate && task.dueDate < today()).length,
        completedToday: active.filter((task) => task.completedAt && new Date(task.completedAt).toLocaleDateString('sv-SE') === today()).length,
        week: dates.map((date) => ({
            date,
            count: active.filter((task) => task.completedAt && new Date(task.completedAt).toLocaleDateString('sv-SE') === date).length,
        })),
    }
}

function filterTasks(tasks: Task[], query: TaskQuery): Task[] {
    let result = tasks.filter((task) => !task.deletedAt)
    switch (query.view) {
        case 'inbox': result = result.filter((task) => !task.listId && !task.completedAt); break
        case 'today': result = result.filter((task) => Boolean(task.dueDate) && task.dueDate <= today() && !task.completedAt); break
        case 'upcoming': result = result.filter((task) => task.dueDate > today() && !task.completedAt); break
        case 'completed': result = result.filter((task) => Boolean(task.completedAt)); break
        case 'list': result = result.filter((task) => task.listId === query.listId && !task.completedAt); break
        default: result = result.filter((task) => !task.completedAt)
    }
    if (query.tagId) result = result.filter((task) => task.tags.some((tag) => tag.id === query.tagId))
    if (query.priority !== 'all') result = result.filter((task) => task.priority === query.priority)
    if (query.search.trim()) {
        const needle = query.search.trim().toLocaleLowerCase()
        result = result.filter((task) => `${task.title} ${task.notes}`.toLocaleLowerCase().includes(needle))
    }
    const priorityRank = {high: 0, medium: 1, low: 2, none: 3}
    return [...result].sort((a, b) => {
        if (query.sort === 'created') return b.createdAt.localeCompare(a.createdAt)
        if (query.sort === 'priority') return priorityRank[a.priority] - priorityRank[b.priority] || a.dueDate.localeCompare(b.dueDate)
        return (a.dueDate || '9999').localeCompare(b.dueDate || '9999') || priorityRank[a.priority] - priorityRank[b.priority]
    })
}

const mockBridge: AppBridge = {
	async GetCompactMode() { return localStorage.getItem(COMPACT_MODE_KEY) === 'true' },
	async SetCompactMode(enabled) { localStorage.setItem(COMPACT_MODE_KEY, String(enabled)) },
    async GetBootstrap() {
        const state = load()
        const openTasks = state.tasks.filter((task) => !task.deletedAt && !task.completedAt)
        const listCounts = openTasks.reduce<Record<string, number>>((counts, task) => {
            if (task.listId) counts[task.listId] = (counts[task.listId] ?? 0) + 1
            return counts
        }, {})
        const tagCounts = openTasks.reduce<Record<string, number>>((counts, task) => {
            task.tags.forEach((tag) => { counts[tag.id] = (counts[tag.id] ?? 0) + 1 })
            return counts
        }, {})
        const activeNotes = state.notes.filter((note) => !note.deletedAt)
        const notebookCounts = activeNotes.reduce<Record<string, number>>((counts, note) => {
            counts[note.notebookId] = (counts[note.notebookId] ?? 0) + 1
            return counts
        }, {})
        const noteTagCounts = activeNotes.reduce<Record<string, number>>((counts, note) => {
            note.tags.forEach((tag) => { counts[tag.id] = (counts[tag.id] ?? 0) + 1 })
            return counts
        }, {})
        return {
            lists: state.lists,
            tags: state.tags,
            notebooks: state.notebooks.filter((notebook) => !notebook.deletedAt),
            noteTags: state.noteTags.filter((tag) => !tag.deletedAt),
            settings: state.settings,
            listCounts,
            tagCounts,
            notebookCounts,
            noteTagCounts,
            currentUser: {id: 'local', username: '本地用户', mode: 'local' as const},
            sync: {configured: false, online: false, syncing: false, pending: 0, serverUrl: '', lastSyncAt: '', lastError: ''},
            unreadCount: 0,
            counts: {
                inbox: state.tasks.filter((task) => !task.deletedAt && !task.completedAt && !task.listId).length,
                today: state.tasks.filter((task) => !task.deletedAt && !task.completedAt && Boolean(task.dueDate) && task.dueDate <= today()).length,
                upcoming: state.tasks.filter((task) => !task.deletedAt && !task.completedAt && task.dueDate > today()).length,
                all: openTasks.length,
                completed: state.tasks.filter((task) => !task.deletedAt && Boolean(task.completedAt)).length,
                notes: activeNotes.length,
            },
        }
    },
    async ListTasks(query) { return filterTasks(load().tasks, query) },
    async CreateTask(input: CreateTaskInput) {
        const state = load()
        const timestamp = iso()
        const task: Task = {
            id: uid(), title: input.title.trim(), notes: input.notes, listId: input.listId,
            priority: input.priority, dueDate: input.dueDate, completedAt: '', deletedAt: '',
            createdAt: timestamp, updatedAt: timestamp,
            tags: state.tags.filter((tag) => input.tagIds.includes(tag.id)),
        }
        state.tasks.push(task); save(state); return task
    },
    async UpdateTask(input) {
        const state = load()
        const index = state.tasks.findIndex((task) => task.id === input.id)
        if (index < 0) throw new Error('任务不存在')
        state.tasks[index] = {
            ...state.tasks[index], ...input, updatedAt: iso(),
            tags: state.tags.filter((tag) => input.tagIds.includes(tag.id)),
        }
        save(state); return state.tasks[index]
    },
    async SetTaskCompleted(id, completed) {
        const state = load(); const task = state.tasks.find((item) => item.id === id)
        if (!task) throw new Error('任务不存在')
        task.completedAt = completed ? iso() : ''; task.updatedAt = iso(); save(state); return task
    },
    async DeleteTask(id) {
        const state = load(); const task = state.tasks.find((item) => item.id === id)
        if (!task) throw new Error('任务不存在')
        task.deletedAt = iso(); save(state)
    },
    async RestoreTask(id) {
        const state = load(); const task = state.tasks.find((item) => item.id === id)
        if (!task) throw new Error('任务不存在')
        task.deletedAt = ''; save(state); return task
    },
    async ListNotebooks() { return load().notebooks.filter((item) => !item.deletedAt) },
    async CreateNotebook(name) {
        const state = load(); const timestamp = iso()
        const item: Notebook = {id: uid(), name: name.trim(), kind: 'personal', role: 'owner', ownerId: 'local', default: false, deletedAt: '', createdAt: timestamp, updatedAt: timestamp, serverVersion: 0}
        state.notebooks.push(item); save(state); return item
    },
    async UpdateNotebook(input) {
        const state = load(); const item = state.notebooks.find((notebook) => notebook.id === input.id && !notebook.deletedAt)
        if (!item) throw new Error('笔记本不存在')
        item.name = input.name.trim(); item.updatedAt = iso(); save(state); return item
    },
    async DeleteNotebook(id) {
        const state = load(); const item = state.notebooks.find((notebook) => notebook.id === id && !notebook.deletedAt)
        if (!item || item.default) throw new Error(item?.default ? '默认笔记本不能删除' : '笔记本不存在')
        const fallback = state.notebooks.find((notebook) => notebook.default)!
        state.notes.forEach((note) => { if (note.notebookId === id) note.notebookId = fallback.id })
        item.deletedAt = iso(); save(state)
    },
    async ListNoteTags(notebookId) { return load().noteTags.filter((tag) => !tag.deletedAt && (!notebookId || tag.notebookId === notebookId)) },
    async CreateNoteTag(input) {
        const state = load(); const timestamp = iso()
        const item: NoteTag = {id: uid(), notebookId: input.notebookId, name: input.name.trim(), color: input.color || 'coral', deletedAt: '', createdAt: timestamp, updatedAt: timestamp}
        state.noteTags.push(item); save(state); return item
    },
    async UpdateNoteTag(input) {
        const state = load(); const item = state.noteTags.find((tag) => tag.id === input.id && !tag.deletedAt)
        if (!item) throw new Error('笔记标签不存在')
        item.name = input.name.trim(); item.color = input.color; item.updatedAt = iso()
        state.notes.forEach((note) => note.tags.forEach((tag) => {if (tag.id === item.id) Object.assign(tag, item)}))
        save(state); return item
    },
    async DeleteNoteTag(id) {
        const state = load(); const item = state.noteTags.find((tag) => tag.id === id)
        if (!item) throw new Error('笔记标签不存在')
        item.deletedAt = iso(); state.notes.forEach((note) => {note.tags = note.tags.filter((tag) => tag.id !== id)}); save(state)
    },
    async ListNotes(query) { return filterNotes(load(), query) },
    async GetNote(id) {
        const note = load().notes.find((item) => item.id === id)
        if (!note) throw new Error('笔记不存在')
        return note
    },
    async CreateNote(input) {
        const state = load(); const timestamp = iso()
        const notebookId = input.notebookId || state.notebooks.find((item) => item.default)!.id
        const note: Note = {
            id: uid(), notebookId, title: input.title.trim() || '无标题笔记', content: input.content,
            pinned: false, revision: 1, deletedAt: '',
            tags: state.noteTags.filter((tag) => input.tagIds.includes(tag.id) && tag.notebookId === notebookId),
            tasks: state.tasks.filter((task) => input.taskIds.includes(task.id)).map((task) => ({id: task.id, title: task.title, dueDate: task.dueDate, completedAt: task.completedAt})),
            attachments: [], createdAt: timestamp, updatedAt: timestamp, serverVersion: 0,
        }
        state.notes.push(note); saveMockVersion(state, note); save(state); return note
    },
    async UpdateNote(input) {
        const state = load(); const note = state.notes.find((item) => item.id === input.id && !item.deletedAt)
        if (!note) throw new Error('笔记不存在')
        if (input.baseRevision && input.baseRevision !== note.revision) throw new Error('笔记已在其他位置更新，请重新加载')
        note.notebookId = input.notebookId || note.notebookId; note.title = input.title.trim() || '无标题笔记'; note.content = input.content
        note.tags = state.noteTags.filter((tag) => input.tagIds.includes(tag.id) && tag.notebookId === note.notebookId)
        note.tasks = state.tasks.filter((task) => input.taskIds.includes(task.id)).map((task) => ({id: task.id, title: task.title, dueDate: task.dueDate, completedAt: task.completedAt}))
        note.revision += 1; note.updatedAt = iso(); saveMockVersion(state, note); save(state); return note
    },
    async SetNotePinned(id, pinned) {
        const state = load(); const note = state.notes.find((item) => item.id === id && !item.deletedAt)
        if (!note) throw new Error('笔记不存在')
        note.pinned = pinned; note.revision += 1; note.updatedAt = iso(); save(state); return note
    },
    async DeleteNote(id) {
        const state = load(); const note = state.notes.find((item) => item.id === id && !item.deletedAt)
        if (!note) throw new Error('笔记不存在')
        note.deletedAt = iso(); save(state)
    },
    async RestoreNote(id) {
        const state = load(); const note = state.notes.find((item) => item.id === id)
        if (!note) throw new Error('笔记不存在')
        note.deletedAt = ''; note.updatedAt = iso(); save(state); return note
    },
    async ListNoteVersions(noteId) { return load().versions.filter((version) => version.noteId === noteId).sort((a, b) => b.createdAt.localeCompare(a.createdAt)) },
    async RestoreNoteVersion(versionId) {
        const state = load(); const version = state.versions.find((item) => item.id === versionId)
        const note = version && state.notes.find((item) => item.id === version.noteId)
        if (!version || !note) throw new Error('历史版本不存在')
        note.title = version.title; note.content = version.content; note.tags = version.tags; note.attachments = version.attachments; note.revision += 1; note.updatedAt = iso()
        saveMockVersion(state, note); save(state); return note
    },
    async ListComments(noteId) { return load().comments.filter((comment) => comment.noteId === noteId) },
    async CreateComment(input) {
        const state = load(); const timestamp = iso()
        const item: NoteComment = {id: uid(), noteId: input.noteId, parentId: input.parentId, authorId: 'local', authorName: '本地用户', content: input.content.trim(), deletedAt: '', createdAt: timestamp, updatedAt: timestamp}
        state.comments.push(item); save(state); return item
    },
    async UpdateComment(input) {
        const state = load(); const item = state.comments.find((comment) => comment.id === input.id)
        if (!item) throw new Error('评论不存在')
        item.content = input.content.trim(); item.updatedAt = iso(); save(state); return item
    },
    async DeleteComment(id) {
        const state = load(); const item = state.comments.find((comment) => comment.id === id)
        if (!item) throw new Error('评论不存在')
        item.deletedAt = iso(); save(state)
    },
    async ListNotebookMembers(notebookId) { return [{notebookId, userId: 'local', username: '本地用户', role: 'owner' as const, createdAt: iso()}] },
    async CreateNotebookInvite() { throw new Error('浏览器预览模式不支持协作邀请') },
    async UpdateNotebookMember() { throw new Error('浏览器预览模式不支持成员管理') },
    async RemoveNotebookMember() { throw new Error('浏览器预览模式不支持成员管理') },
    async TransferNotebookOwnership() { throw new Error('浏览器预览模式不支持所有权转让') },
    async AcceptInvitation() { throw new Error('浏览器预览模式不支持协作邀请') },
    async AcquireEditLease(noteId) { return {noteId, holderId: 'local', holderName: '本地用户', expiresAt: new Date(Date.now() + 120_000).toISOString(), editable: true} },
    async ReleaseEditLease() {},
    async AddNoteAttachments() { throw new Error('浏览器预览模式请使用粘贴或拖放添加附件') },
    async SavePastedAttachment(input) {
        const state = load(); const note = state.notes.find((item) => item.id === input.noteId)
        if (!note) throw new Error('笔记不存在')
        const timestamp = iso(); const size = Math.floor(input.data.length * 0.75)
        const item: Attachment = {id: uid(), notebookId: note.notebookId, fileName: input.fileName, mimeType: input.mimeType, size, sha256: '', status: 'local', remoteKey: '', deletedAt: '', createdAt: timestamp, updatedAt: timestamp}
        note.attachments.push(item); note.revision += 1; note.updatedAt = timestamp; state.attachmentData[item.id] = input.data; saveMockVersion(state, note); save(state); return item
    },
    async ReadAttachment(id) {
        const state = load(); const attachment = state.notes.flatMap((note) => note.attachments).find((item) => item.id === id)
        if (!attachment || !state.attachmentData[id]) throw new Error('附件不存在')
        return {mimeType: attachment.mimeType, data: state.attachmentData[id]}
    },
    async OpenAttachment() {},
    async DeleteAttachment(noteId, id) {
        const state = load(); const note = state.notes.find((item) => item.id === noteId)
        if (!note) throw new Error('笔记不存在')
        note.attachments = note.attachments.filter((item) => item.id !== id); delete state.attachmentData[id]; note.revision += 1; note.updatedAt = iso(); save(state)
    },
    async Login() { throw new Error('浏览器预览模式不支持连接同步服务器，请在桌面应用中使用') },
    async Logout() {},
    async SyncNow() { return {configured: false, online: false, syncing: false, pending: 0, serverUrl: '', lastSyncAt: '', lastError: ''} },
    async GetSyncStatus() { return {configured: false, online: false, syncing: false, pending: 0, serverUrl: '', lastSyncAt: '', lastError: ''} },
    async ListNotifications() { return [] },
    async MarkNotificationRead() {},
    async CreateList(name) {
        const state = load(); const timestamp = iso()
        const item = {id: uid(), name: name.trim(), position: state.lists.length, createdAt: timestamp, updatedAt: timestamp}
        state.lists.push(item); save(state); return item
    },
    async UpdateList(input) {
        const state = load(); const item = state.lists.find((list) => list.id === input.id)
        if (!item) throw new Error('清单不存在')
        item.name = input.name.trim(); item.updatedAt = iso(); save(state); return item
    },
    async DeleteList(id) {
        const state = load(); state.lists = state.lists.filter((list) => list.id !== id)
        state.tasks.forEach((task) => { if (task.listId === id) task.listId = '' }); save(state)
    },
    async CreateTag(input) {
        const state = load(); const timestamp = iso()
        const item = {id: uid(), name: input.name.trim(), color: input.color, createdAt: timestamp, updatedAt: timestamp}
        state.tags.push(item); save(state); return item
    },
    async UpdateTag(input) {
        const state = load(); const item = state.tags.find((tag) => tag.id === input.id)
        if (!item) throw new Error('标签不存在')
        item.name = input.name.trim(); item.color = input.color; item.updatedAt = iso()
        state.tasks.forEach((task) => task.tags.forEach((tag) => { if (tag.id === item.id) Object.assign(tag, item) }))
        save(state); return item
    },
    async DeleteTag(id) {
        const state = load(); state.tags = state.tags.filter((tag) => tag.id !== id)
        state.tasks.forEach((task) => { task.tags = task.tags.filter((tag) => tag.id !== id) }); save(state)
    },
    async GetOverview() { return mockOverview(load().tasks) },
    async UpdateSettings(settings) { const state = load(); state.settings = settings; save(state); return settings },
    async ExportBackup() {
        const state = load()
        const blob = new Blob([JSON.stringify({schemaVersion: 2, exportedAt: iso(), ...state}, null, 2)], {type: 'application/json'})
        const url = URL.createObjectURL(blob)
        const anchor = document.createElement('a')
        anchor.href = url; anchor.download = `TODO-backup-${today()}.json`; anchor.click(); URL.revokeObjectURL(url)
        return {path: anchor.download, cancelled: false, count: state.tasks.length + state.notes.length, tasks: state.tasks.length, notes: state.notes.length, attachments: state.notes.reduce((count, note) => count + note.attachments.length, 0)}
    },
    async ImportBackup(): Promise<ImportResult> { throw new Error('浏览器预览模式不支持恢复，请在桌面应用中使用') },
}

export const api: AppBridge = window.go?.backend?.App ?? mockBridge
export const isDesktop = Boolean(window.go?.backend?.App)
