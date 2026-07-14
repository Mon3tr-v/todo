import * as Dialog from '@radix-ui/react-dialog'
import {Bell, Check, MessageSquare, X} from 'lucide-react'
import type {AppNotification} from '../types'
import {IconButton} from './IconButton'

function notificationTime(value: string) {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return ''
    const elapsed = Date.now() - date.getTime()
    if (elapsed < 60_000) return '刚刚'
    if (elapsed < 3_600_000) return `${Math.floor(elapsed / 60_000)} 分钟前`
    if (elapsed < 86_400_000) return `${Math.floor(elapsed / 3_600_000)} 小时前`
    return new Intl.DateTimeFormat('zh-CN', {month: 'short', day: 'numeric'}).format(date)
}

export function NotificationCenter({open, items, loading, error, onClose, onOpen}: {
    open: boolean
    items: AppNotification[]
    loading: boolean
    error: Error | null
    onClose: () => void
    onOpen: (item: AppNotification) => Promise<void>
}) {
    return (
        <Dialog.Root open={open} onOpenChange={(next) => {if (!next) onClose()}}>
            <Dialog.Portal>
                <Dialog.Overlay className="notification-overlay"/>
                <Dialog.Content className="notification-drawer" onOpenAutoFocus={(event) => event.preventDefault()}>
                    <header>
                        <div><Dialog.Title>通知</Dialog.Title><Dialog.Description>协作提及和笔记动态</Dialog.Description></div>
                        <Dialog.Close asChild><IconButton label="关闭"><X size={18}/></IconButton></Dialog.Close>
                    </header>
                    <div className="notification-list">
                        {loading && <div className="notification-skeleton" aria-label="正在加载通知"><span/><span/><span/></div>}
                        {!loading && error && <div className="notification-state"><Bell size={22}/><strong>无法读取通知</strong><p>{error.message}</p></div>}
                        {!loading && !error && items.length === 0 && <div className="notification-state"><Bell size={22}/><strong>暂无通知</strong><p>新的协作提及会显示在这里。</p></div>}
                        {!loading && !error && items.map((item) => (
                            <button className={`notification-item ${item.readAt ? '' : 'is-unread'}`} key={item.id} onClick={() => void onOpen(item)}>
                                <span className="notification-icon">{item.kind === 'mention' ? <MessageSquare size={16}/> : <Bell size={16}/>}</span>
                                <span className="notification-copy"><strong>{item.message}</strong><small>{notificationTime(item.createdAt)}</small></span>
                                {item.readAt ? <Check className="notification-check" size={14}/> : <span className="notification-unread" aria-label="未读"/>}
                            </button>
                        ))}
                    </div>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    )
}
