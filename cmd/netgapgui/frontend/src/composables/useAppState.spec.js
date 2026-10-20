import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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
    onLogs: vi.fn(),
    onAlert: vi.fn(),
  },
  addNotification: vi.fn(),
}))

vi.mock('../services/backend', () => ({ backend }))
vi.mock('./useNotification', () => ({ useNotification: () => ({ addNotification }) }))

let app
const Harness = defineComponent({
  setup() {
    app = useAppState()
    return () => h('div')
  },
})

describe('useAppState', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    backend.getInitialState.mockResolvedValue({
      language: 'en',
      interval: 5,
      closeAction: 'tray',
      removeRouteOnMatch: true,
      regions: ['alpha'],
      logs: ['ready'],
    })
    backend.saveSettings.mockResolvedValue({ closeAction: 'quit', removeRouteOnMatch: false })
  })

  it('loads initial state and subscribes to live state and logs', async () => {
    mount(Harness)
    await flushPromises()

    expect(backend.getInitialState).toHaveBeenCalledOnce()
    expect(app.state.language).toBe('en')
    expect(app.state.regions).toEqual(['alpha'])
    expect(app.state.logs).toEqual(['ready'])
    expect(backend.onState).toHaveBeenCalledOnce()
    expect(backend.onLogs).toHaveBeenCalledOnce()
    expect(backend.onAlert).toHaveBeenCalledOnce()

    backend.onState.mock.calls[0][0]({ running: true })
    backend.onLogs.mock.calls[0][0](['ready', 'connected'])
    expect(app.state.running).toBe(true)
    expect(app.state.logs).toEqual(['ready', 'connected'])
  })

  it('saves close and route preferences and merges the backend result', async () => {
    mount(Harness)
    await flushPromises()

    await app.saveSettings({ closeAction: 'quit', removeRouteOnMatch: false })

    expect(backend.saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      language: 'en',
      closeAction: 'quit',
      removeRouteOnMatch: false,
    }))
    expect(app.state.closeAction).toBe('quit')
    expect(app.state.removeRouteOnMatch).toBe(false)
  })

  it('routes tray and quit actions to their backend methods', async () => {
    mount(Harness)
    await app.hideToTray()
    await app.quitApp()
    expect(backend.hideToTray).toHaveBeenCalledOnce()
    expect(backend.quitApp).toHaveBeenCalledOnce()
  })
})
