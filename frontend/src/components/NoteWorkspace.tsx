import * as ContextMenu from '@radix-ui/react-context-menu'
import * as Dialog from '@radix-ui/react-dialog'
import * as Popover from '@radix-ui/react-popover'
import {AnimatePresence, motion, useReducedMotion} from 'motion/react'
import ReactMarkdown, {defaultUrlTransform} from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
    ArchiveRestore,
    Bold,
    BookOpen,
    Check,
    CheckSquare,
    ChevronRight,
    Clock3,
    Code2,
    Columns2,
    Download,
    Eye,
    File,
    FileText,
    Heading2,
    Image as ImageIcon,
    Italic,
    Link2,
    List,
    ListOrdered,
    MessageSquare,
    MoreHorizontal,
    Paperclip,
    Pin,
    PinOff,
    Plus,
    Quote,
    Save,
    Search,
    Tags,
    Trash2,
    Unlink,
    UsersRound,
    X,
} from 'lucide-react'
import {useCallback, useEffect, useMemo, useRef, useState} from 'react'
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query'
import {api} from '../lib/api'
import type {
    Attachment,
    Bootstrap,
    Note,
    NoteComment,
    NoteSummary,
    NoteVersion,
    Task,
    TaskReference,
    UpdateNoteInput,
} from '../types'
import {CustomSelect} from './CustomSelect'
import {IconButton} from './IconButton'

interface NoteWorkspaceProps {
    bootstrap?: Bootstrap
    notebookId: string
    noteTagId: string
    selectedId: string
    onSelect: (id: string) => void
    onOpenTask: (task: TaskReference) => void
    onRequestDelete: (note: NoteSummary | Note) => void
    showToast: (message: string) => void
}

type Drawer = 'history' | 'comments' | 'members' | null
type SaveState = 'saved' | 'dirty' | 'saving' | 'error'

const tagColors: Record<string, string> = {
    coral: '#e45d48', amber: '#c88b2a', mint: '#3f987d', blue: '#477eae', violet: '#8068a8', rose: '#b85f78',
}

function relativeDate(value: string) {
    const date = new Date(value)
    const diff = Date.now() - date.getTime()
    if (diff < 60_000) return '刚刚'
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
    if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
    return new Intl.DateTimeFormat('zh-CN', {month: 'numeric', day: 'numeric'}).format(date)
}

function formatBytes(size: number) {
    if (size < 1024) return `${size} B`
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
    return `${(size / 1024 / 1024).toFixed(1)} MB`
}

function AttachmentImage({id, alt, onOpen}: {id: string; alt: string; onOpen: (source: string, alt: string) => void}) {
    const query = useQuery({
        queryKey: ['attachment-content', id],
        queryFn: async () => {
            const result = await api.ReadAttachment(id)
            return `data:${result.mimeType};base64,${result.data}`
        },
    })
    if (query.isLoading) return <span className="markdown-image-loading"><ImageIcon size={18}/>图片加载中</span>
    if (!query.data) return <span className="markdown-image-error"><ImageIcon size={18}/>图片不可用</span>
    return <button className="markdown-image-button" aria-label={`查看图片${alt ? `：${alt}` : ''}`} onClick={() => onOpen(query.data, alt)}><img src={query.data} alt={alt}/></button>
}

function MarkdownPreview({content, attachments}: {content: string; attachments: Attachment[]}) {
    const attachmentIDs = useMemo(() => new Set(attachments.map((item) => item.id)), [attachments])
    const [lightbox, setLightbox] = useState<{source: string; alt: string} | null>(null)
    return (
        <div className="markdown-preview">
            <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                urlTransform={(url) => url.startsWith('attachment://') ? url : defaultUrlTransform(url)}
                components={{
                    img: ({src, alt}) => {
                        const id = src?.startsWith('attachment://') ? src.slice('attachment://'.length) : ''
                        if (!id || !attachmentIDs.has(id)) return <span className="markdown-image-error"><ImageIcon size={18}/>已阻止远程图片</span>
                        return <AttachmentImage id={id} alt={alt ?? ''} onOpen={(source, imageAlt) => setLightbox({source, alt: imageAlt})}/>
                    },
                    a: ({href, children}) => {
                        if (href?.startsWith('attachment://')) {
                            const id = href.slice('attachment://'.length)
                            return <button className="markdown-attachment-link" onClick={() => void api.OpenAttachment(id)}>{children}</button>
                        }
                        return <a href={href} target="_blank" rel="noreferrer">{children}</a>
                    },
                }}
            >{content || '*暂无正文*'}</ReactMarkdown>
            <Dialog.Root open={Boolean(lightbox)} onOpenChange={(open) => {if (!open) setLightbox(null)}}>
                <Dialog.Portal><Dialog.Overlay className="lightbox-overlay"/><Dialog.Content className="lightbox-content"><Dialog.Title className="sr-only">图片预览</Dialog.Title><Dialog.Description className="sr-only">查看笔记中的原始图片</Dialog.Description><Dialog.Close asChild><IconButton className="lightbox-close" label="关闭图片预览"><X size={20}/></IconButton></Dialog.Close>{lightbox && <img src={lightbox.source} alt={lightbox.alt}/>}</Dialog.Content></Dialog.Portal>
            </Dialog.Root>
        </div>
    )
}

