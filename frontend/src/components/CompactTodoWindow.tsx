import * as ContextMenu from '@radix-ui/react-context-menu'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import {AnimatePresence, motion, useReducedMotion} from 'motion/react'
import {
    CalendarClock,
    CalendarDays,
    Check,
    CheckCircle2,
    ChevronDown,
    Circle,
    Expand,
    Flag,
    Inbox,
    Layers3,
    ListTodo,
    Pin,
    Plus,
    RotateCcw,
    Tag as TagIcon,
    Trash2,
} from 'lucide-react'
import {type ReactNode, type RefObject, useMemo, useState} from 'react'
import type {Bootstrap, Task, TodoList, ViewKey} from '../types'
import {IconButton} from './IconButton'

export type CompactViewKey = Exclude<ViewKey, 'overview' | 'notes'>

export interface CompactSelection {
    view: CompactViewKey
    listId: string
    tagId: string
}

interface CompactTodoWindowProps {
    tasks: Task[]
    bootstrap?: Bootstrap
    selection: CompactSelection
    isLoading: boolean
    error: Error | null
    quickInputRef: RefObject<HTMLInputElement | null>
    onSelectionChange: (selection: CompactSelection) => void
    onCreate: (title: string) => Promise<void>
    onToggle: (task: Task) => void
    onDelete: (task: Task) => void
    onOpenTask: (task: Task) => void
    onExit: () => void
}

const priorityLabel = {none: '', low: '低', medium: '中', high: '高'} as const

const tagColors: Record<string, string> = {
    coral: '#e45d48', amber: '#c88b2a', mint: '#3f987d', blue: '#477eae', violet: '#8068a8', rose: '#b85f78',
}

function relativeDueDate(value: string) {
    const today = new Date().toLocaleDateString('sv-SE')
    const diff = Math.round((new Date(`${value}T12:00:00`).getTime() - new Date(`${today}T12:00:00`).getTime()) / 86_400_000)
    if (diff < 0) return `逾期 ${Math.abs(diff)} 天`
    if (diff === 0) return '今天'
    if (diff === 1) return '明天'
    return new Intl.DateTimeFormat('zh-CN', {month: 'numeric', day: 'numeric'}).format(new Date(`${value}T12:00:00`))
}

function CompactQuickAdd({inputRef, label, onCreate}: {
    inputRef: RefObject<HTMLInputElement | null>
    label: string
    onCreate: (title: string) => Promise<void>
}) {
    const [title, setTitle] = useState('')
    const [busy, setBusy] = useState(false)
    const submit = async () => {
        if (!title.trim() || busy) return
        setBusy(true)
        try {
            await onCreate(title.trim())
            setTitle('')
        } finally {
            setBusy(false)
        }
    }
    return (
        <div className="compact-quick-add">
            <Plus size={18}/>
            <label className="sr-only" htmlFor="compact-quick-task">{label}</label>
            <input
                id="compact-quick-task"
                ref={inputRef}
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                onKeyDown={(event) => {if (event.key === 'Enter') void submit()}}
                placeholder={label}
                maxLength={240}
                disabled={busy}
            />
            <button type="button" disabled={!title.trim() || busy} onClick={() => void submit()}>{busy ? '添加中' : '添加'}</button>
        </div>
    )
}

function CompactTaskRow({task, list, onToggle, onDelete, onOpen}: {
    task: Task
    list?: TodoList
    onToggle: () => void
    onDelete: () => void
    onOpen: () => void
}) {
    const reduce = useReducedMotion()
    const completed = Boolean(task.completedAt)
    const hasMeta = Boolean(task.dueDate || list || task.tags.length || task.priority !== 'none')
    return (
        <ContextMenu.Root>
            <ContextMenu.Trigger asChild>
                <motion.div
                    className={`compact-task-row ${completed ? 'is-completed' : ''} ${hasMeta ? 'has-meta' : ''}`}
                    role="listitem"
                    tabIndex={0}
                    layout={!reduce}
                    initial={reduce ? false : {opacity: 0, y: 8, scale: 0.985}}
                    animate={{opacity: 1, y: 0, scale: 1}}
                    exit={reduce ? {opacity: 0} : {opacity: 0, x: 18, scale: 0.98}}
                    transition={reduce ? {duration: 0} : {type: 'spring', stiffness: 430, damping: 36}}
                    onDoubleClick={onOpen}
                    onKeyDown={(event) => {if (event.key === 'Enter') onOpen()}}
                >
                    <button className={`compact-complete ${completed ? 'is-complete' : ''}`} type="button" aria-label={completed ? '恢复为未完成' : '标记为已完成'} onClick={onToggle}>
                        <motion.span whileTap={reduce ? undefined : {scale: 0.82}}>{completed ? <Check size={14} strokeWidth={3}/> : <Circle size={19}/>}</motion.span>
                    </button>
                    <div className="compact-task-copy">
                        <strong>{task.title}</strong>
                        {hasMeta && <div className="compact-task-meta">
                            {task.dueDate && <span className={task.dueDate < new Date().toLocaleDateString('sv-SE') && !completed ? 'is-overdue' : ''}><CalendarDays size={12}/>{relativeDueDate(task.dueDate)}</span>}
                            {list && <span><Inbox size={12}/>{list.name}</span>}
                            {task.tags.slice(0, 1).map((tag) => <span className="compact-task-tag" style={{color: tagColors[tag.color] ?? tagColors.coral}} key={tag.id}><TagIcon size={11}/>{tag.name}</span>)}
                            {task.priority !== 'none' && <span className={`priority-${task.priority}`}><Flag size={11}/>{priorityLabel[task.priority]}</span>}
                        </div>}
                    </div>
                </motion.div>
            </ContextMenu.Trigger>
            <ContextMenu.Portal>
                <ContextMenu.Content className="menu-content task-context-menu">
                    <ContextMenu.Item className="menu-item" onSelect={onOpen}><Expand size={15}/>在主窗口打开</ContextMenu.Item>
                    <ContextMenu.Item className="menu-item" onSelect={onToggle}>
                        {completed ? <RotateCcw size={15}/> : <CheckCircle2 size={15}/>}
                        {completed ? '恢复为未完成' : '标记为已完成'}
                    </ContextMenu.Item>
                    <ContextMenu.Separator className="menu-separator"/>
                    <ContextMenu.Item className="menu-item danger" onSelect={onDelete}><Trash2 size={15}/>删除任务</ContextMenu.Item>
                </ContextMenu.Content>
            </ContextMenu.Portal>
        </ContextMenu.Root>
    )
}

