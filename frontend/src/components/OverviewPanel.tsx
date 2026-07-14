import {motion, useReducedMotion} from 'motion/react'
import {ArrowUpRight, CheckCircle2, Clock3, ListChecks} from 'lucide-react'
import type {Overview} from '../types'

export function OverviewPanel({overview, isLoading, error, onOpenToday}: {overview?: Overview; isLoading: boolean; error: Error | null; onOpenToday: () => void}) {
    const reduce = useReducedMotion()
    if (isLoading) return <main className="overview-panel"><div className="overview-loading"><span/><span/><span/></div></main>
    if (error || !overview) return <main className="overview-panel"><div className="state-view error-state"><strong>无法读取概览</strong><p>{error?.message}</p></div></main>
    const max = Math.max(1, ...overview.week.map((day) => day.count))
    const weekTotal = overview.week.reduce((sum, day) => sum + day.count, 0)
    const formatter = new Intl.DateTimeFormat('zh-CN', {weekday: 'short'})
    return (
        <main className="overview-panel">
            <header className="overview-header"><p>今天也从一件小事开始</p><h1>概览</h1></header>
            <section className="focus-summary">
                <div className="focus-primary"><span>待完成</span><strong>{overview.open}</strong><button onClick={onOpenToday}>查看今天<ArrowUpRight size={16}/></button></div>
                <div className="focus-stat"><Clock3 size={19}/><span>已逾期<strong>{overview.overdue}</strong></span></div>
                <div className="focus-stat"><CheckCircle2 size={19}/><span>今日完成<strong>{overview.completedToday}</strong></span></div>
            </section>
            <section className="week-section">
                <div className="week-heading"><div><h2>最近 7 天</h2><p>完成节奏</p></div><span><ListChecks size={17}/>{weekTotal} 项</span></div>
                <div className="week-chart" role="img" aria-label={`最近 7 天共完成 ${weekTotal} 项任务`}>
                    {overview.week.map((day, index) => (
                        <div className="day-column" key={day.date}>
                            <span className="day-value">{day.count || ''}</span>
                            <div className="bar-space"><motion.i initial={reduce ? false : {scaleY: 0}} animate={{scaleY: day.count ? Math.max(0.12, day.count / max) : 0.05}} transition={reduce ? {duration: 0} : {type: 'spring', stiffness: 120, damping: 18, delay: index * 0.04}}/></div>
                            <small>{formatter.format(new Date(`${day.date}T00:00:00`))}</small>
                        </div>
                    ))}
                </div>
            </section>
        </main>
    )
}