function NoteRow({note, active, onSelect, onPin, onDelete}: {
    note: NoteSummary
    active: boolean
    onSelect: () => void
    onPin: () => void
    onDelete: () => void
}) {
    return (
        <ContextMenu.Root>
            <ContextMenu.Trigger asChild>
                <motion.button layout className={`note-row ${active ? 'is-active' : ''}`} onClick={onSelect}>
                    <span className="note-row-heading"><strong>{note.title}</strong>{note.pinned && <Pin size={13}/>}</span>
                    <span className="note-row-excerpt">{note.excerpt || '暂无正文'}</span>
                    <span className="note-row-meta">
                        <time>{relativeDate(note.updatedAt)}</time>
                        {note.attachmentCount > 0 && <span><Paperclip size={12}/>{note.attachmentCount}</span>}
                        {note.linkedTaskCount > 0 && <span><CheckSquare size={12}/>{note.linkedTaskCount}</span>}
                        {note.commentCount > 0 && <span><MessageSquare size={12}/>{note.commentCount}</span>}
                    </span>
                </motion.button>
            </ContextMenu.Trigger>
            <ContextMenu.Portal>
                <ContextMenu.Content className="menu-content task-context-menu">
                    <ContextMenu.Item className="menu-item" onSelect={onSelect}><FileText size={15}/>打开笔记</ContextMenu.Item>
                    <ContextMenu.Item className="menu-item" onSelect={onPin}>{note.pinned ? <PinOff size={15}/> : <Pin size={15}/>} {note.pinned ? '取消置顶' : '置顶'}</ContextMenu.Item>
                    <ContextMenu.Separator className="menu-separator"/>
                    <ContextMenu.Item className="menu-item danger" onSelect={onDelete}><Trash2 size={15}/>删除笔记</ContextMenu.Item>
                </ContextMenu.Content>
            </ContextMenu.Portal>
        </ContextMenu.Root>
    )
}

function ToolbarButton({label, onClick, children}: {label: string; onClick: () => void; children: React.ReactNode}) {
    return <IconButton label={label} side="bottom" onClick={onClick}>{children}</IconButton>
}

function TaskPicker({tasks, selected, onChange}: {tasks: Task[]; selected: string[]; onChange: (ids: string[]) => void}) {
    const [search, setSearch] = useState('')
    const visible = tasks.filter((task) => `${task.title} ${task.notes}`.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()))
    return (
        <Popover.Root>
            <Popover.Trigger asChild><button className="note-property-button"><CheckSquare size={15}/>关联任务{selected.length > 0 && <span>{selected.length}</span>}<ChevronRight size={14}/></button></Popover.Trigger>
            <Popover.Portal>
                <Popover.Content className="note-picker-popover" sideOffset={6} align="end">
                    <div className="picker-search"><Search size={15}/><input aria-label="搜索关联任务" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索任务"/></div>
                    <div className="picker-options">
                        {visible.length === 0 && <p>没有匹配的任务</p>}
                        {visible.map((task) => {
                            const checked = selected.includes(task.id)
                            return <button key={task.id} className={checked ? 'is-selected' : ''} onClick={() => onChange(checked ? selected.filter((id) => id !== task.id) : [...selected, task.id])}><span className="picker-check">{checked && <Check size={13}/>}</span><span>{task.title}<small>{task.completedAt ? '已完成' : task.dueDate || '未安排日期'}</small></span></button>
                        })}
                    </div>
                    <Popover.Arrow className="popover-arrow"/>
                </Popover.Content>
            </Popover.Portal>
        </Popover.Root>
    )
}