function ViewMenuItem({active, icon, label, count, onSelect}: {
    active: boolean
    icon: ReactNode
    label: string
    count?: number
    onSelect: () => void
}) {
    return (
        <DropdownMenu.Item className={`menu-item compact-view-option ${active ? 'is-active' : ''}`} aria-label={label} onSelect={onSelect}>
            <span className="compact-view-icon">{icon}</span>
            <span>{label}</span>
            {typeof count === 'number' && count > 0 && <span className="compact-menu-count">{count}</span>}
            <span className="compact-view-check">{active && <Check size={14}/>}</span>
        </DropdownMenu.Item>
    )
}

function selectionCopy(selection: CompactSelection, bootstrap?: Bootstrap) {
    const list = bootstrap?.lists.find((item) => item.id === selection.listId)
    const tag = bootstrap?.tags.find((item) => item.id === selection.tagId)
    if (tag) return {title: tag.name, subtitle: '标签任务', quickLabel: `添加到${tag.name}`, emptyTitle: '这个标签下还没有任务', emptyBody: '新增任务会自动关联当前标签。'}
    if (selection.view === 'list') return {title: list?.name ?? '清单', subtitle: '自定义清单', quickLabel: `添加到${list?.name ?? '清单'}`, emptyTitle: '这个清单还是空的', emptyBody: '新增任务会直接放入当前清单。'}
    if (selection.view === 'inbox') return {title: '收集箱', subtitle: '未归类任务', quickLabel: '添加到收集箱', emptyTitle: '收集箱是空的', emptyBody: '暂未归类的任务会显示在这里。'}
    if (selection.view === 'upcoming') return {title: '即将到期', subtitle: '接下来的任务', quickLabel: '添加明日任务', emptyTitle: '还没有即将到期的任务', emptyBody: '在这里新增会默认安排到明天。'}
    if (selection.view === 'all') return {title: '全部任务', subtitle: '所有未完成任务', quickLabel: '添加任务', emptyTitle: '没有未完成任务', emptyBody: '新增任务会显示在这里。'}
    if (selection.view === 'completed') return {title: '已完成', subtitle: '最近完成的任务', quickLabel: '', emptyTitle: '还没有已完成的任务', emptyBody: '完成任务后可以在这里查看和恢复。'}
    return {
        title: '今天',
        subtitle: new Intl.DateTimeFormat('zh-CN', {month: 'long', day: 'numeric', weekday: 'long'}).format(new Date()),
        quickLabel: '添加今日任务',
        emptyTitle: '今天已经清空了',
        emptyBody: '新增任务会直接安排到今天。',
    }
}

