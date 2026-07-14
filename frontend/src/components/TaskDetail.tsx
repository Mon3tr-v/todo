import {useEffect, useMemo, useState} from 'react'
import {Check, ChevronRight, CircleCheck, FileText, Flag, NotebookPen, Save, Trash2, X} from 'lucide-react'
import type {NoteSummary, Priority, Tag, Task, TodoList, UpdateTaskInput} from '../types'
import {CustomSelect} from './CustomSelect'
import {DatePickerField} from './DatePickerField'
import {IconButton} from './IconButton'

interface TaskDetailProps {
    task: Task
    lists: TodoList[]
    tags: Tag[]
    saving: boolean
    onSave: (input: UpdateTaskInput) => Promise<void>
    onDelete: (task: Task) => void
    relatedNotes: NoteSummary[]
    onOpenNote: (note: NoteSummary) => void
    onCreateLinkedNote: () => void
    onClose: () => void
}

const priorities: Array<{value: Priority; label: string}> = [
    {value: 'none', label: '无'}, {value: 'low', label: '低'}, {value: 'medium', label: '中'}, {value: 'high', label: '高'},
]

export function TaskDetail({task, lists, tags, saving, onSave, onDelete, relatedNotes, onOpenNote, onCreateLinkedNote, onClose}: TaskDetailProps) {
    const [title, setTitle] = useState(task.title)
    const [notes, setNotes] = useState(task.notes)
    const [listId, setListId] = useState(task.listId)
    const [priority, setPriority] = useState<Priority>(task.priority)
    const [dueDate, setDueDate] = useState(task.dueDate)
    const [tagIds, setTagIds] = useState(task.tags.map((tag) => tag.id))

    useEffect(() => {
        setTitle(task.title); setNotes(task.notes); setListId(task.listId); setPriority(task.priority)
        setDueDate(task.dueDate); setTagIds(task.tags.map((tag) => tag.id))
    }, [task])

    const dirty = useMemo(() => {
        const originalTags = task.tags.map((tag) => tag.id).sort().join(',')
        return title.trim() !== task.title || notes !== task.notes || listId !== task.listId || priority !== task.priority || dueDate !== task.dueDate || [...tagIds].sort().join(',') !== originalTags
    }, [task, title, notes, listId, priority, dueDate, tagIds])

    const submit = async () => {
        if (!title.trim() || saving) return
        await onSave({id: task.id, title: title.trim(), notes, listId, priority, dueDate, tagIds})
    }

    return (
        <aside className="detail-panel" aria-label="任务详情">
            <header className="detail-header">
                <div><span className={`detail-status ${task.completedAt ? 'complete' : ''}`}>{task.completedAt ? <CircleCheck size={16}/> : <Flag size={16}/>} {task.completedAt ? '已完成' : '任务详情'}</span></div>
                <div className="detail-actions">
                    <IconButton label="删除任务" onClick={() => onDelete(task)}><Trash2 size={17}/></IconButton>
                    <IconButton label="关闭详情" onClick={onClose}><X size={18}/></IconButton>
                </div>
            </header>
            <div className="detail-scroll">
                <div className="form-field title-field">
                    <label htmlFor="detail-title">任务标题</label>
                    <textarea id="detail-title" value={title} onChange={(event) => setTitle(event.target.value)} rows={2} maxLength={240}/>
                    {!title.trim() && <small className="field-error">标题不能为空</small>}
                </div>
                <div className="form-field">
                    <label htmlFor="detail-notes">备注</label>
                    <textarea id="detail-notes" className="notes-input" value={notes} onChange={(event) => setNotes(event.target.value)} rows={5} placeholder="补充上下文、链接或想法"/>
                </div>
                <div className="form-field">
                    <label htmlFor="detail-list">清单</label>
                    <CustomSelect id="detail-list" ariaLabel="清单" value={listId} options={[{value: '', label: '收集箱'}, ...lists.map((list) => ({value: list.id, label: list.name}))]} onValueChange={setListId} className="field-select"/>
                </div>
                <fieldset className="form-field">
                    <legend>优先级</legend>
                    <div className="priority-control">
                        {priorities.map((item) => (
                            <button key={item.value} type="button" className={priority === item.value ? `is-active priority-${item.value}` : ''} onClick={() => setPriority(item.value)}>
                                {priority === item.value && <Check size={14}/>}<span>{item.label}</span>
                            </button>
                        ))}
                    </div>
                </fieldset>
                <div className="form-field">
                    <label>截止日期</label>
                    <DatePickerField value={dueDate} onChange={setDueDate}/>
                </div>
                <fieldset className="form-field tag-selector">
                    <legend>标签</legend>
                    {tags.length === 0 ? <p className="field-empty">可在左侧栏创建标签。</p> : (
                        <div className="tag-options">
                            {tags.map((tag) => {
                                const selected = tagIds.includes(tag.id)
                                return <button key={tag.id} type="button" className={`tag-option tag-${tag.color} ${selected ? 'is-selected' : ''}`} aria-pressed={selected} onClick={() => setTagIds(selected ? tagIds.filter((id) => id !== tag.id) : [...tagIds, tag.id])}>{selected && <Check size={13}/>}<span>{tag.name}</span></button>
                            })}
                        </div>
                    )}
                </fieldset>
                <section className="form-field related-notes">
                    <div className="related-notes-heading"><label>关联笔记</label><button type="button" onClick={onCreateLinkedNote}><NotebookPen size={14}/>新建</button></div>
                    {relatedNotes.length === 0 ? <p className="field-empty">还没有关联笔记。</p> : <div>{relatedNotes.map((note) => <button type="button" key={note.id} onClick={() => onOpenNote(note)}><FileText size={15}/><span>{note.title}<small>{note.excerpt || '暂无正文'}</small></span><ChevronRight size={14}/></button>)}</div>}
                </section>
            </div>
            <footer className="detail-footer">
                <span>{dirty ? '有未保存的更改' : '更改已保存'}</span>
                <button className="primary-button" disabled={!dirty || !title.trim() || saving} onClick={() => void submit()}><Save size={16}/>{saving ? '保存中' : '保存更改'}</button>
            </footer>
        </aside>
    )
}
