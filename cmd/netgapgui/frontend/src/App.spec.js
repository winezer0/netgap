import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import TitleBar from './components/TitleBar.vue'
import SettingsDialog from './components/SettingsDialog.vue'

const { appState, mocks } = vi.hoisted(() => ({
  appState: { current: null },
  mocks: {
    saveSettings: vi.fn(),
    hideToTray: vi.fn(),
    quitApp: vi.fn(),
  },
}))

vi.mock('./composables/useAppState', () => ({ useAppState: () => appState.current }))

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
    appState.current = {
      state: makeState(),
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
})
