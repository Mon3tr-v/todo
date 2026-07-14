import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import {motion, useReducedMotion} from 'motion/react'
import {
    BarChart3,
    Bell,
    CalendarClock,
    CalendarDays,
    Check,
    CheckCircle2,
    Inbox,
    Layers3,
    ListTodo,
    MoreHorizontal,
    NotebookPen,
    BookOpen,
    Pencil,
    PictureInPicture2,
    Plus,
    Settings,
    Tag as TagIcon,
    Trash2,
    UsersRound,
} from 'lucide-react'
import type {Bootstrap, Notebook, NoteTag, Tag, TodoList, ViewKey} from '../types'
import {IconButton} from './IconButton'

interface Selection { view: ViewKey; listId: string; tagId: string; notebookId: string; noteTagId: string }

interface SidebarProps {
    bootstrap?: Bootstrap
    selection: Selection
    onSelect: (view: ViewKey, id?: string) => void
    onSelectTag: (tagId: string) => void
    onAddList: () => void
    onEditList: (list: TodoList) => void
    onDeleteList: (list: TodoList) => void
    onAddTag: () => void
    onEditTag: (tag: Tag) => void
    onDeleteTag: (tag: Tag) => void
    onSelectNotebook: (notebookId: string) => void
    onSelectNoteTag: (tagId: string) => void
    onAddNotebook: () => void
    onEditNotebook: (notebook: Notebook) => void
    onDeleteNotebook: (notebook: Notebook) => void
    onAddNoteTag: () => void
    onEditNoteTag: (tag: NoteTag) => void
    onDeleteNoteTag: (tag: NoteTag) => void
    onOpenSettings: () => void
    onOpenNotifications: () => void
    onOpenCompact: () => void
}

const tagColors: Record<string, string> = {
    coral: '#e45d48', amber: '#c88b2a', mint: '#3f987d', blue: '#477eae', violet: '#8068a8', rose: '#b85f78',
}

function SidebarItem({active, label, count, icon, onClick}: {
    active: boolean; label: string; count?: number; icon: React.ReactNode; onClick: () => void
}) {
    const reduce = useReducedMotion()
    return (
        <button className={`sidebar-item ${active ? 'is-active' : ''}`} onClick={onClick}>
            {active && <motion.span className="sidebar-active" layoutId="sidebar-active" transition={reduce ? {duration: 0} : {type: 'spring', stiffness: 440, damping: 36}}/>}
            <span className="sidebar-item-icon">{icon}</span>
            <span className="sidebar-item-label">{label}</span>
            {typeof count === 'number' && count > 0 && <span className="sidebar-count">{count}</span>}
        </button>
    )
}

function EntityMenu({onEdit, onDelete}: {onEdit: () => void; onDelete: () => void}) {
    return (
        <DropdownMenu.Root>
            <DropdownMenu.Trigger asChild>
                <IconButton className="sidebar-more" label="更多操作" side="right"><MoreHorizontal size={16}/></IconButton>
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
                <DropdownMenu.Content className="menu-content" sideOffset={6} align="start">
                    <DropdownMenu.Item className="menu-item" onSelect={onEdit}><Pencil size={15}/>重命名</DropdownMenu.Item>
                    <DropdownMenu.Separator className="menu-separator"/>
                    <DropdownMenu.Item className="menu-item danger" onSelect={onDelete}><Trash2 size={15}/>删除</DropdownMenu.Item>
                </DropdownMenu.Content>
            </DropdownMenu.Portal>
        </DropdownMenu.Root>
    )
}

