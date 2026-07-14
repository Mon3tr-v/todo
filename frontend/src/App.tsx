import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query'
import * as Tooltip from '@radix-ui/react-tooltip'
import {AnimatePresence, motion, useReducedMotion} from 'motion/react'
import {PanelRightOpen} from 'lucide-react'
import {lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState} from 'react'
import {api, isDesktop} from './lib/api'
import {EventsOn} from '../wailsjs/runtime/runtime'
import type {AppNotification, AppSettings, LoginInput, Note, NoteSummary, NoteTag, Notebook, Priority, SortKey, Tag, Task, TaskReference, TodoList, UpdateTaskInput, ViewKey} from './types'
import {Sidebar} from './components/Sidebar'
import {TaskList} from './components/TaskList'
import {TaskDetail} from './components/TaskDetail'
import {OverviewPanel} from './components/OverviewPanel'
import {NotificationCenter} from './components/NotificationCenter'
import {AppToast, type ToastState} from './components/AppToast'
import {CompactTodoWindow, type CompactSelection, type CompactViewKey} from './components/CompactTodoWindow'
import {
    ConfirmDialog,
    type ConfirmState,
    EntityDialog,
    type EntityEditor,
    SettingsDialog,
} from './components/Dialogs'
import './App.css'

const NoteWorkspace = lazy(async () => ({default: (await import('./components/NoteWorkspace')).NoteWorkspace}))

const localToday = () => new Date().toLocaleDateString('sv-SE')
const localTomorrow = () => {
    const date = new Date()
    date.setHours(12, 0, 0, 0)
    date.setDate(date.getDate() + 1)
    return date.toLocaleDateString('sv-SE')
}

function errorMessage(error: unknown) {
    if (error instanceof Error) return error.message
    if (typeof error === 'string') return error
    return '操作失败，请稍后重试'
}

const viewCopy: Record<Exclude<ViewKey, 'list' | 'overview' | 'notes'>, {title: string; subtitle: string}> = {
    inbox: {title: '收集箱', subtitle: '还没有归类的想法和任务'},
    today: {title: '今天', subtitle: new Intl.DateTimeFormat('zh-CN', {month: 'long', day: 'numeric', weekday: 'long'}).format(new Date())},
    upcoming: {title: '即将到期', subtitle: '按日期安排接下来的任务'},
    completed: {title: '已完成', subtitle: '回顾已经完成的事情'},
    all: {title: '全部任务', subtitle: '所有尚未完成的任务'},
}

