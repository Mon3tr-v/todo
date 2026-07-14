import * as Select from '@radix-ui/react-select'
import {Check, ChevronDown} from 'lucide-react'

const emptyValue = '__todo_empty_select_value__'

export interface SelectOption<T extends string> {
    value: T
    label: string
    disabled?: boolean
}

interface CustomSelectProps<T extends string> {
    value: T
    options: Array<SelectOption<T>>
    onValueChange: (value: T) => void
    ariaLabel: string
    id?: string
    className?: string
}

export function CustomSelect<T extends string>({value, options, onValueChange, ariaLabel, id, className = ''}: CustomSelectProps<T>) {
    return (
        <Select.Root value={value || emptyValue} onValueChange={(next) => onValueChange((next === emptyValue ? '' : next) as T)}>
            <Select.Trigger id={id} className={`select-trigger ${className}`} aria-label={ariaLabel}>
                <Select.Value/>
                <Select.Icon className="select-chevron"><ChevronDown size={14}/></Select.Icon>
            </Select.Trigger>
            <Select.Portal>
                <Select.Content className="select-content" position="popper" sideOffset={5} collisionPadding={10}>
                    <Select.Viewport className="select-viewport">
                        {options.map((option) => (
                            <Select.Item className="select-item" value={option.value || emptyValue} disabled={option.disabled} key={option.value || emptyValue}>
                                <Select.ItemText>{option.label}</Select.ItemText>
                                <Select.ItemIndicator className="select-indicator"><Check size={14}/></Select.ItemIndicator>
                            </Select.Item>
                        ))}
                    </Select.Viewport>
                </Select.Content>
            </Select.Portal>
        </Select.Root>
    )
}
