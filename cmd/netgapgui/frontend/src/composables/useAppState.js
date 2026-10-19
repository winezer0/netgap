import { onMounted, reactive } from 'vue'
import { backend } from '../services/backend'
import { translations } from '../services/translations'
import { useNotification } from './useNotification'

const state = reactive({
  language: 'zh',
  interval: 3,
  keywordInput: '',
  regions: [],
  apiEnabled: [],
  customApiUrl: '',
  logs: [],
  isAdmin: false,
  running: false,
})

function mergeState(payload) {
  if (!payload) {
    return
  }
  state.language = payload.language ?? state.language
  state.interval = payload.interval ?? state.interval
  state.keywordInput = payload.keywordInput ?? state.keywordInput
  state.regions = payload.regions ?? state.regions
  state.apiEnabled = payload.apiEnabled ?? state.apiEnabled
  state.customApiUrl = payload.customApiUrl ?? state.customApiUrl
  state.logs = payload.logs ?? state.logs
  state.isAdmin = payload.isAdmin ?? state.isAdmin
  state.running = payload.running ?? state.running
}

function t(key) {
  const table = translations[state.language] ?? translations.zh
  return table[key] ?? key
}

async function saveSettings() {
  mergeState(await backend.saveSettings({
    language: state.language,
    interval: state.interval,
    keywordInput: state.keywordInput,
    regions: state.regions,
    apiEnabled: state.apiEnabled,
    customApiUrl: state.customApiUrl,
  }))
}

async function addRegion(keyword) {
  mergeState(await backend.addRegion(keyword))
}

async function removeRegion(region) {
  mergeState(await backend.removeRegion(region))
}

async function startMonitoring() {
  mergeState(await backend.startMonitoring())
}

async function stopMonitoring() {
  mergeState(await backend.stopMonitoring())
}

async function toggleLanguage() {
  state.language = state.language === 'zh' ? 'en' : 'zh'
  await saveSettings()
}

async function minimiseWindow() {
  await backend.minimiseWindow()
}

async function hideToTray() {
  await backend.hideToTray()
}

async function quitApp() {
  await backend.quitApp()
}

export function useAppState() {
  onMounted(async () => {
    const { addNotification } = useNotification()
    mergeState(await backend.getInitialState())
    backend.onState((payload) => mergeState(payload))
    backend.onLogs((payload) => {
      state.logs = payload ?? []
    })
    backend.onAlert((payload) => {
      const message = payload?.message ?? ''
      if (message) {
        addNotification(message, payload?.type ?? 'error', 3000)
      }
    })
  })

  return {
    state,
    t,
    addRegion,
    removeRegion,
    saveSettings,
    startMonitoring,
    stopMonitoring,
    toggleLanguage,
    minimiseWindow,
    hideToTray,
    quitApp,
  }
}
