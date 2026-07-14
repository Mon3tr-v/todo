import * as ContextMenu from '@radix-ui/react-context-menu'
import {AnimatePresence, motion, useReducedMotion} from 'motion/react'
import {
    CalendarClock,
    CalendarDays,
    CalendarPlus,
    Check,
    CheckCircle2,
    ChevronRight,
    Circle,
    Flag,
    Inbox,
    ListFilter,
    ListTodo,
    PanelRightOpen,
    NotebookPen,
    Plus,
    RotateCcw,
    Search,
    Tag as TagIcon,
    Trash2,
} from 'lucide-react'
import {type RefObject, useMemo, useState} from 'react'
import type {Priority, SortKey, Tag, Task, TodoList, UpdateTaskInput, ViewKey} from '../types'
import {CustomSelect} from './CustomSelect'

interface TaskListProps {
    title: string
    subtitle: string
    view: ViewKey
    tasks: Task[]
    lists: TodoList[]
    tags: Tag[]
    selectedId: string
    isLoading: boolean
    error: Error | null
    search: string
    priority: Priority | 'all'
    sort: SortKey
    quickInputRef: RefObject<HTMLInputElement | null>
    searchInputRef: RefObject<HTMLInputElement | null>
    onSearch: (value: string) => void
    onPriority: (value: Priority | 'all') => void
    onSort: (value: SortKey) => void
    onSelect: (task: Task) => void
    onToggle: (task: Task) => void
    onUpdate: (input: UpdateTaskInput) => void
    onOpenDatePicker: (task: Task) => void
    onDelete: (task: Task) => void
    onCreateLinkedNote: (task: Task) => void
    onCreate: (title: string) => Promise<void>
}

const priorityLabel: Record<Priority, string> = {none: '', low: '低', medium: '中', high: '高'}
const priorityOptions: Array<{value: Priority | 'all'; label: string}> = [
    {value: 'all', label: '全部优先级'},
    {value: 'high', label: '高优先级'},
    {value: 'medium', label: '中优先级'},
    {value: 'low', label: '低优先级'},
    {value: 'none', label: '无优先级'},
]
const sortOptions: Array<{value: SortKey; label: string}> = [
    {value: 'due', label: '按日期'},
    {value: 'priority', label: '按优先级'},
    {value: 'created', label: '按创建时间'},
]
const inboxValue = '__todo_inbox__'
const noDueDateValue = '__todo_no_due_date__'

function localDate(value: string) { return new Date(`${value}T00:00:00`) }

function dueLabel(value: string) {
    if (!value) return ''
    const due = localDate(value)
    const now = new Date()
    const current = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const diff = Math.round((due.getTime() - current.getTime()) / 86_400_000)
    if (diff < 0) return `逾期 ${Math.abs(diff)} 天`
    if (diff === 0) return '今天'
    if (diff === 1) return '明天'
    return new Intl.DateTimeFormat('zh-CN', {month: 'numeric', day: 'numeric'}).format(due)
}

function dateOffset(days: number) {
    const date = new Date()
    date.setHours(12, 0, 0, 0)
    date.setDate(date.getDate() + days)
    return date.toLocaleDateString('sv-SE')
}

function taskUpdateInput(task: Task, changes: Partial<Pick<UpdateTaskInput, 'listId' | 'priority' | 'dueDate' | 'tagIds'>>): UpdateTaskInput {
    return {
        id: task.id,
        title: task.title,
        notes: task.notes,
        listId: task.listId,
        priority: task.priority,
        dueDate: task.dueDate,
        tagIds: task.tags.map((tag) => tag.id),
        ...changes,
    }
}

function MenuRadioItem({value, onSelect, children}: {value: string; onSelect: () => void; children: React.ReactNode}) {
    return (
        <ContextMenu.RadioItem className="menu-item menu-choice" value={value} onSelect={onSelect}>
            <span className="menu-indicator"><ContextMenu.ItemIndicator><Check size={14}/></ContextMenu.ItemIndicator></span>
            <span>{children}</span>
        </ContextMenu.RadioItem>
    )
}

