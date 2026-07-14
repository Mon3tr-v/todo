import {useEffect, useRef, useState} from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import {Cloud, DatabaseBackup, Download, LogIn, LogOut, Monitor, Moon, RefreshCw, Sun, Upload, X} from 'lucide-react'
import type {AccountUser, AppSettings, LoginInput, Notebook, NoteTag, SyncStatus, Tag, Theme, TodoList} from '../types'
import {CustomSelect} from './CustomSelect'
import {IconButton} from './IconButton'

const colors = [
    {value: 'coral', label: '珊瑚'}, {value: 'amber', label: '琥珀'}, {value: 'mint', label: '薄荷'},
    {value: 'blue', label: '蓝'}, {value: 'violet', label: '紫罗兰'}, {value: 'rose', label: '玫瑰'},
]

export type EntityEditor =
    | {kind: 'list'; item?: TodoList}
    | {kind: 'tag'; item?: Tag}
    | {kind: 'notebook'; item?: Notebook}
    | {kind: 'noteTag'; notebookId: string; item?: NoteTag}
    | null

export function EntityDialog({editor, busy, onClose, onSubmit}: {
    editor: EntityEditor
    busy: boolean
    onClose: () => void
    onSubmit: (name: string, color: string) => Promise<void>
}) {
    const [name, setName] = useState('')
    const [color, setColor] = useState('coral')
    useEffect(() => {
        setName(editor?.item?.name ?? '')
        setColor(editor?.kind === 'tag' || editor?.kind === 'noteTag' ? editor.item?.color ?? 'coral' : 'coral')
    }, [editor])
    if (!editor) return null
    const isTag = editor.kind === 'tag' || editor.kind === 'noteTag'
    const isNoteEntity = editor.kind === 'notebook' || editor.kind === 'noteTag'
    const editing = Boolean(editor.item)
    const entityLabel = editor.kind === 'notebook' ? '笔记本' : editor.kind === 'noteTag' ? '笔记标签' : isTag ? '标签' : '清单'
    const title = `${editing ? '编辑' : '新建'}${entityLabel}`
    return (
        <Dialog.Root open onOpenChange={(open) => {if (!open) onClose()}}>
            <Dialog.Portal>
                <Dialog.Overlay className="dialog-overlay"/>
                <Dialog.Content className="dialog-content entity-dialog">
                    <div className="dialog-heading"><div><Dialog.Title>{title}</Dialog.Title><Dialog.Description>{isNoteEntity ? (isTag ? '笔记标签用于筛选当前笔记本中的内容。' : '每篇笔记归属于一个笔记本。') : isTag ? '标签可以跨清单筛选任务。' : '清单用于组织不同领域的任务。'}</Dialog.Description></div><Dialog.Close asChild><IconButton label="关闭"><X size={18}/></IconButton></Dialog.Close></div>
                    <form onSubmit={(event) => {event.preventDefault(); if (name.trim()) void onSubmit(name.trim(), color)}}>
                        <div className="form-field"><label htmlFor="entity-name">名称</label><input id="entity-name" autoFocus value={name} maxLength={isTag ? 28 : editor.kind === 'notebook' ? 60 : 40} onChange={(event) => setName(event.target.value)}/></div>
                        {isTag && <fieldset className="form-field color-picker"><legend>颜色</legend><div>{colors.map((item) => <button key={item.value} type="button" aria-label={item.label} aria-pressed={color === item.value} className={`color-swatch swatch-${item.value} ${color === item.value ? 'is-selected' : ''}`} onClick={() => setColor(item.value)}/>)}</div></fieldset>}
                        <div className="dialog-buttons"><button type="button" className="secondary-button" onClick={onClose}>取消</button><button type="submit" className="primary-button" disabled={!name.trim() || busy}>{busy ? '保存中' : '保存'}</button></div>
                    </form>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    )
}

export interface ConfirmState {title: string; description: string; confirmLabel: string; onConfirm: () => Promise<void>}

export function ConfirmDialog({state, busy, onClose}: {state: ConfirmState | null; busy: boolean; onClose: () => void}) {
    if (!state) return null
    return (
        <Dialog.Root open onOpenChange={(open) => {if (!open) onClose()}}>
            <Dialog.Portal><Dialog.Overlay className="dialog-overlay"/><Dialog.Content className="dialog-content confirm-dialog">
                <Dialog.Title>{state.title}</Dialog.Title><Dialog.Description>{state.description}</Dialog.Description>
                <div className="dialog-buttons"><button className="secondary-button" onClick={onClose}>取消</button><button className="danger-button" disabled={busy} onClick={() => void state.onConfirm()}>{busy ? '处理中' : state.confirmLabel}</button></div>
            </Dialog.Content></Dialog.Portal>
        </Dialog.Root>
    )
}

const themeOptions: Array<{value: Theme; label: string; icon: React.ReactNode}> = [
    {value: 'system', label: '跟随系统', icon: <Monitor size={17}/>}, {value: 'light', label: '浅色', icon: <Sun size={17}/>}, {value: 'dark', label: '深色', icon: <Moon size={17}/>},
]
const startupViewOptions: Array<{value: AppSettings['startupView']; label: string}> = [
    {value: 'today', label: '今天'},
    {value: 'overview', label: '概览'},
    {value: 'notes', label: '笔记'},
    {value: 'inbox', label: '收集箱'},
    {value: 'upcoming', label: '即将到期'},
    {value: 'all', label: '全部任务'},
]