function SideDrawer({drawer, note, bootstrap, onClose, onRestored, onAccessChanged, showToast}: {
    drawer: Drawer
    note: Note
    bootstrap?: Bootstrap
    onClose: () => void
    onRestored: (note: Note) => void
    onAccessChanged: () => void
    showToast: (message: string) => void
}) {
    const client = useQueryClient()
    const versions = useQuery({queryKey: ['note-versions', note.id], queryFn: () => api.ListNoteVersions(note.id), enabled: drawer === 'history'})
    const comments = useQuery({queryKey: ['note-comments', note.id], queryFn: () => api.ListComments(note.id), enabled: drawer === 'comments'})
    const members = useQuery({queryKey: ['notebook-members', note.notebookId], queryFn: () => api.ListNotebookMembers(note.notebookId), enabled: drawer === 'members'})
    const [comment, setComment] = useState('')
    const [replyTo, setReplyTo] = useState<NoteComment | null>(null)
    const [editingComment, setEditingComment] = useState<NoteComment | null>(null)
    const [inviteUsername, setInviteUsername] = useState('')
    const [inviteRole, setInviteRole] = useState<'editor' | 'viewer'>('editor')
    const [inviteToken, setInviteToken] = useState('')
    const [memberAction, setMemberAction] = useState<{kind: 'remove' | 'transfer' | 'leave'; member: {userId: string; username: string}} | null>(null)
    const [memberBusy, setMemberBusy] = useState(false)
    if (!drawer) return null
    const heading = drawer === 'history' ? '历史版本' : drawer === 'comments' ? '评论' : '协作者'
    const restore = async (version: NoteVersion) => {
        try {
            const restored = await api.RestoreNoteVersion(version.id)
            await client.invalidateQueries({queryKey: ['notes']})
            onRestored(restored); showToast('已恢复历史版本')
        } catch (error) { showToast(error instanceof Error ? error.message : '恢复失败') }
    }
    const submitComment = async () => {
        if (!comment.trim()) return
        try {
            if (editingComment) await api.UpdateComment({id: editingComment.id, noteId: note.id, parentId: editingComment.parentId, content: comment.trim()})
            else await api.CreateComment({id: '', noteId: note.id, parentId: replyTo?.id ?? '', content: comment.trim()})
            setComment(''); setReplyTo(null); setEditingComment(null); await client.invalidateQueries({queryKey: ['note-comments', note.id]}); await client.invalidateQueries({queryKey: ['notes']})
        } catch (error) { showToast(error instanceof Error ? error.message : '评论失败') }
    }
    const deleteComment = async (item: NoteComment) => {
        try {
            await api.DeleteComment(item.id)
            await client.invalidateQueries({queryKey: ['note-comments', note.id]}); await client.invalidateQueries({queryKey: ['notes']})
        } catch (error) {showToast(error instanceof Error ? error.message : '删除评论失败')}
    }
    const createInvite = async () => {
        try {
            const result = await api.CreateNotebookInvite({notebookId: note.notebookId, username: inviteUsername.trim(), role: inviteRole})
            setInviteToken(result.token); showToast('邀请已创建，有效期 7 天')
        } catch (error) { showToast(error instanceof Error ? error.message : '创建邀请失败') }
    }
    const notebook = bootstrap?.notebooks.find((item) => item.id === note.notebookId)
    const refreshMembers = async () => {
        await Promise.all([client.invalidateQueries({queryKey: ['notebook-members', note.notebookId]}), client.invalidateQueries({queryKey: ['bootstrap']}), client.invalidateQueries({queryKey: ['notes']})])
    }
    const updateMemberRole = async (userId: string, role: 'editor' | 'viewer') => {
        try {
            await api.UpdateNotebookMember({notebookId: note.notebookId, userId, role})
            await refreshMembers(); showToast('成员权限已更新')
        } catch (error) {showToast(error instanceof Error ? error.message : '更新成员失败')}
    }
    const confirmMemberAction = async () => {
        if (!memberAction) return
        setMemberBusy(true)
        try {
            if (memberAction.kind === 'transfer') await api.TransferNotebookOwnership(note.notebookId, memberAction.member.userId)
            else await api.RemoveNotebookMember(note.notebookId, memberAction.member.userId)
            const leaving = memberAction.kind === 'leave'
            await refreshMembers(); setMemberAction(null)
            showToast(leaving ? '已离开共享笔记本' : memberAction.kind === 'transfer' ? '所有权已转让' : '成员已移除')
            if (leaving) onAccessChanged()
        } catch (error) {showToast(error instanceof Error ? error.message : '成员操作失败')}
        finally {setMemberBusy(false)}
    }
    return (
        <motion.aside className="note-drawer" initial={{opacity: 0, x: 24}} animate={{opacity: 1, x: 0}} exit={{opacity: 0, x: 18}}>
            <header><strong>{heading}</strong><IconButton label="关闭" onClick={onClose}><X size={17}/></IconButton></header>
            <div className="note-drawer-content">
                {drawer === 'history' && (versions.data?.length ? versions.data.map((version) => <article className="history-item" key={version.id}><div><strong>{version.title}</strong><span>{relativeDate(version.createdAt)} · {version.actorName}</span></div><p>{version.content.replace(/\s+/g, ' ').slice(0, 110) || '空白版本'}</p><button className="secondary-button" onClick={() => void restore(version)}><ArchiveRestore size={14}/>恢复</button></article>) : <p className="drawer-empty">暂无历史版本</p>)}
                {drawer === 'comments' && <>
                    <div className="comment-list">{comments.data?.filter((item) => !item.deletedAt).map((item: NoteComment) => {
                        const own = item.authorId === bootstrap?.currentUser.id || item.authorId === 'local'
                        const parent = item.parentId ? comments.data?.find((candidate) => candidate.id === item.parentId) : null
                        return <article className="comment-item" key={item.id}><span>{item.authorName.slice(0, 1)}</span><div><strong>{item.authorName}<time>{relativeDate(item.createdAt)}</time></strong>{parent && <small className="comment-reply-context">回复 {parent.authorName}</small>}<p>{item.content}</p>{notebook?.role !== 'viewer' && <div className="comment-actions"><button onClick={() => {setReplyTo(item); setEditingComment(null); setComment('')}}>回复</button>{own && <><button onClick={() => {setEditingComment(item); setReplyTo(null); setComment(item.content)}}>编辑</button><button className="danger-text" onClick={() => void deleteComment(item)}>删除</button></>}</div>}</div></article>
                    })}{comments.data?.filter((item) => !item.deletedAt).length === 0 && <p className="drawer-empty">还没有评论</p>}</div>
                    {notebook?.role !== 'viewer' ? <div className="comment-composer">{(replyTo || editingComment) && <div className="comment-composer-context"><span>{editingComment ? '编辑评论' : `回复 ${replyTo?.authorName}`}</span><button aria-label="取消评论操作" onClick={() => {setReplyTo(null); setEditingComment(null); setComment('')}}><X size={13}/></button></div>}<textarea aria-label="添加评论" value={comment} onChange={(event) => setComment(event.target.value)} placeholder="评论或使用 @用户名 提醒成员"/><button className="primary-button" disabled={!comment.trim()} onClick={() => void submitComment()}>{editingComment ? '保存' : '发送'}</button></div> : <p className="drawer-permission-note">只读成员不能发表评论</p>}
                </>}
                {drawer === 'members' && <>
                    <div className="member-list">{members.data?.map((member) => {
                        const current = member.userId === bootstrap?.currentUser.id
                        return <article className="member-item" key={member.userId}>
                            <span>{member.username.slice(0, 1)}</span>
                            <div><strong>{member.username}{current && <small>你</small>}</strong><small>{member.role === 'owner' ? '所有者' : member.role === 'editor' ? '编辑者' : '只读成员'}</small></div>
                            {notebook?.role === 'owner' && member.role !== 'owner' && <div className="member-actions"><CustomSelect ariaLabel={`${member.username}的权限`} value={member.role} options={[{value: 'editor', label: '编辑者'}, {value: 'viewer', label: '只读'}]} onValueChange={(role) => {if (role === 'editor' || role === 'viewer') void updateMemberRole(member.userId, role)}}/><button onClick={() => setMemberAction({kind: 'transfer', member})}>转让</button><button className="danger-text" onClick={() => setMemberAction({kind: 'remove', member})}>移除</button></div>}
                            {current && member.role !== 'owner' && <button className="member-leave" onClick={() => setMemberAction({kind: 'leave', member})}>离开</button>}
                        </article>
                    })}</div>
                    {memberAction && <div className="member-confirm"><strong>{memberAction.kind === 'transfer' ? `将所有权转让给 ${memberAction.member.username}？` : memberAction.kind === 'leave' ? '离开这个共享笔记本？' : `移除 ${memberAction.member.username}？`}</strong><p>{memberAction.kind === 'transfer' ? '转让后你将变为编辑者。' : '成员将无法继续访问这里的内容。'}</p><div><button className="secondary-button" disabled={memberBusy} onClick={() => setMemberAction(null)}>取消</button><button className="danger-button" disabled={memberBusy} onClick={() => void confirmMemberAction()}>{memberBusy ? '处理中' : '确认'}</button></div></div>}
                    {notebook?.role === 'owner' && bootstrap?.sync.configured && <div className="invite-form"><strong>邀请成员</strong><input aria-label="邀请用户名" value={inviteUsername} onChange={(event) => setInviteUsername(event.target.value)} placeholder="用户名，可留空生成通用邀请"/><div><CustomSelect ariaLabel="邀请权限" value={inviteRole} options={[{value: 'editor', label: '编辑者'}, {value: 'viewer', label: '只读成员'}]} onValueChange={setInviteRole}/><button className="secondary-button" onClick={() => void createInvite()}>生成邀请</button></div>{inviteToken && <div className="invite-token"><code>{inviteToken}</code><button onClick={() => void navigator.clipboard.writeText(inviteToken).then(() => showToast('邀请令牌已复制'))}>复制</button></div>}</div>}
                </>}
            </div>
        </motion.aside>
    )
}

