/// <reference types="vite/client" />

import type {AppBridge} from './types'

declare global {
    interface Window {
        go?: {backend?: {App?: AppBridge}}
    }
}