export function SettingsDialog({open, settings, user, sync, busy, onClose, onSave, onExport, onImport, onLogin, onLogout, onSync, onAcceptInvitation}: {
    open: boolean
    settings: AppSettings
    user: AccountUser
    sync: SyncStatus
    busy: boolean
    onClose: () => void
    onSave: (settings: AppSettings) => Promise<void>
    onExport: () => Promise<void>
    onImport: () => Promise<void>
    onLogin: (input: LoginInput) => Promise<void>
    onLogout: () => Promise<void>
    onSync: () => Promise<void>
    onAcceptInvitation: (token: string) => Promise<void>
}) {
    const [draft, setDraft] = useState(settings)
    const [login, setLogin] = useState<LoginInput>({serverUrl: 'http://localhost:8080', username: '', password: '', migrateLocal: false})
    const [inviteToken, setInviteToken] = useState('')
    const initialFocusRef = useRef<HTMLButtonElement>(null)
    useEffect(() => setDraft(settings), [settings, open])
    return (
        <Dialog.Root open={open} onOpenChange={(next) => {if (!next) onClose()}}>
            <Dialog.Portal><Dialog.Overlay className="dialog-overlay"/><Dialog.Content className="dialog-content settings-dialog" onOpenAutoFocus={(event) => {event.preventDefault(); initialFocusRef.current?.focus({preventScroll: true})}}>
                <div className="dialog-heading"><div><Dialog.Title>设置</Dialog.Title><Dialog.Description>调整外观、启动视图并管理本地备份。</Dialog.Description></div><Dialog.Close asChild><IconButton label="关闭"><X size={18}/></IconButton></Dialog.Close></div>
                <section className="settings-section"><h3>外观</h3><div className="theme-control">{themeOptions.map((option) => <button ref={draft.theme === option.value ? initialFocusRef : undefined} key={option.value} type="button" className={draft.theme === option.value ? 'is-active' : ''} onClick={() => setDraft({...draft, theme: option.value})}>{option.icon}<span>{option.label}</span></button>)}</div></section>
                <section className="settings-section"><label htmlFor="startup-view">启动视图</label><CustomSelect id="startup-view" ariaLabel="启动视图" value={draft.startupView} options={startupViewOptions} onValueChange={(startupView) => setDraft({...draft, startupView})} className="field-select settings-select"/></section>
                <section className="settings-section sync-settings">
                    <div className="sync-settings-heading"><div><h3>同步与协作</h3><p>连接自托管 TODO 服务，在设备间同步并共享笔记本。</p></div><Cloud size={19}/></div>
                    {sync.configured ? <div className="sync-account">
                        <div className="sync-account-copy"><span className={sync.lastError ? 'is-error' : sync.online ? 'is-online' : ''}/><div><strong>{user.username}</strong><small>{sync.serverUrl}</small></div></div>
                        <div className="sync-account-meta"><span>{sync.pending > 0 ? `${sync.pending} 项待同步` : sync.lastError || (sync.lastSyncAt ? `上次同步 ${new Intl.DateTimeFormat('zh-CN', {hour: '2-digit', minute: '2-digit'}).format(new Date(sync.lastSyncAt))}` : '等待首次同步')}</span><div><button className="secondary-button" disabled={busy} onClick={() => void onSync()}><RefreshCw size={15}/>立即同步</button><button className="secondary-button" disabled={busy} onClick={() => void onLogout()}><LogOut size={15}/>退出</button></div></div>
                        <div className="accept-invite"><input aria-label="邀请令牌" value={inviteToken} onChange={(event) => setInviteToken(event.target.value)} placeholder="粘贴协作邀请令牌"/><button className="secondary-button" disabled={busy || !inviteToken.trim()} onClick={() => void onAcceptInvitation(inviteToken.trim()).then(() => setInviteToken(''))}>加入笔记本</button></div>
                    </div> : <div className="sync-login-form">
                        <div className="form-field"><label htmlFor="sync-server">服务器地址</label><input id="sync-server" value={login.serverUrl} onChange={(event) => setLogin({...login, serverUrl: event.target.value})} placeholder="https://todo.example.com"/></div>
                        <div className="sync-login-row"><div className="form-field"><label htmlFor="sync-username">用户名</label><input id="sync-username" autoComplete="username" value={login.username} onChange={(event) => setLogin({...login, username: event.target.value})}/></div><div className="form-field"><label htmlFor="sync-password">密码</label><input id="sync-password" type="password" autoComplete="current-password" value={login.password} onChange={(event) => setLogin({...login, password: event.target.value})}/></div></div>
                        <label className="sync-migration-confirm"><input type="checkbox" checked={login.migrateLocal} onChange={(event) => setLogin({...login, migrateLocal: event.target.checked})}/><span>迁移当前本地数据，并在迁移前创建完整本地档案</span></label>
                        <button className="secondary-button sync-login-button" disabled={busy || !login.serverUrl.trim() || !login.username.trim() || login.password.length < 10 || !login.migrateLocal} onClick={() => void onLogin(login).then(() => setLogin({...login, password: '', migrateLocal: false}))}><LogIn size={15}/>连接服务器</button>
                    </div>}
                </section>
                <section className="settings-section backup-section"><div><h3>数据备份</h3><p>备份包含任务、笔记、历史版本和本地附件。</p></div><div className="backup-actions"><button className="secondary-button" onClick={() => void onExport()} disabled={busy}><Download size={16}/>导出备份</button><button className="secondary-button" onClick={() => void onImport()} disabled={busy || sync.configured}><Upload size={16}/>恢复备份</button></div><span className="backup-note"><DatabaseBackup size={15}/>{sync.configured ? '退出同步账号后才能恢复本地备份' : '恢复前会自动保存当前数据'}</span></section>
                <div className="dialog-buttons"><button className="secondary-button" onClick={onClose}>取消</button><button className="primary-button" disabled={busy} onClick={() => void onSave(draft)}>{busy ? '保存中' : '保存设置'}</button></div>
            </Dialog.Content></Dialog.Portal>
        </Dialog.Root>
    )
}
