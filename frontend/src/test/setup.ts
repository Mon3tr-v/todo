import '@testing-library/jest-dom/vitest'

(globalThis as typeof globalThis & {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true

class ResizeObserverMock {
    observe() {}
    unobserve() {}
    disconnect() {}
}

globalThis.ResizeObserver = ResizeObserverMock

HTMLElement.prototype.hasPointerCapture = () => false
HTMLElement.prototype.setPointerCapture = () => undefined
HTMLElement.prototype.releasePointerCapture = () => undefined
HTMLElement.prototype.scrollIntoView = () => undefined

Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        addListener: () => undefined,
        removeListener: () => undefined,
        dispatchEvent: () => false,
    }),
})