function NoteEditor({note, bootstrap, tasks, onSaved, onClose, onOpenTask, onDelete, showToast}: {
    note: Note
    bootstrap?: Bootstrap
    tasks: Task[]
    onSaved: (note: Note) => void
    onClose: () => void
    onOpenTask: (task: TaskReference) => void
    onDelete: () => void
    showToast: (message: string) => void
}) {
    const [title, setTitle] = useState(note.title)
    const [content, setContent] = useState(note.content)
    const [notebookId, setNotebookId] = useState(note.notebookId)
    const [tagIds, setTagIds] = useState(note.tags.map((tag) => tag.id))
    const [taskIds, setTaskIds] = useState(note.tasks.map((task) => task.id))
    const [revision, setRevision] = useState(note.revision)
    const [mode, setMode] = useState<'edit' | 'preview'>('edit')
    const [saveState, setSaveState] = useState<SaveState>('saved')
    const [drawer, setDrawer] = useState<Drawer>(null)
    const [leaseEditable, setLeaseEditable] = useState(true)
    const [attachments, setAttachments] = useState(note.attachments)
    const textareaRef = useRef<HTMLTextAreaElement>(null)
    const savingRef = useRef(false)
    const pendingRef = useRef(false)
    const stateRef = useRef({title, content, notebookId, tagIds, taskIds, revision})
    const reduce = useReducedMotion()
    const draftKey = `todo-note-draft:${note.id}`
    const editable = leaseEditable && bootstrap?.notebooks.find((item) => item.id === notebookId)?.role !== 'viewer'

    useEffect(() => {
        const draft = localStorage.getItem(draftKey)
        setTitle(note.title); setContent(note.content); setNotebookId(note.notebookId); setTagIds(note.tags.map((tag) => tag.id)); setTaskIds(note.tasks.map((task) => task.id)); setRevision(note.revision); setAttachments(note.attachments); setSaveState('saved')
        if (draft) {
            try {
                const parsed = JSON.parse(draft) as typeof stateRef.current
                if (parsed.revision === note.revision) {
                    setTitle(parsed.title); setContent(parsed.content); setNotebookId(parsed.notebookId); setTagIds(parsed.tagIds); setTaskIds(parsed.taskIds); setSaveState('dirty'); showToast('已恢复未保存的笔记内容')
                }
            } catch { localStorage.removeItem(draftKey) }
        }
        void api.AcquireEditLease(note.id, false).then((lease) => setLeaseEditable(lease.editable)).catch(() => setLeaseEditable(true))
        return () => { void api.ReleaseEditLease(note.id) }
    }, [note.id])

    useEffect(() => { stateRef.current = {title, content, notebookId, tagIds, taskIds, revision} }, [title, content, notebookId, tagIds, taskIds, revision])
    const originalTagIds = note.tags.map((tag) => tag.id).sort().join(',')
    const originalTaskIds = note.tasks.map((task) => task.id).sort().join(',')
    const dirty = title !== note.title || content !== note.content || notebookId !== note.notebookId || [...tagIds].sort().join(',') !== originalTagIds || [...taskIds].sort().join(',') !== originalTaskIds

    useEffect(() => {
        if (!dirty || !editable) return
        setSaveState('dirty')
        localStorage.setItem(draftKey, JSON.stringify(stateRef.current))
        const timer = window.setTimeout(() => void save(), 700)
        return () => window.clearTimeout(timer)
    }, [title, content, notebookId, tagIds.join(','), taskIds.join(','), editable])

    const save = useCallback(async () => {
        if (!editable) return
        if (savingRef.current) { pendingRef.current = true; return }
        const snapshot = stateRef.current
        savingRef.current = true; setSaveState('saving')
        try {
            const saved = await api.UpdateNote({id: note.id, title: snapshot.title, content: snapshot.content, notebookId: snapshot.notebookId, tagIds: snapshot.tagIds, taskIds: snapshot.taskIds, baseRevision: snapshot.revision})
            setRevision(saved.revision); stateRef.current.revision = saved.revision; setAttachments(saved.attachments); setSaveState('saved'); localStorage.removeItem(draftKey); onSaved(saved)
        } catch (error) {
            setSaveState('error'); showToast(error instanceof Error ? error.message : '保存笔记失败')
        } finally {
            savingRef.current = false
            if (pendingRef.current) { pendingRef.current = false; window.setTimeout(() => void save(), 0) }
        }
    }, [editable, note.id, onSaved, showToast])

    useEffect(() => {
        const onKeyDown = (event: KeyboardEvent) => {
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') { event.preventDefault(); void save() }
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'b' && mode === 'edit') { event.preventDefault(); wrap('**', '**', '粗体') }
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'i' && mode === 'edit') { event.preventDefault(); wrap('*', '*', '斜体') }
        }
        window.addEventListener('keydown', onKeyDown)
        return () => window.removeEventListener('keydown', onKeyDown)
    }, [save, mode])

    const wrap = (before: string, after = '', placeholder = '') => {
        const textarea = textareaRef.current
        if (!textarea) return
        const start = textarea.selectionStart, end = textarea.selectionEnd
        const selected = content.slice(start, end) || placeholder
        const next = `${content.slice(0, start)}${before}${selected}${after}${content.slice(end)}`
        setContent(next)
        window.setTimeout(() => {textarea.focus(); textarea.setSelectionRange(start + before.length, start + before.length + selected.length)}, 0)
    }
    const prefixLines = (prefix: string) => {
        const textarea = textareaRef.current
        if (!textarea) return
        const start = content.lastIndexOf('\n', textarea.selectionStart - 1) + 1
        const endIndex = content.indexOf('\n', textarea.selectionEnd)
        const end = endIndex < 0 ? content.length : endIndex
        const next = content.slice(start, end).split('\n').map((line, index) => prefix.replace('{n}', String(index + 1)) + line).join('\n')
        setContent(content.slice(0, start) + next + content.slice(end))
    }
    const insertAttachment = (attachment: Attachment) => {
        const markdown = attachment.mimeType.startsWith('image/') ? `\n![${attachment.fileName}](attachment://${attachment.id})\n` : `\n[${attachment.fileName}](attachment://${attachment.id})\n`
        setContent((current) => current + markdown); setAttachments((current) => [...current, attachment])
    }
    const refreshAfterAttachment = async () => {
        const saved = await api.GetNote(note.id)
        setRevision(saved.revision); stateRef.current.revision = saved.revision; setAttachments(saved.attachments); onSaved(saved)
    }
    const addNativeAttachments = async () => {
        try {
            const items = await api.AddNoteAttachments(note.id)
            items.forEach(insertAttachment); await refreshAfterAttachment()
        } catch (error) { showToast(error instanceof Error ? error.message : '添加附件失败') }
    }
    const addFile = async (file: globalThis.File) => {
        if (file.size > 25 * 1024 * 1024) { showToast('附件不能超过 25MB'); return }
        const data = await new Promise<string>((resolve, reject) => {
            const reader = new FileReader(); reader.onerror = () => reject(reader.error); reader.onload = () => resolve(String(reader.result).split(',')[1] ?? ''); reader.readAsDataURL(file)
        })
        try { insertAttachment(await api.SavePastedAttachment({noteId: note.id, fileName: file.name || `粘贴图片-${Date.now()}.png`, mimeType: file.type || 'application/octet-stream', data})); await refreshAfterAttachment() }
        catch (error) { showToast(error instanceof Error ? error.message : '添加附件失败') }
    }
    const removeAttachment = async (attachment: Attachment) => {
        try {
            await api.DeleteAttachment(note.id, attachment.id)
            setAttachments((current) => current.filter((item) => item.id !== attachment.id))
            setContent((current) => current.split('\n').filter((line) => !line.includes(`attachment://${attachment.id}`)).join('\n'))
            const saved = await api.GetNote(note.id)
            setRevision(saved.revision); stateRef.current.revision = saved.revision; onSaved(saved)
        } catch (error) { showToast(error instanceof Error ? error.message : '删除附件失败') }
    }
    const availableTags = bootstrap?.noteTags.filter((tag) => tag.notebookId === notebookId) ?? []
    const saveCopy = saveState === 'saving' ? '保存中' : saveState === 'dirty' ? '有未保存更改' : saveState === 'error' ? '保存失败' : '已保存'
    return (
        <motion.section className="note-editor" aria-label="笔记编辑器" initial={reduce ? false : {opacity: 0, x: 20}} animate={{opacity: 1, x: 0}}>
            <header className="note-editor-header">
                <span className={`save-status is-${saveState}`}>{saveCopy}</span>
                <div className="note-editor-actions">
                    <IconButton label="评论" onClick={() => setDrawer(drawer === 'comments' ? null : 'comments')}><MessageSquare size={17}/></IconButton>
                    <IconButton label="历史版本" onClick={() => setDrawer(drawer === 'history' ? null : 'history')}><Clock3 size={17}/></IconButton>
                    <IconButton label="协作者" onClick={() => setDrawer(drawer === 'members' ? null : 'members')}><UsersRound size={17}/></IconButton>
                    <IconButton label="保存笔记" onClick={() => void save()}><Save size={17}/></IconButton>
                    <IconButton label="删除笔记" onClick={onDelete}><Trash2 size={17}/></IconButton>
                    <IconButton label="关闭编辑器" onClick={onClose}><X size={18}/></IconButton>
                </div>
            </header>
            {!leaseEditable && <div className="lease-warning"><UsersRound size={15}/>另一位成员正在编辑。当前为只读模式。</div>}
            <div className="note-editor-top">
                <textarea aria-label="笔记标题" value={title} disabled={!editable} onChange={(event) => setTitle(event.target.value)} rows={2} maxLength={240}/>
                <div className="note-properties">
                    <CustomSelect ariaLabel="笔记本" value={notebookId} options={(bootstrap?.notebooks ?? []).filter((item) => item.role !== 'viewer').map((item) => ({value: item.id, label: item.name}))} onValueChange={(value) => {setNotebookId(value); setTagIds([])}} className="note-notebook-select"/>
                    <TaskPicker tasks={tasks} selected={taskIds} onChange={setTaskIds}/>
                </div>
                <div className="note-linked-tasks">{tasks.filter((task) => taskIds.includes(task.id)).map((task) => <button key={task.id} onClick={() => onOpenTask({id: task.id, title: task.title, dueDate: task.dueDate, completedAt: task.completedAt})}><CheckSquare size={13}/>{task.title}</button>)}</div>
                {availableTags.length > 0 && <div className="note-tag-selector"><Tags size={14}/>{availableTags.map((tag) => <button key={tag.id} style={{'--tag-color': tagColors[tag.color] ?? tagColors.coral} as React.CSSProperties} className={tagIds.includes(tag.id) ? 'is-selected' : ''} onClick={() => setTagIds(tagIds.includes(tag.id) ? tagIds.filter((id) => id !== tag.id) : [...tagIds, tag.id])}>{tagIds.includes(tag.id) && <Check size={12}/>} {tag.name}</button>)}</div>}
                {attachments.length > 0 && <div className="note-attachments">{attachments.map((attachment) => <div key={attachment.id}><button className="attachment-main" onClick={() => void api.OpenAttachment(attachment.id)}>{attachment.mimeType.startsWith('image/') ? <ImageIcon size={14}/> : <File size={14}/>}<span>{attachment.fileName}<small>{formatBytes(attachment.size)}</small></span></button><IconButton label="移除附件" onClick={() => void removeAttachment(attachment)}><X size={14}/></IconButton></div>)}</div>}
            </div>
            <div className="note-editor-toolbar">
                <div className="editor-mode-control"><button className={mode === 'edit' ? 'is-active' : ''} onClick={() => setMode('edit')}><Columns2 size={14}/>编辑</button><button className={mode === 'preview' ? 'is-active' : ''} onClick={() => setMode('preview')}><Eye size={14}/>预览</button></div>
                <div className="markdown-tools" aria-label="Markdown 工具栏">
                    <ToolbarButton label="二级标题" onClick={() => prefixLines('## ')}><Heading2 size={16}/></ToolbarButton>
                    <ToolbarButton label="粗体" onClick={() => wrap('**', '**', '粗体')}><Bold size={16}/></ToolbarButton>
                    <ToolbarButton label="斜体" onClick={() => wrap('*', '*', '斜体')}><Italic size={16}/></ToolbarButton>
                    <ToolbarButton label="无序列表" onClick={() => prefixLines('- ')}><List size={16}/></ToolbarButton>
                    <ToolbarButton label="有序列表" onClick={() => prefixLines('{n}. ')}><ListOrdered size={16}/></ToolbarButton>
                    <ToolbarButton label="任务列表" onClick={() => prefixLines('- [ ] ')}><CheckSquare size={16}/></ToolbarButton>
                    <ToolbarButton label="引用" onClick={() => prefixLines('> ')}><Quote size={16}/></ToolbarButton>
                    <ToolbarButton label="行内代码" onClick={() => wrap('`', '`', '代码')}><Code2 size={16}/></ToolbarButton>
                    <ToolbarButton label="链接" onClick={() => wrap('[', '](https://)', '链接文字')}><Link2 size={16}/></ToolbarButton>
                    <ToolbarButton label="添加附件" onClick={() => void addNativeAttachments()}><Paperclip size={16}/></ToolbarButton>
                </div>
            </div>
            <div className="note-editor-body" onDragOver={(event) => event.preventDefault()} onDrop={(event) => {event.preventDefault(); Array.from(event.dataTransfer.files).forEach((file) => void addFile(file))}}>
                {mode === 'edit' ? <textarea ref={textareaRef} aria-label="笔记正文" disabled={!editable} value={content} onChange={(event) => setContent(event.target.value)} onPaste={(event) => {Array.from(event.clipboardData.files).forEach((file) => void addFile(file))}} placeholder="开始记录，支持 Markdown..."/> : <MarkdownPreview content={content} attachments={attachments}/>} 
            </div>
            <footer className="note-editor-footer"><span>{content.length} 字符</span><span>更新于 {relativeDate(note.updatedAt)}</span></footer>
            <AnimatePresence>{drawer && <SideDrawer drawer={drawer} note={{...note, title, content, notebookId, tags: availableTags.filter((tag) => tagIds.includes(tag.id)), attachments}} bootstrap={bootstrap} onClose={() => setDrawer(null)} onRestored={onSaved} onAccessChanged={onClose} showToast={showToast}/>}</AnimatePresence>
        </motion.section>
    )
}