function TaskPropertyMenus({task, lists, tags, onUpdate, onOpenDatePicker}: {
    task: Task
    lists: TodoList[]
    tags: Tag[]
    onUpdate: (input: UpdateTaskInput) => void
    onOpenDatePicker: () => void
}) {
    const update = (changes: Partial<Pick<UpdateTaskInput, 'listId' | 'priority' | 'dueDate' | 'tagIds'>>) => onUpdate(taskUpdateInput(task, changes))
    const today = dateOffset(0)
    const tomorrow = dateOffset(1)
    const nextWeek = dateOffset(7)
    const selectedTagIds = task.tags.map((tag) => tag.id)
    return (
        <>
            <ContextMenu.Sub>
                <ContextMenu.SubTrigger className="menu-item menu-sub-trigger"><ListTodo size={15}/><span>清单</span><ChevronRight className="menu-chevron" size={14}/></ContextMenu.SubTrigger>
                <ContextMenu.Portal>
                    <ContextMenu.SubContent className="menu-content menu-sub-content" sideOffset={4} alignOffset={-5}>
                        <ContextMenu.RadioGroup value={task.listId || inboxValue}>
                            <MenuRadioItem value={inboxValue} onSelect={() => update({listId: ''})}>收集箱</MenuRadioItem>
                            {lists.map((list) => <MenuRadioItem value={list.id} onSelect={() => update({listId: list.id})} key={list.id}>{list.name}</MenuRadioItem>)}
                        </ContextMenu.RadioGroup>
                    </ContextMenu.SubContent>
                </ContextMenu.Portal>
            </ContextMenu.Sub>

            <ContextMenu.Sub>
                <ContextMenu.SubTrigger className="menu-item menu-sub-trigger"><Flag size={15}/><span>优先级</span><ChevronRight className="menu-chevron" size={14}/></ContextMenu.SubTrigger>
                <ContextMenu.Portal>
                    <ContextMenu.SubContent className="menu-content menu-sub-content" sideOffset={4} alignOffset={-5}>
                        <ContextMenu.RadioGroup value={task.priority}>
                            <MenuRadioItem value="none" onSelect={() => update({priority: 'none'})}>无优先级</MenuRadioItem>
                            <MenuRadioItem value="low" onSelect={() => update({priority: 'low'})}>低优先级</MenuRadioItem>
                            <MenuRadioItem value="medium" onSelect={() => update({priority: 'medium'})}>中优先级</MenuRadioItem>
                            <MenuRadioItem value="high" onSelect={() => update({priority: 'high'})}>高优先级</MenuRadioItem>
                        </ContextMenu.RadioGroup>
                    </ContextMenu.SubContent>
                </ContextMenu.Portal>
            </ContextMenu.Sub>

            <ContextMenu.Sub>
                <ContextMenu.SubTrigger className="menu-item menu-sub-trigger"><CalendarClock size={15}/><span>截止日期</span><ChevronRight className="menu-chevron" size={14}/></ContextMenu.SubTrigger>
                <ContextMenu.Portal>
                    <ContextMenu.SubContent className="menu-content menu-sub-content" sideOffset={4} alignOffset={-5}>
                        <ContextMenu.RadioGroup value={task.dueDate || noDueDateValue}>
                            <MenuRadioItem value={today} onSelect={() => update({dueDate: today})}>今天</MenuRadioItem>
                            <MenuRadioItem value={tomorrow} onSelect={() => update({dueDate: tomorrow})}>明天</MenuRadioItem>
                            <MenuRadioItem value={nextWeek} onSelect={() => update({dueDate: nextWeek})}>一周后</MenuRadioItem>
                            <MenuRadioItem value={noDueDateValue} onSelect={() => update({dueDate: ''})}>无截止日期</MenuRadioItem>
                        </ContextMenu.RadioGroup>
                        <ContextMenu.Separator className="menu-separator"/>
                        <ContextMenu.Item className="menu-item" onSelect={onOpenDatePicker}><CalendarPlus size={15}/>选择其他日期</ContextMenu.Item>
                    </ContextMenu.SubContent>
                </ContextMenu.Portal>
            </ContextMenu.Sub>

            <ContextMenu.Sub>
                <ContextMenu.SubTrigger className="menu-item menu-sub-trigger"><TagIcon size={15}/><span>标签</span><ChevronRight className="menu-chevron" size={14}/></ContextMenu.SubTrigger>
                <ContextMenu.Portal>
                    <ContextMenu.SubContent className="menu-content menu-sub-content" sideOffset={4} alignOffset={-5}>
                        {tags.length === 0 ? <ContextMenu.Item className="menu-item" disabled>暂无标签</ContextMenu.Item> : tags.map((tag) => {
                            const checked = selectedTagIds.includes(tag.id)
                            return (
                                <ContextMenu.CheckboxItem
                                    className="menu-item menu-choice"
                                    checked={checked}
                                    onSelect={() => update({tagIds: checked ? selectedTagIds.filter((id) => id !== tag.id) : [...selectedTagIds, tag.id]})}
                                    key={tag.id}
                                >
                                    <span className="menu-indicator"><ContextMenu.ItemIndicator><Check size={14}/></ContextMenu.ItemIndicator></span>
                                    <span>{tag.name}</span>
                                </ContextMenu.CheckboxItem>
                            )
                        })}
                    </ContextMenu.SubContent>
                </ContextMenu.Portal>
            </ContextMenu.Sub>
        </>
    )
}

