import * as Popover from '@radix-ui/react-popover'
import {CalendarDays, X} from 'lucide-react'
import {DayPicker} from 'react-day-picker'
import {zhCN} from 'react-day-picker/locale'
import {format} from 'date-fns'
import {IconButton} from './IconButton'

export function DatePickerField({value, onChange}: {value: string; onChange: (value: string) => void}) {
    const selected = value ? new Date(`${value}T00:00:00`) : undefined
    const display = selected ? new Intl.DateTimeFormat('zh-CN', {month: 'long', day: 'numeric', weekday: 'short'}).format(selected) : '选择日期'
    return (
        <div className="date-field">
            <Popover.Root>
                <Popover.Trigger asChild>
                    <button className="field-button" type="button" aria-label="选择截止日期" data-date-picker-trigger><CalendarDays size={17}/><span>{display}</span></button>
                </Popover.Trigger>
                <Popover.Portal>
                    <Popover.Content className="calendar-popover" sideOffset={8} align="start">
                        <DayPicker
                            mode="single"
                            locale={zhCN}
                            selected={selected}
                            onSelect={(date) => onChange(date ? format(date, 'yyyy-MM-dd') : '')}
                        />
                        <Popover.Arrow className="popover-arrow"/>
                    </Popover.Content>
                </Popover.Portal>
            </Popover.Root>
            {value && <IconButton label="清除日期" type="button" onClick={() => onChange('')}><X size={15}/></IconButton>}
        </div>
    )
}
