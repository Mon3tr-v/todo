import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import App from './App'
import {api} from './lib/api'

function renderApp() {
    const client = new QueryClient({defaultOptions: {queries: {retry: false, staleTime: 0}, mutations: {retry: false}}})
    return render(<QueryClientProvider client={client}><App/></QueryClientProvider>)
}

describe('TODO application', () => {
    beforeEach(() => {
        localStorage.clear()
        document.documentElement.removeAttribute('data-theme')
    })

    afterEach(() => {cleanup(); vi.restoreAllMocks()})

    it('creates a task from the quick composer', async () => {
        const user = userEvent.setup()
        renderApp()
        const input = await screen.findByLabelText('添加任务')
        await user.type(input, '整理桌面{Enter}')
        expect((await screen.findAllByText('整理桌面')).length).toBeGreaterThan(0)
        expect(screen.getByLabelText('任务详情')).toBeInTheDocument()
    })

    it('filters tasks by search text', async () => {
        const user = userEvent.setup()
        renderApp()
        const input = await screen.findByLabelText('添加任务')
        await user.type(input, '整理桌面{Enter}')
        await user.type(input, '回复邮件{Enter}')
        await screen.findByText('回复邮件')
        const search = screen.getByLabelText('搜索任务')
        await user.type(search, '桌面')
        expect((await screen.findAllByText('整理桌面')).length).toBeGreaterThan(0)
        await waitFor(() => expect(screen.queryAllByText('回复邮件')).toHaveLength(0))
    })

    it('shows positive sidebar counts and hides zero counts', async () => {
        const user = userEvent.setup()
        renderApp()
        const personal = (await screen.findByText('个人')).closest('button')!
        const work = screen.getByText('工作').closest('button')!
        expect(personal.querySelector('.sidebar-count')).not.toBeInTheDocument()
        expect(work.querySelector('.sidebar-count')).not.toBeInTheDocument()
        await user.click(personal)
        await user.type(screen.getByLabelText('添加任务'), '个人清单任务{Enter}')
        await waitFor(() => expect(personal.querySelector('.sidebar-count')).toHaveTextContent('1'))
        expect(work.querySelector('.sidebar-count')).not.toBeInTheDocument()
        expect(screen.getByRole('button', {name: /全部任务/}).querySelector('.sidebar-count')).toHaveTextContent('1')
    })

    it('completes and restores a task with undo', async () => {
        const user = userEvent.setup()
        renderApp()
        const input = await screen.findByLabelText('添加任务')
        await user.type(input, '完成测试{Enter}')
        await screen.findAllByText('完成测试')
        await user.click(screen.getByLabelText('标记为已完成'))
        expect(await screen.findByText('任务已完成')).toBeInTheDocument()
        await waitFor(() => expect(screen.queryAllByText('完成测试')).toHaveLength(0))
        await user.click(screen.getByRole('button', {name: '撤销'}))
        expect((await screen.findAllByText('完成测试')).length).toBeGreaterThan(0)
    })

    it('supports keyboard focus shortcuts', async () => {
        const user = userEvent.setup()
        renderApp()
        const quickInput = await screen.findByLabelText('添加任务')
        await user.keyboard('{Control>}n{/Control}')
        await waitFor(() => expect(quickInput).toHaveFocus())
        await user.keyboard('{Control>}k{/Control}')
        await waitFor(() => expect(screen.getByLabelText('搜索任务')).toHaveFocus())
    })

	it('opens the always-on-top compact view for quick task work', async () => {
		const user = userEvent.setup()
		const compactSpy = vi.spyOn(api, 'SetCompactMode')
		renderApp()
		await user.click((await screen.findByText('悬浮小窗')).closest('button')!)
		expect(await screen.findByRole('main', {name: '悬浮任务小窗'})).toBeInTheDocument()
		await waitFor(() => expect(compactSpy).toHaveBeenCalledWith(true))

		const input = screen.getByLabelText('添加今日任务')
		await user.type(input, '悬浮窗口任务{Enter}')
		expect(await screen.findByText('悬浮窗口任务')).toBeInTheDocument()
		await user.click(screen.getByLabelText('标记为已完成'))
		await waitFor(() => expect(screen.queryByText('悬浮窗口任务')).not.toBeInTheDocument())

		await user.click(screen.getByRole('button', {name: '返回主窗口'}))
		await waitFor(() => expect(screen.queryByRole('main', {name: '悬浮任务小窗'})).not.toBeInTheDocument())
		expect(compactSpy).toHaveBeenLastCalledWith(false)
		expect(await screen.findByText('设置与备份')).toBeInTheDocument()
	})

	it('switches compact task views and restores completed tasks', async () => {
		const user = userEvent.setup()
		const createSpy = vi.spyOn(api, 'CreateTask')
		renderApp()
		await user.click((await screen.findByText('悬浮小窗')).closest('button')!)

		await user.click(await screen.findByRole('button', {name: '切换小窗视图，当前今天'}))
		expect(screen.getByRole('menuitem', {name: '收集箱'})).toBeInTheDocument()
		expect(screen.getByRole('menuitem', {name: '即将到期'})).toBeInTheDocument()
		expect(screen.getByRole('menuitem', {name: '全部任务'})).toBeInTheDocument()
		expect(screen.getByRole('menuitem', {name: '已完成'})).toBeInTheDocument()
		expect(screen.getByRole('menuitem', {name: '个人'})).toBeInTheDocument()
		await user.click(screen.getByRole('menuitem', {name: '即将到期'}))

		const upcomingInput = await screen.findByLabelText('添加明日任务')
		await user.type(upcomingInput, '明日跟进任务{Enter}')
		await waitFor(() => expect(createSpy).toHaveBeenLastCalledWith(expect.objectContaining({title: '明日跟进任务'})))
		const createdInput = createSpy.mock.calls.at(-1)?.[0]
		expect(Boolean(createdInput?.dueDate && createdInput.dueDate > new Date().toLocaleDateString('sv-SE'))).toBe(true)

		await user.click(screen.getByRole('button', {name: '切换小窗视图，当前即将到期'}))
		await user.click(screen.getByRole('menuitem', {name: '全部任务'}))
		expect(await screen.findByText('明日跟进任务')).toBeInTheDocument()
		await user.click(screen.getByLabelText('标记为已完成'))
		await waitFor(() => expect(screen.queryByText('明日跟进任务')).not.toBeInTheDocument())

		await user.click(screen.getByRole('button', {name: '切换小窗视图，当前全部任务'}))
		await user.click(screen.getByRole('menuitem', {name: '已完成'}))
		expect(await screen.findByText('明日跟进任务')).toBeInTheDocument()
		await user.click(screen.getByLabelText('恢复为未完成'))
		await waitFor(() => expect(screen.queryByText('明日跟进任务')).not.toBeInTheDocument())
	})

    it('persists the dark theme setting', async () => {
        const user = userEvent.setup()
        renderApp()
        await user.click((await screen.findByText('设置与备份')).closest('button')!)
        await waitFor(() => expect(screen.getByRole('button', {name: '跟随系统'})).toHaveFocus())
        expect(document.querySelector('.tooltip')).not.toBeInTheDocument()
        await user.click(screen.getByRole('button', {name: '深色'}))
        await user.click(screen.getByRole('button', {name: '保存设置'}))
        await waitFor(() => expect(document.documentElement.dataset.theme).toBe('dark'))
    })

    it('sends one argument when saving a high priority task', async () => {
        const user = userEvent.setup()
        const updateSpy = vi.spyOn(api, 'UpdateTask')
        renderApp()
        const input = await screen.findByLabelText('添加任务')
        await user.type(input, '修复 Wails 参数{Enter}')
        await screen.findByLabelText('任务标题')
        await user.click(screen.getByRole('button', {name: '高'}))
        await user.click(screen.getByRole('button', {name: '保存更改'}))
        await waitFor(() => expect(updateSpy).toHaveBeenCalledTimes(1))
        expect(updateSpy.mock.calls[0]).toHaveLength(1)
        expect(updateSpy.mock.calls[0][0]).toMatchObject({title: '修复 Wails 参数', priority: 'high'})
    })

    it('opens task actions on right click and deletes after confirmation', async () => {
        const user = userEvent.setup()
        renderApp()
        const input = await screen.findByLabelText('添加任务')
        await user.type(input, '右键删除任务{Enter}')
        const row = (await screen.findAllByText('右键删除任务'))[0].closest('.task-row')!
        fireEvent.contextMenu(row)
        await user.click(await screen.findByRole('menuitem', {name: '删除任务'}))
        expect(await screen.findByRole('heading', {name: '删除这个任务？'})).toBeInTheDocument()
        await user.click(screen.getByRole('button', {name: '删除任务'}))
        expect(await screen.findByText('任务已删除')).toBeInTheDocument()
        await waitFor(() => expect(screen.queryAllByText('右键删除任务')).toHaveLength(0))
    })

    it('shows task property groups in the right click menu', async () => {
        const user = userEvent.setup()
        renderApp()
        const input = await screen.findByLabelText('添加任务')
        await user.type(input, '右键修改属性{Enter}')
        const row = (await screen.findAllByText('右键修改属性'))[0].closest('.task-row')!
        fireEvent.contextMenu(row)
        expect(await screen.findByRole('menuitem', {name: '清单'})).toBeInTheDocument()
        expect(screen.getByRole('menuitem', {name: '截止日期'})).toBeInTheDocument()
        expect(screen.getByRole('menuitem', {name: '标签'})).toBeInTheDocument()
        await user.hover(await screen.findByRole('menuitem', {name: '优先级'}))
        expect(await screen.findByRole('menuitemradio', {name: '高优先级'})).toBeInTheDocument()
    })

    it('uses a styled select and supports choosing a startup view', async () => {
        const user = userEvent.setup()
        renderApp()
        await user.click((await screen.findByText('设置与备份')).closest('button')!)
        const select = screen.getByRole('combobox', {name: '启动视图'})
        expect(select.tagName).toBe('BUTTON')
        expect(document.querySelector('select')).not.toBeInTheDocument()
        await user.click(select)
        await user.click(await screen.findByRole('option', {name: '概览'}))
        expect(select).toHaveTextContent('概览')
    })

    it('closes a select with Escape without closing task details', async () => {
        const user = userEvent.setup()
        renderApp()
        const input = await screen.findByLabelText('添加任务')
        await user.type(input, '保留详情面板{Enter}')
        await screen.findByLabelText('任务详情')
        await user.click(screen.getByRole('combobox', {name: '优先级'}))
        await user.keyboard('{Escape}')
        expect(screen.getByLabelText('任务详情')).toBeInTheDocument()
        expect(screen.queryByRole('option', {name: '高优先级'})).not.toBeInTheDocument()
    })

    it('opens a collaboration notification and marks it as read', async () => {
        const user = userEvent.setup()
        const createdAt = new Date().toISOString()
        vi.spyOn(api, 'ListNotifications').mockResolvedValue([{id: 'notice-1', kind: 'mention', entityId: 'note-1', message: '林舟在评论中提到了你', readAt: '', createdAt}])
        const markRead = vi.spyOn(api, 'MarkNotificationRead').mockResolvedValue()
        vi.spyOn(api, 'GetNote').mockResolvedValue({id: 'note-1', notebookId: 'notebook-default', title: '协作记录', content: '', pinned: false, revision: 1, deletedAt: '', tags: [], tasks: [], attachments: [], createdAt, updatedAt: createdAt, serverVersion: 1})
        renderApp()
        await user.click((await screen.findByText('通知')).closest('button')!)
        expect(await screen.findByText('林舟在评论中提到了你')).toBeInTheDocument()
        await user.click(screen.getByText('林舟在评论中提到了你'))
        await waitFor(() => expect(markRead).toHaveBeenCalledWith('notice-1'))
        await waitFor(() => expect(screen.queryByRole('heading', {name: '通知'})).not.toBeInTheDocument())
    })

    it('autosaves Markdown notes and renders preview content', async () => {
        const user = userEvent.setup()
        const updateNote = vi.spyOn(api, 'UpdateNote')
        renderApp()
        await user.click((await screen.findByText('笔记')).closest('button')!)
        await waitFor(() => expect(document.querySelector('[data-new-note]')).toBeInTheDocument())
        await user.click(document.querySelector<HTMLButtonElement>('[data-new-note]')!)
        const title = await screen.findByLabelText('笔记标题')
        const content = screen.getByLabelText('笔记正文')
        await user.clear(title)
        await user.type(title, '自动保存测试')
        await user.type(content, '# Markdown 标题')
        await waitFor(() => expect(updateNote).toHaveBeenCalled(), {timeout: 3_000})
        expect(updateNote.mock.calls.at(-1)?.[0]).toMatchObject({title: '自动保存测试', content: '# Markdown 标题'})
        await user.click(screen.getByRole('button', {name: '预览'}))
        expect(await screen.findByRole('heading', {name: 'Markdown 标题'})).toBeInTheDocument()
    })

    it('disables note creation in a read-only shared notebook', async () => {
        const user = userEvent.setup()
        const bootstrap = await api.GetBootstrap()
        const shared = {...bootstrap.notebooks[0], id: 'shared-viewer', name: '只读共享', kind: 'shared' as const, role: 'viewer' as const, ownerId: 'remote-owner', default: false, serverVersion: 3}
        vi.spyOn(api, 'GetBootstrap').mockResolvedValue({...bootstrap, notebooks: [...bootstrap.notebooks, shared]})
        renderApp()
        await user.click((await screen.findByText('笔记')).closest('button')!)
        await user.click((await screen.findByText('只读共享')).closest('button')!)
        await waitFor(() => expect(document.querySelector<HTMLButtonElement>('[data-new-note]')).toBeDisabled())
        expect(screen.getByLabelText('新建笔记标签')).toBeDisabled()
    })
})