export function CompactTodoWindow(props: CompactTodoWindowProps) {
    const reduce = useReducedMotion()
    const lists = props.bootstrap?.lists ?? []
    const tags = props.bootstrap?.tags ?? []
    const listById = useMemo(() => new Map(lists.map((list) => [list.id, list])), [lists])
    const copy = selectionCopy(props.selection, props.bootstrap)
    const counts = props.bootstrap?.counts
    const selectSmartView = (view: Exclude<CompactViewKey, 'list'>) => props.onSelectionChange({view, listId: '', tagId: ''})
    return (
        <motion.main
            className="compact-shell"
            aria-label="悬浮任务小窗"
            initial={reduce ? false : {opacity: 0, scale: 0.985}}
            animate={{opacity: 1, scale: 1}}
            transition={reduce ? {duration: 0} : {duration: 0.2, ease: [0.16, 1, 0.3, 1]}}
        >
            <header className="compact-header">
                <div className="compact-heading">
                    <span className="compact-brand"><Check size={15} strokeWidth={2.8}/></span>
                    <DropdownMenu.Root>
                        <DropdownMenu.Trigger asChild>
                            <button className="compact-view-trigger" type="button" aria-label={`切换小窗视图，当前${copy.title}`}>
                                <span><strong role="heading" aria-level={1}>{copy.title}</strong><small>{copy.subtitle}</small></span>
                                <ChevronDown size={15}/>
                            </button>
                        </DropdownMenu.Trigger>
                        <DropdownMenu.Portal>
                            <DropdownMenu.Content className="menu-content compact-view-menu" align="start" sideOffset={7}>
                                <ViewMenuItem active={props.selection.view === 'inbox' && !props.selection.tagId} icon={<Inbox size={15}/>} label="收集箱" count={counts?.inbox} onSelect={() => selectSmartView('inbox')}/>
                                <ViewMenuItem active={props.selection.view === 'today' && !props.selection.tagId} icon={<CalendarDays size={15}/>} label="今天" count={counts?.today} onSelect={() => selectSmartView('today')}/>
                                <ViewMenuItem active={props.selection.view === 'upcoming' && !props.selection.tagId} icon={<CalendarClock size={15}/>} label="即将到期" count={counts?.upcoming} onSelect={() => selectSmartView('upcoming')}/>
                                <ViewMenuItem active={props.selection.view === 'all' && !props.selection.tagId} icon={<Layers3 size={15}/>} label="全部任务" count={counts?.all} onSelect={() => selectSmartView('all')}/>
                                <ViewMenuItem active={props.selection.view === 'completed' && !props.selection.tagId} icon={<CheckCircle2 size={15}/>} label="已完成" count={counts?.completed} onSelect={() => selectSmartView('completed')}/>
                                {lists.length > 0 && <>
                                    <DropdownMenu.Separator className="menu-separator"/>
                                    <DropdownMenu.Label className="compact-menu-label">清单</DropdownMenu.Label>
                                    {lists.map((list) => <ViewMenuItem active={props.selection.view === 'list' && props.selection.listId === list.id} icon={<ListTodo size={15}/>} label={list.name} count={props.bootstrap?.listCounts[list.id]} onSelect={() => props.onSelectionChange({view: 'list', listId: list.id, tagId: ''})} key={list.id}/>)}
                                </>}
                                {tags.length > 0 && <>
                                    <DropdownMenu.Separator className="menu-separator"/>
                                    <DropdownMenu.Label className="compact-menu-label">标签</DropdownMenu.Label>
                                    {tags.map((tag) => <ViewMenuItem active={props.selection.tagId === tag.id} icon={<TagIcon size={15} style={{color: tagColors[tag.color] ?? tagColors.coral}}/>} label={tag.name} count={props.bootstrap?.tagCounts[tag.id]} onSelect={() => props.onSelectionChange({view: 'all', listId: '', tagId: tag.id})} key={tag.id}/>)}
                                </>}
                            </DropdownMenu.Content>
                        </DropdownMenu.Portal>
                    </DropdownMenu.Root>
                </div>
                <div className="compact-header-actions">
                    <span className="compact-pinned" aria-label="窗口已置顶"><Pin size={13}/></span>
                    {props.tasks.length > 0 && <span className="compact-count">{props.tasks.length}</span>}
                    <IconButton label="返回主窗口" side="bottom" onClick={props.onExit}><Expand size={17}/></IconButton>
                </div>
            </header>

            {props.selection.view !== 'completed' && <CompactQuickAdd inputRef={props.quickInputRef} label={copy.quickLabel} onCreate={props.onCreate}/>} 

            <section className="compact-task-list" aria-label={`${copy.title}任务列表`} role="list">
                {props.isLoading && <div className="compact-skeletons">{[0, 1, 2, 3].map((item) => <span key={item}/>)}</div>}
                {props.error && <div className="compact-state is-error"><strong>无法读取任务</strong><p>{props.error.message}</p></div>}
                {!props.isLoading && !props.error && props.tasks.length === 0 && (
                    <div className="compact-state"><span><CheckCircle2 size={25}/></span><strong>{copy.emptyTitle}</strong><p>{copy.emptyBody}</p></div>
                )}
                <AnimatePresence initial={false} mode="popLayout">
                    {props.tasks.map((task) => (
                        <CompactTaskRow
                            key={task.id}
                            task={task}
                            list={listById.get(task.listId)}
                            onToggle={() => props.onToggle(task)}
                            onDelete={() => props.onDelete(task)}
                            onOpen={() => props.onOpenTask(task)}
                        />
                    ))}
                </AnimatePresence>
            </section>
        </motion.main>
    )
}