function QuickAdd({inputRef, onCreate}: {inputRef: RefObject<HTMLInputElement | null>; onCreate: (title: string) => Promise<void>}) {
    const [title, setTitle] = useState('')
    const [busy, setBusy] = useState(false)
    const submit = async () => {
        if (!title.trim() || busy) return
        setBusy(true)
        try { await onCreate(title.trim()); setTitle('') } finally { setBusy(false) }
    }
    return (
        <div className={`quick-add ${title ? 'has-value' : ''}`}>
            <span className="quick-add-icon"><Plus size={19}/></span>
            <label className="sr-only" htmlFor="quick-task">添加任务</label>
            <input
                id="quick-task"
                ref={inputRef}
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                onKeyDown={(event) => { if (event.key === 'Enter') void submit() }}
                placeholder="写下需要完成的事"
                maxLength={240}
                disabled={busy}
            />
            <button className="quick-add-submit" type="button" disabled={!title.trim() || busy} onClick={() => void submit()}>
                {busy ? '添加中' : '添加'}
            </button>
        </div>
    )
}

function TaskRow({task, list, lists, tags, active, onSelect, onToggle, onUpdate, onOpenDatePicker, onDelete, onCreateLinkedNote}: {
    task: Task
    list?: TodoList
    lists: TodoList[]
    tags: Tag[]
    active: boolean
    onSelect: () => void
    onToggle: () => void
    onUpdate: (input: UpdateTaskInput) => void
    onOpenDatePicker: () => void
    onDelete: () => void
    onCreateLinkedNote: () => void
}) {
    const reduce = useReducedMotion()
    const completed = Boolean(task.completedAt)
    const hasMeta = Boolean(task.dueDate || list || task.tags.length > 0)
    return (
        <ContextMenu.Root onOpenChange={(open) => {if (open) onSelect()}}>
            <ContextMenu.Trigger asChild>
                <motion.div
                    layout={!reduce}
                    initial={reduce ? false : {opacity: 0, y: 10, scale: 0.985}}
                    animate={{opacity: 1, y: 0, scale: 1}}
                    exit={reduce ? {opacity: 0} : {opacity: 0, x: 20, scale: 0.98}}
                    transition={reduce ? {duration: 0} : {type: 'spring', stiffness: 430, damping: 36}}
                    className={`task-row ${active ? 'is-selected' : ''} ${completed ? 'is-completed' : ''} ${hasMeta ? 'has-meta' : ''}`}
                    role="listitem"
                    tabIndex={0}
                    onClick={onSelect}
                    onKeyDown={(event) => { if (event.key === 'Enter') onSelect() }}
                    onContextMenu={onSelect}
                    data-task-id={task.id}
                >
                    <button
                        className={`complete-button ${completed ? 'is-complete' : ''}`}
                        aria-label={completed ? '恢复为未完成' : '标记为已完成'}
                        onClick={(event) => {event.stopPropagation(); onToggle()}}
                    >
                        <motion.span animate={completed ? {scale: [0.75, 1.12, 1]} : {scale: 1}} transition={reduce ? {duration: 0} : {duration: 0.3}}>
                            {completed ? <Check size={15} strokeWidth={3}/> : <Circle size={18}/>} 
                        </motion.span>
                    </button>
                    <div className="task-copy">
                        <div className="task-title-line">
                            <span className="task-title">{task.title}</span>
                            {task.priority !== 'none' && <span className={`priority priority-${task.priority}`}><Flag size={12}/>{priorityLabel[task.priority]}</span>}
                        </div>
                        {hasMeta && (
                            <div className="task-meta">
                                {task.dueDate && <span className={task.dueDate < new Date().toLocaleDateString('sv-SE') && !completed ? 'overdue' : ''}><CalendarDays size={13}/>{dueLabel(task.dueDate)}</span>}
                                {list && <span><Inbox size={13}/>{list.name}</span>}
                                {task.tags.slice(0, 2).map((tag) => <span className={`task-tag tag-${tag.color}`} key={tag.id}>{tag.name}</span>)}
                                {task.tags.length > 2 && <span className="more-tags">+{task.tags.length - 2}</span>}
                            </div>
                        )}
                    </div>
                </motion.div>
            </ContextMenu.Trigger>
            <ContextMenu.Portal>
                <ContextMenu.Content className="menu-content task-context-menu">
                    <ContextMenu.Item className="menu-item" onSelect={onSelect}><PanelRightOpen size={15}/>打开详情</ContextMenu.Item>
                    <ContextMenu.Item className="menu-item" onSelect={onCreateLinkedNote}><NotebookPen size={15}/>新建关联笔记</ContextMenu.Item>
                    <ContextMenu.Item className="menu-item" onSelect={onToggle}>
                        {completed ? <RotateCcw size={15}/> : <CheckCircle2 size={15}/>}
                        {completed ? '恢复为未完成' : '标记为已完成'}
                    </ContextMenu.Item>
                    <ContextMenu.Separator className="menu-separator"/>
                    <TaskPropertyMenus task={task} lists={lists} tags={tags} onUpdate={onUpdate} onOpenDatePicker={onOpenDatePicker}/>
                    <ContextMenu.Separator className="menu-separator"/>
                    <ContextMenu.Item className="menu-item danger" onSelect={onDelete}><Trash2 size={15}/>删除任务</ContextMenu.Item>
                </ContextMenu.Content>
            </ContextMenu.Portal>
        </ContextMenu.Root>
    )
}