export function NoteWorkspace(props: NoteWorkspaceProps) {
    const client = useQueryClient()
    const reduce = useReducedMotion()
    const [search, setSearch] = useState('')
    const [pinnedOnly, setPinnedOnly] = useState(false)
    const selectedNotebook = props.bootstrap?.notebooks.find((item) => item.id === props.notebookId)
    const canCreate = selectedNotebook?.role !== 'viewer'
    const notesQuery = useQuery({queryKey: ['notes', props.notebookId, props.noteTagId, search, pinnedOnly], queryFn: () => api.ListNotes({notebookId: props.notebookId, tagId: props.noteTagId, taskId: '', search, pinnedOnly})})
    const noteQuery = useQuery({queryKey: ['note', props.selectedId], queryFn: () => api.GetNote(props.selectedId), enabled: Boolean(props.selectedId)})
    const tasksQuery = useQuery({queryKey: ['note-task-options'], queryFn: () => api.ListTasks({view: 'all', listId: '', tagId: '', priority: 'all', search: '', sort: 'created', status: 'all'})})
    const create = useMutation({
        mutationFn: async () => {
            const notebookId = props.notebookId || props.bootstrap?.notebooks.find((item) => item.default)?.id || props.bootstrap?.notebooks[0]?.id || ''
            return api.CreateNote({notebookId, title: '无标题笔记', content: '', tagIds: props.noteTagId ? [props.noteTagId] : [], taskIds: []})
        },
        onSuccess: async (note) => {await Promise.all([client.invalidateQueries({queryKey: ['notes']}), client.invalidateQueries({queryKey: ['bootstrap']})]); props.onSelect(note.id)},
        onError: (error) => props.showToast(error instanceof Error ? error.message : '新建笔记失败'),
    })
    const pin = async (note: NoteSummary) => {
        try {await api.SetNotePinned(note.id, !note.pinned); await Promise.all([client.invalidateQueries({queryKey: ['notes']}), client.invalidateQueries({queryKey: ['note', note.id]})])}
        catch (error) {props.showToast(error instanceof Error ? error.message : '置顶失败')}
    }
    const selectedSummary = notesQuery.data?.find((item) => item.id === props.selectedId)
    return (
        <section className="notes-workspace">
            <div className="notes-list-panel">
                <header className="notes-list-header"><div><h1>笔记</h1><p>{props.notebookId ? selectedNotebook?.name : '所有笔记本'}</p></div><button data-new-note className="primary-button note-create-button" onClick={() => create.mutate()} disabled={create.isPending || !canCreate}><Plus size={16}/>新建</button></header>
                <div className="notes-toolbar">
                    <label className="search-field"><Search size={16}/><span className="sr-only">搜索笔记</span><input id="note-search" aria-label="搜索笔记" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索标题和正文"/></label>
                    <div className="note-filter-control"><button className={!pinnedOnly ? 'is-active' : ''} onClick={() => setPinnedOnly(false)}>全部</button><button className={pinnedOnly ? 'is-active' : ''} onClick={() => setPinnedOnly(true)}><Pin size={13}/>置顶</button></div>
                </div>
                <div className="notes-list" role="listbox" aria-label="笔记列表">
                    {notesQuery.isLoading && Array.from({length: 5}, (_, index) => <div className="note-skeleton" key={index}><i/><i/><i/></div>)}
                    {notesQuery.error && <div className="state-view error-state"><strong>无法加载笔记</strong><p>{notesQuery.error.message}</p></div>}
                    {!notesQuery.isLoading && notesQuery.data?.length === 0 && <div className="state-view notes-empty"><span className="state-icon"><BookOpen size={24}/></span><strong>{search ? '没有匹配的笔记' : canCreate ? '开始记录第一篇笔记' : '这个笔记本还没有内容'}</strong><p>{search ? '尝试其他关键词或筛选条件。' : canCreate ? '笔记会保存在本地，并可关联任务。' : '你当前拥有只读权限。'}</p>{!search && canCreate && <button className="secondary-button" onClick={() => create.mutate()}><Plus size={15}/>新建笔记</button>}</div>}
                    <AnimatePresence initial={false}>{notesQuery.data?.map((note) => <NoteRow key={note.id} note={note} active={props.selectedId === note.id} onSelect={() => props.onSelect(note.id)} onPin={() => void pin(note)} onDelete={() => props.onRequestDelete(note)}/>)}</AnimatePresence>
                </div>
            </div>
            <div className={`note-editor-slot ${noteQuery.data ? 'is-open' : ''}`}>
                <AnimatePresence mode="wait">
                    {noteQuery.data ? <NoteEditor key={noteQuery.data.id} note={noteQuery.data} bootstrap={props.bootstrap} tasks={tasksQuery.data ?? []} onSaved={(saved) => {client.setQueryData(['note', saved.id], saved); void client.invalidateQueries({queryKey: ['notes']}); void client.invalidateQueries({queryKey: ['bootstrap']})}} onClose={() => props.onSelect('')} onOpenTask={props.onOpenTask} onDelete={() => props.onRequestDelete(selectedSummary ?? noteQuery.data)} showToast={props.showToast}/> : <div className="detail-placeholder note-placeholder"><span><FileText size={24}/></span><strong>选择一篇笔记</strong><p>在这里编辑 Markdown、附件和关联任务。</p></div>}
                </AnimatePresence>
            </div>
            {noteQuery.data && <button className="note-editor-backdrop" aria-label="关闭笔记编辑器" onClick={() => props.onSelect('')}/>} 
        </section>
    )
}
