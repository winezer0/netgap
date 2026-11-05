import { flushPromises, mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import TitleBar from './components/TitleBar.vue'
import SettingsDialog from './components/SettingsDialog.vue'
import UpdateReadyDialog from './components/UpdateReadyDialog.vue'

const { appState, mocks, backend } = vi.hoisted(() => ({
  appState: { current: null },
  mocks: {
    saveSettings: vi.fn(),
    hideToTray: vi.fn(),
    quitApp: vi.fn(),
  },
  backend: {
    checkForUpdates: vi.fn(),
    downloadUpdate: vi.fn(),
    getPendingUpdate: vi.fn(),
    showWindow: vi.fn(),
    installUpdate: vi.fn(),
    getStartupStatus: vi.fn(),
    setStartupEnabled: vi.fn(),
  },
}))

vi.mock('./composables/useAppState', () => ({ useAppState: () => appState.current }))
vi.mock('./services/backend', () => ({ backend }))

const makeState = (closeAction = 'tray') => reactive({
  language: 'zh',
  interval: 3,
  keywordInput: '',
  regions: [],
  apiEnabled: [],
  customApiUrl: '',
  closeAction,
  removeRouteOnMatch: true,
  notificationPopup: true,
  autoCheckUpdates: false,
  logs: [],
  isAdmin: false,
  running: false,
})

const mountApp = () => mount(App, {
  global: {
    stubs: {
      ConfigPanel: true,
      ActionPanel: true,
      LogPanel: true,
      StatusBar: true,
      NotificationToast: true,
    },
  },
})

describe('App settings and close behavior', () => {
  beforeEach(() => {
    mocks.saveSettings.mockResolvedValue(undefined)
    mocks.hideToTray.mockResolvedValue(undefined)
    mocks.quitApp.mockResolvedValue(undefined)
    backend.checkForUpdates.mockReset().mockResolvedValue({ available: false })
    backend.downloadUpdate.mockReset().mockResolvedValue({ latestVersion: 'v1.1.0' })
    backend.getPendingUpdate.mockReset().mockResolvedValue({ available: false })
    backend.showWindow.mockReset().mockResolvedValue(undefined)
    backend.installUpdate.mockReset().mockResolvedValue(undefined)
    backend.getStartupStatus.mockReset().mockResolvedValue({ supported: false })
    backend.setStartupEnabled.mockReset().mockResolvedValue({ supported: false })
    appState.current = {
      state: makeState(),
      ready: ref(false),
      t: (key) => key,
      addRegion: vi.fn(),
      removeRegion: vi.fn(),
      saveSettings: mocks.saveSettings,
      startMonitoring: vi.fn(),
      stopMonitoring: vi.fn(),
      toggleLanguage: vi.fn(),
      hideToTray: mocks.hideToTray,
      quitApp: mocks.quitApp,
    }
  })

  it('opens settings, saves the preferences, and closes the dialog', async () => {
    const wrapper = mountApp()
    await wrapper.getComponent(TitleBar).vm.$emit('open-settings')
    expect(wrapper.getComponent(SettingsDialog).props('visible')).toBe(true)

    const preferences = {
      closeAction: 'quit',
      removeRouteOnMatch: false,
      notificationPopup: false,
    }
    await wrapper.getComponent(SettingsDialog).vm.$emit('save', preferences)
    await flushPromises()

    expect(mocks.saveSettings).toHaveBeenCalledWith(preferences)
    expect(wrapper.getComponent(SettingsDialog).props('visible')).toBe(false)
  })

  it('uses the configured close action', async () => {
    const wrapper = mountApp()
    await wrapper.getComponent(TitleBar).vm.$emit('close-window')
    expect(mocks.hideToTray).toHaveBeenCalledOnce()
    expect(mocks.quitApp).not.toHaveBeenCalled()

    appState.current.state.closeAction = 'quit'
    await wrapper.getComponent(TitleBar).vm.$emit('close-window')
    await flushPromises()
    expect(mocks.quitApp).toHaveBeenCalledOnce()
    expect(mocks.hideToTray).toHaveBeenCalledOnce()
  })

  it('automatically downloads an available update and prompts for restart', async () => {
    appState.current.state.autoCheckUpdates = true
    backend.checkForUpdates.mockResolvedValue({ available: true, latestVersion: 'v1.1.0' })
    const wrapper = mountApp()
    appState.current.ready.value = true
    await flushPromises()

    expect(backend.getPendingUpdate).toHaveBeenCalledOnce()
    expect(backend.checkForUpdates).toHaveBeenCalledOnce()
    expect(backend.downloadUpdate).toHaveBeenCalledOnce()
    expect(backend.showWindow).toHaveBeenCalledOnce()
    expect(wrapper.getComponent(UpdateReadyDialog).props('visible')).toBe(true)
  })

  it('does not download automatically after a manual check when auto-check is disabled', async () => {
    backend.checkForUpdates.mockResolvedValue({ available: true, latestVersion: 'v1.1.0' })
    const wrapper = mountApp()
    await wrapper.getComponent(SettingsDialog).vm.$emit('check-updates')
    await flushPromises()

    expect(backend.checkForUpdates).toHaveBeenCalledOnce()
    expect(backend.downloadUpdate).not.toHaveBeenCalled()
    expect(wrapper.getComponent(SettingsDialog).props('updatePhase')).toBe('available')
  })
})