export function TaskList(props: TaskListProps) {
    const listById = useMemo(() => new Map(props.lists.map((list) => [list.id, list])), [props.lists])
    const navigate = (direction: number) => {
        if (!props.tasks.length) return
        const current = props.tasks.findIndex((task) => task.id === props.selectedId)
        const next = current < 0 ? 0 : Math.min(props.tasks.length - 1, Math.max(0, current + direction))
        props.onSelect(props.tasks[next])
        document.querySelector<HTMLElement>(`[data-task-id="${props.tasks[next].id}"]`)?.focus()
    }
    return (
        <main className="task-panel" onKeyDown={(event) => {
            if (event.target instanceof HTMLInputElement || (event.target instanceof HTMLElement && event.target.closest('.select-trigger'))) return
            if (event.key === 'ArrowDown') {event.preventDefault(); navigate(1)}
            if (event.key === 'ArrowUp') {event.preventDefault(); navigate(-1)}
        }}>
            <header className="task-panel-header">
                <div><h1>{props.title}</h1><p>{props.subtitle}</p></div>
                <span className="task-total">{props.tasks.length}</span>
            </header>

            <div className="task-toolbar">
                <div className="search-field">
                    <Search size={16}/>
                    <label className="sr-only" htmlFor="task-search">搜索任务</label>
                    <input id="task-search" ref={props.searchInputRef} value={props.search} onChange={(event) => props.onSearch(event.target.value)} placeholder="搜索"/>
                </div>
                <div className="select-field"><ListFilter size={15}/><CustomSelect ariaLabel="优先级" value={props.priority} options={priorityOptions} onValueChange={props.onPriority} className="toolbar-select priority-filter"/></div>
                <div className="select-field compact"><CustomSelect ariaLabel="排序" value={props.sort} options={sortOptions} onValueChange={props.onSort} className="toolbar-select sort-filter"/></div>
            </div>

            {props.view !== 'completed' && <QuickAdd inputRef={props.quickInputRef} onCreate={props.onCreate}/>} 

            <section className="task-list" aria-label="任务列表" role="list">
                {props.isLoading && <div className="task-skeletons">{[0, 1, 2, 3].map((index) => <div className="task-skeleton" key={index}><span/><div><i/><i/></div></div>)}</div>}
                {props.error && <div className="state-view error-state"><strong>无法读取任务</strong><p>{props.error.message}</p></div>}
                {!props.isLoading && !props.error && props.tasks.length === 0 && (
                    <div className="state-view"><span className="state-icon"><CheckCircle2 size={28}/></span><strong>{props.search ? '没有匹配的任务' : '这里已经清空了'}</strong><p>{props.search ? '换个关键词或调整筛选条件。' : '新任务会出现在这里。'}</p></div>
                )}
                <AnimatePresence initial={false} mode="popLayout">
                    {props.tasks.map((task) => (
                        <TaskRow key={task.id} task={task} list={listById.get(task.listId)} lists={props.lists} tags={props.tags} active={props.selectedId === task.id} onSelect={() => props.onSelect(task)} onToggle={() => props.onToggle(task)} onUpdate={props.onUpdate} onOpenDatePicker={() => props.onOpenDatePicker(task)} onDelete={() => props.onDelete(task)} onCreateLinkedNote={() => props.onCreateLinkedNote(task)}/>
                    ))}
                </AnimatePresence>
            </section>
        </main>
    )
}
