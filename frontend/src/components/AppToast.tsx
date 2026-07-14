import {AnimatePresence, motion, useReducedMotion} from 'motion/react'
import {CheckCircle2, Info, X} from 'lucide-react'
import {IconButton} from './IconButton'

export interface ToastState {id: number; message: string; tone?: 'success' | 'info'; actionLabel?: string; onAction?: () => void}

export function AppToast({toast, onClose}: {toast: ToastState | null; onClose: () => void}) {
    const reduce = useReducedMotion()
    return (
        <div className="toast-viewport">
            <AnimatePresence>
                {toast && <motion.div className="app-toast" role="status" initial={reduce ? false : {opacity: 0, y: -14, scale: 0.97}} animate={{opacity: 1, y: 0, scale: 1}} exit={reduce ? {opacity: 0} : {opacity: 0, y: -8, scale: 0.98}} transition={reduce ? {duration: 0} : {type: 'spring', stiffness: 420, damping: 34}}>
                    {toast.tone === 'info' ? <Info size={18}/> : <CheckCircle2 size={18}/>}<span>{toast.message}</span>
                    {toast.actionLabel && <button className="toast-action" onClick={toast.onAction}>{toast.actionLabel}</button>}
                    <IconButton label="关闭提示" onClick={onClose}><X size={15}/></IconButton>
                </motion.div>}
            </AnimatePresence>
        </div>
    )
}
