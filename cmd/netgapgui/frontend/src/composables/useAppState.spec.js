import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAppState } from './useAppState'

const { backend, addNotification } = vi.hoisted(() => ({
  backend: {
    getInitialState: vi.fn(),
    saveSettings: vi.fn(),
    addRegion: vi.fn(),
    removeRegion: vi.fn(),
    startMonitoring: vi.fn(),
    stopMonitoring: vi.fn(),
    hideToTray: vi.fn(),
    quitApp: vi.fn(),
    onState: vi.fn(),
    onHealth: vi.fn(),
    onLogs: vi.fn(),
    onAlert: vi.fn(),
  },
  addNotification: vi.fn(),
}))

vi.mock('../services/backend', () => ({ backend }))
vi.mock('./useNotification', () => ({ useNotification: () => ({ addNotification }) }))

let app
let wrapper
let initialState
let unsubscribe
const Harness = defineComponent({
  setup() {
    app = useAppState()
    return () => h('div')
  },
})

describe('useAppState', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    // The composable intentionally shares one state object across consumers.
    // Reset its health generation so cases do not reject each other's fixture.
    if (app) {
      app.state.health = { runId: 0, revision: 0, running: false, phase: 'stopped', disconnectState: 'idle' }
      app.state.running = false
      app.state.errorCount = 0
    }
    unsubscribe = {}
    for (const name of ['onState', 'onHealth', 'onLogs', 'onAlert']) {
      unsubscribe[name] = vi.fn()
      backend[name].mockReturnValue(unsubscribe[name])
    }
    initialState = {
      language: 'en',
      interval: 5,
      closeAction: 'tray',
      removeRouteOnMatch: true,
      notificationPopup: true,
      regions: ['alpha'],
      logs: ['ready'],
      running: false,
      health: { runId: 1, revision: 1, running: false, phase: 'stopped', disconnectState: 'idle' },
    }
    backend.getInitialState.mockResolvedValue(initialState)
    backend.saveSettings.mockResolvedValue({
      closeAction: 'quit',
      removeRouteOnMatch: false,
      notificationPopup: false,
    })
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
  })

  it('keeps session error counts after recovery, clearing logs and late state updates', async () => {
    wrapper = mount(Harness)
    await flushPromises()
    const onLogs = backend.onLogs.mock.calls[0][0]
    onLogs(['operation failed'], 3)
    expect(app.state.errorCount).toBe(3)

    backend.onHealth.mock.calls[0][0]({ runId: 1, revision: 2, running: true, phase: 'healthy', consecutiveFailures: 0 })
    expect(app.state.errorCount).toBe(3)

    onLogs([], 3)
    expect(app.state.logs).toEqual([])
    expect(app.state.errorCount).toBe(3)

    backend.onState.mock.calls[0][0]({ errorCount: 1 })
    onLogs(['late log snapshot'], 2)
    expect(app.state.errorCount).toBe(3)
    onLogs(['next failure'], 4)
    expect(app.state.errorCount).toBe(4)
  })

  it('loads initial state and subscribes to live state and logs', async () => {
    wrapper = mount(Harness)
    await flushPromises()

    expect(backend.getInitialState).toHaveBeenCalledOnce()
    expect(app.state.language).toBe('en')
    expect(app.state.regions).toEqual(['alpha'])
    expect(app.state.logs).toEqual(['ready'])
    expect(backend.onState).toHaveBeenCalledOnce()
    expect(backend.onHealth).toHaveBeenCalledOnce()
    expect(backend.onLogs).toHaveBeenCalledOnce()
    expect(backend.onAlert).toHaveBeenCalledOnce()

    backend.onState.mock.calls[0][0]({ running: true })
    backend.onLogs.mock.calls[0][0](['ready', 'connected'])
    expect(app.state.running).toBe(true)
    expect(app.state.logs).toEqual(['ready', 'connected'])
  })

  it('saves close and route preferences and merges the backend result', async () => {
    wrapper = mount(Harness)
    await flushPromises()

    await app.saveSettings({
      closeAction: 'quit',
      removeRouteOnMatch: false,
      notificationPopup: false,
    })

    expect(backend.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      language: 'en',
      closeAction: 'quit',
      removeRouteOnMatch: false,
      notificationPopup: false,
    }))
    expect(app.state.closeAction).toBe('quit')
    expect(app.state.removeRouteOnMatch).toBe(false)
    expect(app.state.notificationPopup).toBe(false)
  })

  it('routes tray and quit actions to their backend methods', async () => {
    wrapper = mount(Harness)
    await flushPromises()
    await app.hideToTray()
    await app.quitApp()
    expect(backend.hideToTray).toHaveBeenCalledOnce()
    expect(backend.quitApp).toHaveBeenCalledOnce()
  })

  it('updates health while rejecting older runs and revisions', async () => {
    wrapper = mount(Harness)
    await flushPromises()
    const onHealth = backend.onHealth.mock.calls[0][0]
    const current = { runId: 2, revision: 3, running: true, phase: 'checking', consecutiveFailures: 2 }
    onHealth(current)
    expect(app.state.health).toEqual(current)
    expect(app.state.running).toBe(true)

    onHealth({ runId: 1, revision: 99, running: false })
    onHealth({ runId: 2, revision: 2, running: false })
    onHealth(null)
    expect(app.state.health).toEqual(current)
    expect(app.state.running).toBe(true)

    onHealth({ runId: 2, revision: 4, running: false, phase: 'stopped' })
    expect(app.state.running).toBe(false)
    onHealth({ runId: 3, revision: 1, running: true, phase: 'waiting' })
    expect(app.state.health.runId).toBe(3)
    expect(app.state.running).toBe(true)
  })

  it('preserves newer health when initial state arrives after a live event', async () => {
    let resolveInitial
    backend.getInitialState.mockReturnValue(new Promise((resolve) => { resolveInitial = resolve }))
    wrapper = mount(Harness)
    // Subscriptions must exist even while the initial request is unresolved.
    const latest = { runId: 2, revision: 5, running: true, phase: 'checking' }
    backend.onHealth.mock.calls[0][0](latest)
    resolveInitial(initialState)
    await flushPromises()
    expect(app.state.language).toBe('en')
    expect(app.state.health).toEqual(latest)
    expect(app.state.running).toBe(true)

    backend.onState.mock.calls[0][0]({ running: false, health: initialState.health })
    expect(app.state.health).toEqual(latest)
    expect(app.state.running).toBe(true)
  })

  it('removes every event subscription on unmount', async () => {
    wrapper = mount(Harness)
    await flushPromises()
    for (const remove of Object.values(unsubscribe)) expect(remove).not.toHaveBeenCalled()
    wrapper.unmount()
    wrapper = undefined
    for (const remove of Object.values(unsubscribe)) expect(remove).toHaveBeenCalledOnce()
  })
})
