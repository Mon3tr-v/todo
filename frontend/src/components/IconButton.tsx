import {forwardRef, type ButtonHTMLAttributes} from 'react'
import * as Tooltip from '@radix-ui/react-tooltip'

interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
    label: string
    side?: 'top' | 'right' | 'bottom' | 'left'
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
    ({label, side = 'top', className = '', children, ...props}, ref) => (
        <Tooltip.Root delayDuration={450}>
            <Tooltip.Trigger asChild>
                <button ref={ref} className={`icon-button ${className}`} aria-label={label} {...props}>
                    {children}
                </button>
            </Tooltip.Trigger>
            <Tooltip.Portal>
                <Tooltip.Content className="tooltip" side={side} sideOffset={7}>
                    {label}
                    <Tooltip.Arrow className="tooltip-arrow"/>
                </Tooltip.Content>
            </Tooltip.Portal>
        </Tooltip.Root>
    ),
)

IconButton.displayName = 'IconButton'