function App() {
    const client = useQueryClient()
    const reduce = useReducedMotion()
    const [view, setView] = useState<ViewKey>('today')
    const [listId, setListId] = useState('')
    const [tagId, setTagId] = useState('')
    const [notebookId, setNotebookId] = useState('')
    const [noteTagId, setNoteTagId] = useState('')
    const [search, setSearch] = useState('')
    const [priority, setPriority] = useState<Priority | 'all'>('all')
    const [sort, setSort] = useState<SortKey>('due')
    const [selectedId, setSelectedId] = useState('')
    const [selectedNoteId, setSelectedNoteId] = useState('')
    const [pendingTaskId, setPendingTaskId] = useState('')
    const [editor, setEditor] = useState<EntityEditor>(null)
    const [confirm, setConfirm] = useState<ConfirmState | null>(null)
    const [settingsOpen, setSettingsOpen] = useState(false)
    const [notificationsOpen, setNotificationsOpen] = useState(false)
    const [dialogBusy, setDialogBusy] = useState(false)
    const [toast, setToast] = useState<ToastState | null>(null)
    const [compactMode, setCompactMode] = useState(false)
    const [compactSelection, setCompactSelection] = useState<CompactSelection>({view: 'today', listId: '', tagId: ''})
    const initialized = useRef(false)
    const toastTimer = useRef<number | undefined>(undefined)
    const quickInputRef = useRef<HTMLInputElement>(null)
    const searchInputRef = useRef<HTMLInputElement>(null)

    const bootstrapQuery = useQuery({queryKey: ['bootstrap'], queryFn: () => api.GetBootstrap()})
    const selection = {view, listId, tagId, notebookId, noteTagId}
    const taskQuery = useQuery({
        queryKey: ['tasks', selection, priority, search, sort],
        queryFn: () => api.ListTasks({view, listId, tagId, priority, search, sort, status: ''}),
        enabled: view !== 'overview' && view !== 'notes',
    })
    const overviewQuery = useQuery({queryKey: ['overview'], queryFn: () => api.GetOverview(), enabled: view === 'overview'})
    const notificationsQuery = useQuery({queryKey: ['notifications'], queryFn: () => api.ListNotifications(), enabled: notificationsOpen})
    const compactTasksQuery = useQuery({
        queryKey: ['tasks', 'compact', compactSelection],
        queryFn: () => api.ListTasks({...compactSelection, priority: 'all', search: '', sort: 'due', status: ''}),
        enabled: compactMode,
    })

	useEffect(() => {
		let active = true
		void api.GetCompactMode().then((enabled) => {if (active) setCompactMode(enabled)}).catch(() => undefined)
		return () => {active = false}
	}, [])

	useEffect(() => {
		document.body.classList.toggle('compact-mode', compactMode)
		return () => document.body.classList.remove('compact-mode')
	}, [compactMode])

    useEffect(() => {
        if (!bootstrapQuery.data || initialized.current) return
        initialized.current = true
        setView(bootstrapQuery.data.settings.startupView)
    }, [bootstrapQuery.data])

    const settings = bootstrapQuery.data?.settings ?? {theme: 'system' as const, startupView: 'today' as const}
    useEffect(() => {
        const media = window.matchMedia('(prefers-color-scheme: dark)')
        const apply = () => {
            const theme = settings.theme === 'system' ? (media.matches ? 'dark' : 'light') : settings.theme
            document.documentElement.dataset.theme = theme
            document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#191b1f' : '#f4f5f7')
        }
        apply()
        media.addEventListener('change', apply)
        return () => media.removeEventListener('change', apply)
    }, [settings.theme])

    const invalidate = useCallback(async () => {
        await Promise.all([
            client.invalidateQueries({queryKey: ['bootstrap']}),
            client.invalidateQueries({queryKey: ['tasks']}),
            client.invalidateQueries({queryKey: ['overview']}),
            client.invalidateQueries({queryKey: ['notes']}),
            client.invalidateQueries({queryKey: ['note']}),
        ])
    }, [client])

    useEffect(() => {
        if (!isDesktop) return
        return EventsOn('sync:status', () => {
            void invalidate()
            void client.invalidateQueries({queryKey: ['notifications']})
            void client.invalidateQueries({queryKey: ['notebook-members']})
            void client.invalidateQueries({queryKey: ['note-comments']})
        })
    }, [client, invalidate])

    const showToast = useCallback((next: Omit<ToastState, 'id'>) => {
        window.clearTimeout(toastTimer.current)
        setToast({id: Date.now(), ...next})
        toastTimer.current = window.setTimeout(() => setToast(null), next.actionLabel ? 6000 : 4200)
    }, [])

	const changeCompactMode = useCallback(async (enabled: boolean) => {
		setCompactMode(enabled)
		try {
			await api.SetCompactMode(enabled)
			if (enabled) {
				setSelectedId('')
				setSelectedNoteId('')
				setNotificationsOpen(false)
				setSettingsOpen(false)
			}
		} catch (error) {
			setCompactMode(!enabled)
			showToast({message: errorMessage(error), tone: 'info'})
		}
	}, [showToast])

    useEffect(() => () => window.clearTimeout(toastTimer.current), [])

    const createTask = useMutation({
        mutationFn: (title: string) => api.CreateTask({
            title, notes: '', listId: view === 'list' ? listId : '', priority: 'none',
            dueDate: view === 'today' ? localToday() : '', tagIds: tagId ? [tagId] : [],
        }),
        onSuccess: async (task) => { await invalidate(); setSelectedId(task.id) },
        onError: (error) => showToast({message: errorMessage(error), tone: 'info'}),
    })

    const createCompactTask = useMutation({
        mutationFn: (title: string) => api.CreateTask({
            title,
            notes: '',
            listId: compactSelection.view === 'list' ? compactSelection.listId : '',
            priority: 'none',
            dueDate: compactSelection.view === 'today' ? localToday() : compactSelection.view === 'upcoming' ? localTomorrow() : '',
            tagIds: compactSelection.tagId ? [compactSelection.tagId] : [],
        }),
        onSuccess: async () => {await invalidate()},
        onError: (error) => showToast({message: errorMessage(error), tone: 'info'}),
    })

    const updateTask = useMutation({
        mutationFn: (input: UpdateTaskInput) => api.UpdateTask(input),
        onSuccess: async () => { await invalidate(); showToast({message: '更改已保存'}) },
        onError: (error) => showToast({message: errorMessage(error), tone: 'info'}),
    })

    const toggleTask = useMutation({
        mutationFn: ({task, completed}: {task: Task; completed: boolean}) => api.SetTaskCompleted(task.id, completed),
        onSuccess: async (_, variables) => {
            await invalidate()
            showToast({
                message: variables.completed ? '任务已完成' : '任务已恢复',
                actionLabel: '撤销',
                onAction: () => {
                    setToast(null)
                    toggleTask.mutate({task: variables.task, completed: !variables.completed})
                },
            })
        },
        onError: (error) => showToast({message: errorMessage(error), tone: 'info'}),
    })

    const tasks = taskQuery.data ?? []
    const compactTasks = compactTasksQuery.data ?? []
    const selectedTask = tasks.find((task) => task.id === selectedId)
    const relatedNotesQuery = useQuery({
        queryKey: ['notes', 'task', selectedTask?.id],
        queryFn: () => api.ListNotes({notebookId: '', tagId: '', taskId: selectedTask!.id, search: '', pinnedOnly: false}),
        enabled: Boolean(selectedTask),
    })
    useEffect(() => {
        if (selectedId && taskQuery.data && !taskQuery.data.some((task) => task.id === selectedId)) setSelectedId('')
    }, [selectedId, taskQuery.data])
    useEffect(() => {
        if (!pendingTaskId || !taskQuery.data) return
        if (taskQuery.data.some((task) => task.id === pendingTaskId)) {
            setSelectedId(pendingTaskId)
            setPendingTaskId('')
        }
    }, [pendingTaskId, taskQuery.data])

    const selectView = (next: ViewKey, id = '') => {
        setView(next); setListId(next === 'list' ? id : ''); setTagId(''); setSelectedId(''); setSearch('')
        if (next !== 'notes') {setSelectedNoteId(''); setNotebookId(''); setNoteTagId('')}
    }

    const selectTag = (id: string) => {
        setTagId(id)
        if (view === 'overview' || view === 'completed') setView('all')
        setSelectedId('')
    }

    const enterCompactMode = useCallback(() => {
        const compactView: CompactViewKey = view === 'overview' || view === 'notes' ? 'today' : view
        setCompactSelection({
            view: tagId ? 'all' : compactView,
            listId: !tagId && compactView === 'list' ? listId : '',
            tagId,
        })
        void changeCompactMode(true)
    }, [changeCompactMode, view, listId, tagId])

    const exitCompactMode = useCallback((taskId = '') => {
        setView(compactSelection.view)
        setListId(compactSelection.view === 'list' ? compactSelection.listId : '')
        setTagId(compactSelection.tagId)
        setNotebookId('')
        setNoteTagId('')
        setSelectedNoteId('')
        setSearch('')
        setSelectedId('')
        if (taskId) setPendingTaskId(taskId)
        void changeCompactMode(false)
    }, [changeCompactMode, compactSelection])

    useEffect(() => {
        const onKeyDown = (event: KeyboardEvent) => {
			if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === 'm') {
				event.preventDefault()
				if (compactMode) exitCompactMode()
				else enterCompactMode()
				return
			}
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'n') {
                event.preventDefault()
				if (compactMode) quickInputRef.current?.focus()
				else if (view === 'notes') document.querySelector<HTMLButtonElement>('[data-new-note]')?.click()
                else {if (view === 'completed' || view === 'overview') selectView('today'); window.setTimeout(() => quickInputRef.current?.focus(), 0)}
            }
			if (!compactMode && (event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {event.preventDefault(); if (view === 'notes') document.querySelector<HTMLInputElement>('#note-search')?.focus(); else searchInputRef.current?.focus()}
            if (event.key === 'Escape' && selectedId) {
                const target = event.target instanceof HTMLElement ? event.target : null
                if (target?.closest('.select-trigger, [data-date-picker-trigger], [role="dialog"], [role="menu"]')) return
                setSelectedId('')
            }
        }
        window.addEventListener('keydown', onKeyDown)
        return () => window.removeEventListener('keydown', onKeyDown)
	}, [view, selectedId, compactMode, enterCompactMode, exitCompactMode])

    const currentCopy = useMemo(() => {
        if (tagId) {
            const tag = bootstrapQuery.data?.tags.find((item) => item.id === tagId)
            if (tag) return {title: tag.name, subtitle: '按标签筛选的任务'}
        }
        if (view === 'list') {
            const list = bootstrapQuery.data?.lists.find((item) => item.id === listId)
            return {title: list?.name ?? '清单', subtitle: '这个清单中的未完成任务'}
        }
        return view === 'overview' ? {title: '概览', subtitle: ''} : view === 'notes' ? {title: '笔记', subtitle: ''} : viewCopy[view]
    }, [view, listId, tagId, bootstrapQuery.data])

    const submitEntity = async (name: string, color: string) => {
        if (!editor) return
        setDialogBusy(true)
        try {
            if (editor.kind === 'list') {
                if (editor.item) await api.UpdateList({id: editor.item.id, name})
                else await api.CreateList(name)
            } else if (editor.kind === 'tag') {
                if (editor.item) await api.UpdateTag({id: editor.item.id, name, color})
                else await api.CreateTag({id: '', name, color})
            } else if (editor.kind === 'notebook') {
                if (editor.item) await api.UpdateNotebook({id: editor.item.id, name})
                else await api.CreateNotebook(name)
            } else {
                if (editor.item) await api.UpdateNoteTag({id: editor.item.id, notebookId: editor.notebookId, name, color})
                else await api.CreateNoteTag({id: '', notebookId: editor.notebookId, name, color})
            }
            await invalidate(); setEditor(null); showToast({message: '已保存'})
        } catch (error) { showToast({message: errorMessage(error), tone: 'info'}) }
        finally { setDialogBusy(false) }
    }

    const requestDeleteList = (list: TodoList) => setConfirm({
        title: `删除“${list.name}”？`,
        description: '清单中的任务会移回收集箱，任务本身不会被删除。',
        confirmLabel: '删除清单',
        onConfirm: async () => {
            setDialogBusy(true)
            try { await api.DeleteList(list.id); if (listId === list.id) selectView('inbox'); await invalidate(); setConfirm(null); showToast({message: '清单已删除'}) }
            catch (error) { showToast({message: errorMessage(error), tone: 'info'}) }
            finally { setDialogBusy(false) }
        },
    })

    const requestDeleteTag = (tag: Tag) => setConfirm({
        title: `删除“${tag.name}”？`, description: '标签会从相关任务中移除，任务不会被删除。', confirmLabel: '删除标签',
        onConfirm: async () => {
            setDialogBusy(true)
            try { await api.DeleteTag(tag.id); if (tagId === tag.id) setTagId(''); await invalidate(); setConfirm(null); showToast({message: '标签已删除'}) }
            catch (error) { showToast({message: errorMessage(error), tone: 'info'}) }
            finally { setDialogBusy(false) }
        },
    })

    const requestDeleteTask = (task: Task) => setConfirm({
        title: '删除这个任务？', description: '任务会暂时保留，可以立即撤销。', confirmLabel: '删除任务',
        onConfirm: async () => {
            setDialogBusy(true)
            try {
                await api.DeleteTask(task.id); setSelectedId(''); await invalidate(); setConfirm(null)
                showToast({message: '任务已删除', actionLabel: '撤销', onAction: () => {setToast(null); void api.RestoreTask(task.id).then(invalidate).catch((error) => showToast({message: errorMessage(error), tone: 'info'}))}})
            } catch (error) { showToast({message: errorMessage(error), tone: 'info'}) }
            finally { setDialogBusy(false) }
        },
    })

    const requestDeleteNotebook = (notebook: Notebook) => setConfirm({
        title: `删除“${notebook.name}”？`,
        description: notebook.kind === 'shared' ? '共享笔记本及其中内容会进入回收状态，所有成员都将无法继续编辑。' : '其中的笔记会移入“我的笔记”，内容不会丢失。',
        confirmLabel: '删除笔记本',
        onConfirm: async () => {
            setDialogBusy(true)
            try {await api.DeleteNotebook(notebook.id); if (notebookId === notebook.id) {setNotebookId(''); setNoteTagId('')} await invalidate(); setConfirm(null); showToast({message: '笔记本已删除'})}
            catch (error) {showToast({message: errorMessage(error), tone: 'info'})}
            finally {setDialogBusy(false)}
        },
    })

    const requestDeleteNoteTag = (tag: NoteTag) => setConfirm({
        title: `删除“${tag.name}”？`, description: '标签会从相关笔记中解除，笔记内容不会被删除。', confirmLabel: '删除笔记标签',
        onConfirm: async () => {
            setDialogBusy(true)
            try {await api.DeleteNoteTag(tag.id); if (noteTagId === tag.id) setNoteTagId(''); await invalidate(); setConfirm(null); showToast({message: '笔记标签已删除'})}
            catch (error) {showToast({message: errorMessage(error), tone: 'info'})}
            finally {setDialogBusy(false)}
        },
    })

    const requestDeleteNote = (note: NoteSummary | Note) => setConfirm({
        title: '删除这篇笔记？', description: '笔记会暂时保留，可以立即撤销。', confirmLabel: '删除笔记',
        onConfirm: async () => {
            setDialogBusy(true)
            try {
                await api.DeleteNote(note.id); setSelectedNoteId(''); await invalidate(); setConfirm(null)
                showToast({message: '笔记已删除', actionLabel: '撤销', onAction: () => {setToast(null); void api.RestoreNote(note.id).then(invalidate).catch((error) => showToast({message: errorMessage(error), tone: 'info'}))}})
            } catch (error) {showToast({message: errorMessage(error), tone: 'info'})}
            finally {setDialogBusy(false)}
        },
    })

    const openTaskFromNote = (task: TaskReference) => {
        setView(task.completedAt ? 'completed' : 'all'); setNotebookId(''); setNoteTagId(''); setSelectedNoteId(''); setListId(''); setTagId(''); setPendingTaskId(task.id)
    }

    const openNoteFromTask = (note: NoteSummary) => {
        setView('notes'); setSelectedId(''); setListId(''); setTagId(''); setNotebookId(note.notebookId); setNoteTagId(''); setSelectedNoteId(note.id)
    }

    const createLinkedNote = async (task: Task) => {
        const targetNotebook = bootstrapQuery.data?.notebooks.find((item) => item.default)?.id || bootstrapQuery.data?.notebooks[0]?.id || ''
        try {
            const note = await api.CreateNote({notebookId: targetNotebook, title: task.title, content: `# ${task.title}\n\n`, tagIds: [], taskIds: [task.id]})
            await invalidate(); setView('notes'); setSelectedId(''); setListId(''); setTagId(''); setNotebookId(targetNotebook); setNoteTagId(''); setSelectedNoteId(note.id)
        } catch (error) {showToast({message: errorMessage(error), tone: 'info'})}
    }

    const saveSettings = async (next: AppSettings) => {
        setDialogBusy(true)
        try { await api.UpdateSettings(next); await invalidate(); setSettingsOpen(false); showToast({message: '设置已保存'}) }
        catch (error) { showToast({message: errorMessage(error), tone: 'info'}) }
        finally { setDialogBusy(false) }
    }

    const login = async (input: LoginInput) => {
        setDialogBusy(true)
        try {await api.Login(input); await invalidate(); showToast({message: '同步服务器已连接'})}
        catch (error) {showToast({message: errorMessage(error), tone: 'info'})}
        finally {setDialogBusy(false)}
    }

    const logout = async () => {
        setDialogBusy(true)
        try {await api.Logout(); await invalidate(); showToast({message: '已退出同步账号，本地数据仍然保留'})}
        catch (error) {showToast({message: errorMessage(error), tone: 'info'})}
        finally {setDialogBusy(false)}
    }

    const syncNow = async () => {
        setDialogBusy(true)
        try {await api.SyncNow(); await invalidate(); showToast({message: '同步已完成'})}
        catch (error) {await invalidate(); showToast({message: errorMessage(error), tone: 'info'})}
        finally {setDialogBusy(false)}
    }

    const acceptInvitation = async (token: string) => {
        setDialogBusy(true)
        try {await api.AcceptInvitation(token); await invalidate(); showToast({message: '已加入共享笔记本'})}
        catch (error) {showToast({message: errorMessage(error), tone: 'info'})}
        finally {setDialogBusy(false)}
    }

    const openNotification = async (notification: AppNotification) => {
        try {
            if (!notification.readAt) await api.MarkNotificationRead(notification.id)
            const note = await api.GetNote(notification.entityId)
            setView('notes'); setSelectedId(''); setListId(''); setTagId(''); setNotebookId(note.notebookId); setNoteTagId(''); setSelectedNoteId(note.id); setNotificationsOpen(false)
            await Promise.all([client.invalidateQueries({queryKey: ['bootstrap']}), client.invalidateQueries({queryKey: ['notifications']})])
        } catch (error) {showToast({message: errorMessage(error), tone: 'info'})}
    }

    const exportBackup = async () => {
        setDialogBusy(true)
        try { const result = await api.ExportBackup(); if (!result.cancelled) showToast({message: `已导出 ${result.tasks} 项任务、${result.notes} 篇笔记和 ${result.attachments} 个附件`}) }
        catch (error) { showToast({message: errorMessage(error), tone: 'info'}) }
        finally { setDialogBusy(false) }
    }

    const requestImport = async () => {
        setConfirm({
            title: '恢复本地备份？', description: '备份内容会替换当前数据。操作前会自动保存当前数据。', confirmLabel: '选择备份',
            onConfirm: async () => {
                setDialogBusy(true)
                try {
                    const result = await api.ImportBackup()
                    if (!result.cancelled) {await invalidate(); setSelectedId(''); setSelectedNoteId(''); setConfirm(null); setSettingsOpen(false); showToast({message: `已恢复 ${result.tasks} 项任务和 ${result.notes} 篇笔记`})}
                    else setConfirm(null)
                } catch (error) { showToast({message: errorMessage(error), tone: 'info'}); setConfirm(null) }
                finally { setDialogBusy(false) }
            },
        })
    }

    const openTaskFromCompact = (task: Task) => exitCompactMode(task.id)

    return (
        <Tooltip.Provider>
			{compactMode ? (
				<CompactTodoWindow
					tasks={compactTasks}
					bootstrap={bootstrapQuery.data}
					selection={compactSelection}
					isLoading={compactTasksQuery.isLoading}
					error={compactTasksQuery.error as Error | null}
					quickInputRef={quickInputRef}
					onSelectionChange={setCompactSelection}
					onCreate={async (title) => {await createCompactTask.mutateAsync(title)}}
					onToggle={(task) => toggleTask.mutate({task, completed: !task.completedAt})}
					onDelete={requestDeleteTask}
					onOpenTask={openTaskFromCompact}
					onExit={() => exitCompactMode()}
				/>
			) : (
			<div className={`app-shell ${view === 'overview' ? 'is-overview' : ''} ${view === 'notes' ? 'is-notes' : ''}`}>
                <Sidebar
                    bootstrap={bootstrapQuery.data}
                    selection={selection}
                    onSelect={selectView}
                    onSelectTag={selectTag}
                    onAddList={() => setEditor({kind: 'list'})}
                    onEditList={(item) => setEditor({kind: 'list', item})}
                    onDeleteList={requestDeleteList}
                    onAddTag={() => setEditor({kind: 'tag'})}
                    onEditTag={(item) => setEditor({kind: 'tag', item})}
                    onDeleteTag={requestDeleteTag}
                    onSelectNotebook={(id) => {setNotebookId(id); setNoteTagId(''); setSelectedNoteId('')}}
                    onSelectNoteTag={(id) => {setNoteTagId(id); setSelectedNoteId('')}}
                    onAddNotebook={() => setEditor({kind: 'notebook'})}
                    onEditNotebook={(item) => setEditor({kind: 'notebook', item})}
                    onDeleteNotebook={requestDeleteNotebook}
                    onAddNoteTag={() => {
                        const targetNotebook = notebookId || bootstrapQuery.data?.notebooks.find((item) => item.default)?.id
                        if (targetNotebook) setEditor({kind: 'noteTag', notebookId: targetNotebook})
                        else showToast({message: '请先创建笔记本', tone: 'info'})
                    }}
                    onEditNoteTag={(item) => setEditor({kind: 'noteTag', notebookId: item.notebookId, item})}
                    onDeleteNoteTag={requestDeleteNoteTag}
                    onOpenNotifications={() => setNotificationsOpen(true)}
                    onOpenSettings={() => setSettingsOpen(true)}
					onOpenCompact={enterCompactMode}
                />
                {view === 'overview' ? (
                    <OverviewPanel overview={overviewQuery.data} isLoading={overviewQuery.isLoading} error={overviewQuery.error as Error | null} onOpenToday={() => selectView('today')}/>
                ) : view === 'notes' ? (
                    <Suspense fallback={<div className="workspace-loading"><span/><span/><span/></div>}><NoteWorkspace bootstrap={bootstrapQuery.data} notebookId={notebookId} noteTagId={noteTagId} selectedId={selectedNoteId} onSelect={setSelectedNoteId} onOpenTask={openTaskFromNote} onRequestDelete={requestDeleteNote} showToast={(message) => showToast({message, tone: 'info'})}/></Suspense>
                ) : (
                    <TaskList
                        title={currentCopy.title} subtitle={currentCopy.subtitle} view={view} tasks={tasks}
                        lists={bootstrapQuery.data?.lists ?? []} tags={bootstrapQuery.data?.tags ?? []} selectedId={selectedId} isLoading={taskQuery.isLoading}
                        error={taskQuery.error as Error | null} search={search} priority={priority} sort={sort}
                        quickInputRef={quickInputRef} searchInputRef={searchInputRef}
                        onSearch={setSearch} onPriority={setPriority} onSort={setSort}
                        onSelect={(task) => setSelectedId(task.id)} onToggle={(task) => toggleTask.mutate({task, completed: !task.completedAt})}
                        onUpdate={(input) => updateTask.mutate(input)}
                        onOpenDatePicker={(task) => {
                            setSelectedId(task.id)
                            window.setTimeout(() => document.querySelector<HTMLButtonElement>('[data-date-picker-trigger]')?.click(), 180)
                        }}
                        onDelete={requestDeleteTask}
                        onCreateLinkedNote={(task) => void createLinkedNote(task)}
                        onCreate={async (title) => {await createTask.mutateAsync(title)}}
                    />
                )}

                <div className={`detail-slot ${selectedTask ? 'is-open' : ''}`}>
                    <AnimatePresence mode="wait">
                        {selectedTask ? (
                            <motion.div key={selectedTask.id} className="detail-motion" initial={reduce ? false : {opacity: 0, x: 24}} animate={{opacity: 1, x: 0}} exit={reduce ? {opacity: 0} : {opacity: 0, x: 18}} transition={reduce ? {duration: 0} : {type: 'spring', stiffness: 380, damping: 34}}>
                                <TaskDetail task={selectedTask} lists={bootstrapQuery.data?.lists ?? []} tags={bootstrapQuery.data?.tags ?? []} saving={updateTask.isPending} onSave={async (input) => {await updateTask.mutateAsync(input)}} onDelete={requestDeleteTask} relatedNotes={relatedNotesQuery.data ?? []} onOpenNote={openNoteFromTask} onCreateLinkedNote={() => void createLinkedNote(selectedTask)} onClose={() => setSelectedId('')}/>
                            </motion.div>
                        ) : (
                            <div className="detail-placeholder"><span><PanelRightOpen size={24}/></span><strong>选择一个任务</strong><p>在这里查看详情和编辑属性。</p></div>
                        )}
                    </AnimatePresence>
                </div>
                {selectedTask && <button className="detail-backdrop" aria-label="关闭任务详情" onClick={() => setSelectedId('')}/>} 
            </div>
			)}

            <EntityDialog editor={editor} busy={dialogBusy} onClose={() => setEditor(null)} onSubmit={submitEntity}/>
            <ConfirmDialog state={confirm} busy={dialogBusy} onClose={() => setConfirm(null)}/>
            <SettingsDialog open={settingsOpen} settings={settings} user={bootstrapQuery.data?.currentUser ?? {id: 'local', username: '本地用户', mode: 'local'}} sync={bootstrapQuery.data?.sync ?? {configured: false, online: false, syncing: false, pending: 0, serverUrl: '', lastSyncAt: '', lastError: ''}} busy={dialogBusy} onClose={() => setSettingsOpen(false)} onSave={saveSettings} onExport={exportBackup} onImport={requestImport} onLogin={login} onLogout={logout} onSync={syncNow} onAcceptInvitation={acceptInvitation}/>
            <NotificationCenter open={notificationsOpen} items={notificationsQuery.data ?? []} loading={notificationsQuery.isLoading} error={notificationsQuery.error as Error | null} onClose={() => setNotificationsOpen(false)} onOpen={openNotification}/>
            <AppToast toast={toast} onClose={() => setToast(null)}/>
        </Tooltip.Provider>
    )
}

export default App