export function Sidebar(props: SidebarProps) {
    const {bootstrap, selection} = props
    const dateLabel = new Intl.DateTimeFormat('zh-CN', {month: 'long', day: 'numeric', weekday: 'short'}).format(new Date())
    const counts = bootstrap?.counts
    const listCounts = bootstrap?.listCounts ?? {}
    const tagCounts = bootstrap?.tagCounts ?? {}
    const notebookCounts = bootstrap?.notebookCounts ?? {}
    const noteTagCounts = bootstrap?.noteTagCounts ?? {}
    const noteMode = selection.view === 'notes'
    const selectedNotebook = bootstrap?.notebooks.find((item) => item.id === selection.notebookId)
    const visibleNoteTags = bootstrap?.noteTags.filter((tag) => !selection.notebookId || tag.notebookId === selection.notebookId) ?? []
    return (
        <aside className="sidebar">
            <header className="brand">
                <span className="brand-mark"><Check size={18} strokeWidth={2.6}/></span>
                <span><strong>TODO</strong><small>{dateLabel}</small></span>
            </header>

            <nav className="smart-nav" aria-label="智能视图">
                <SidebarItem active={selection.view === 'overview'} label="概览" icon={<BarChart3 size={18}/>} onClick={() => props.onSelect('overview')}/>
                <SidebarItem active={selection.view === 'notes'} label="笔记" count={counts?.notes} icon={<NotebookPen size={18}/>} onClick={() => props.onSelect('notes')}/>
                <SidebarItem active={selection.view === 'inbox'} label="收集箱" count={counts?.inbox} icon={<Inbox size={18}/>} onClick={() => props.onSelect('inbox')}/>
                <SidebarItem active={selection.view === 'today'} label="今天" count={counts?.today} icon={<CalendarDays size={18}/>} onClick={() => props.onSelect('today')}/>
                <SidebarItem active={selection.view === 'upcoming'} label="即将到期" count={counts?.upcoming} icon={<CalendarClock size={18}/>} onClick={() => props.onSelect('upcoming')}/>
                <SidebarItem active={selection.view === 'all'} label="全部任务" count={counts?.all} icon={<Layers3 size={18}/>} onClick={() => props.onSelect('all')}/>
                <SidebarItem active={selection.view === 'completed'} label="已完成" count={counts?.completed} icon={<CheckCircle2 size={18}/>} onClick={() => props.onSelect('completed')}/>
            </nav>

            {noteMode ? <>
                <div className="sidebar-section notebooks-section">
                    <div className="sidebar-section-header"><span>笔记本</span><IconButton label="新建笔记本" side="right" onClick={props.onAddNotebook}><Plus size={16}/></IconButton></div>
                    <div className="entity-list">
                        {bootstrap?.notebooks.map((notebook) => (
                            <div className={`entity-row ${selection.notebookId === notebook.id ? 'is-active' : ''}`} key={notebook.id}>
                                <button className="entity-main" onClick={() => props.onSelectNotebook(selection.notebookId === notebook.id ? '' : notebook.id)}>
                                    {notebook.kind === 'shared' ? <UsersRound size={16}/> : <BookOpen size={16}/>}<span className="entity-name">{notebook.name}</span>
                                    {(notebookCounts[notebook.id] ?? 0) > 0 && <span className="sidebar-count entity-count">{notebookCounts[notebook.id]}</span>}
                                </button>
                                {!notebook.default && notebook.role === 'owner' && <EntityMenu onEdit={() => props.onEditNotebook(notebook)} onDelete={() => props.onDeleteNotebook(notebook)}/>} 
                            </div>
                        ))}
                    </div>
                </div>
                <div className="sidebar-section tags-section note-tags-section">
                    <div className="sidebar-section-header"><span>笔记标签</span><IconButton label="新建笔记标签" side="right" disabled={selectedNotebook?.role === 'viewer'} onClick={props.onAddNoteTag}><Plus size={16}/></IconButton></div>
                    <div className="entity-list">
                        {visibleNoteTags.length === 0 && <p className="sidebar-empty">还没有笔记标签</p>}
                        {visibleNoteTags.map((tag) => (
                            <div className={`entity-row ${selection.noteTagId === tag.id ? 'is-active' : ''}`} key={tag.id}>
                                <button className="entity-main" onClick={() => props.onSelectNoteTag(selection.noteTagId === tag.id ? '' : tag.id)}>
                                    <TagIcon size={15} style={{color: tagColors[tag.color] ?? tagColors.coral}}/><span className="entity-name">{tag.name}</span>
                                    {(noteTagCounts[tag.id] ?? 0) > 0 && <span className="sidebar-count entity-count">{noteTagCounts[tag.id]}</span>}
                                </button>
                                {bootstrap?.notebooks.find((item) => item.id === tag.notebookId)?.role !== 'viewer' && <EntityMenu onEdit={() => props.onEditNoteTag(tag)} onDelete={() => props.onDeleteNoteTag(tag)}/>} 
                            </div>
                        ))}
                    </div>
                </div>
            </> : <>
            <div className="sidebar-section">
                <div className="sidebar-section-header"><span>清单</span><IconButton label="新建清单" side="right" onClick={props.onAddList}><Plus size={16}/></IconButton></div>
                <div className="entity-list">
                    {bootstrap?.lists.map((list) => (
                        <div className={`entity-row ${selection.view === 'list' && selection.listId === list.id ? 'is-active' : ''}`} key={list.id}>
                            <button className="entity-main" onClick={() => props.onSelect('list', list.id)}><ListTodo size={16}/><span className="entity-name">{list.name}</span>{(listCounts[list.id] ?? 0) > 0 && <span className="sidebar-count entity-count">{listCounts[list.id]}</span>}</button>
                            <EntityMenu onEdit={() => props.onEditList(list)} onDelete={() => props.onDeleteList(list)}/>
                        </div>
                    ))}
                </div>
            </div>

            <div className="sidebar-section tags-section">
                <div className="sidebar-section-header"><span>标签</span><IconButton label="新建标签" side="right" onClick={props.onAddTag}><Plus size={16}/></IconButton></div>
                <div className="entity-list">
                    {bootstrap?.tags.length === 0 && <p className="sidebar-empty">还没有标签</p>}
                    {bootstrap?.tags.map((tag) => (
                        <div className={`entity-row ${selection.tagId === tag.id ? 'is-active' : ''}`} key={tag.id}>
                            <button className="entity-main" onClick={() => props.onSelectTag(selection.tagId === tag.id ? '' : tag.id)}>
                                <TagIcon size={15} style={{color: tagColors[tag.color] ?? tagColors.coral}}/><span className="entity-name">{tag.name}</span>{(tagCounts[tag.id] ?? 0) > 0 && <span className="sidebar-count entity-count">{tagCounts[tag.id]}</span>}
                            </button>
                            <EntityMenu onEdit={() => props.onEditTag(tag)} onDelete={() => props.onDeleteTag(tag)}/>
                        </div>
                    ))}
                </div>
            </div>
            </>}

            <div className="sidebar-footer">
				<button className="settings-button" onClick={props.onOpenCompact}><PictureInPicture2 size={17}/><span>悬浮小窗</span></button>
                <button className="settings-button" onClick={props.onOpenNotifications}><Bell size={17}/><span>通知</span>{(bootstrap?.unreadCount ?? 0) > 0 && <span className="sidebar-count footer-count">{bootstrap?.unreadCount}</span>}</button>
                <button className="settings-button" onClick={props.onOpenSettings}><Settings size={17}/><span>设置与备份</span></button>
            </div>
        </aside>
    )
}
