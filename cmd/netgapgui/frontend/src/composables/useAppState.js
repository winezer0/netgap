import { onMounted, onUnmounted, reactive } from 'vue'
import { backend } from '../services/backend'
import { translations } from '../services/translations'
import { useNotification } from './useNotification'

let windowReportRevision = Date.now() * 1000

const state = reactive({
  language: 'zh',
  interval: 3,
  keywordInput: '',
  regions: [],
  apiEnabled: [],
  customApiUrl: '',
  closeAction: 'tray',
  removeRouteOnMatch: true,
  notificationPopup: true,
  logs: [],
  errorCount: 0,
  isAdmin: false,
  running: false,
  latestAlert: null,
  alertUnread: false,
  alertRevision: 0,
  health: { runId: 0, revision: 0, running: false, phase: 'stopped', disconnectState: 'idle' },
})

function mergeHealth(payload) {
  if (!payload) return
  const current = state.health
  if (payload.runId < current.runId ||
      (payload.runId === current.runId && payload.revision < current.revision)) return
  state.health = payload
  state.running = payload.running
  useNotification().removeStaleMatchNotifications?.(payload.runId, payload.running || payload.disconnectState === 'completed')
}

function mergeAlertState(payload) {
  if (!payload) return
  if ((payload.alertRevision || 0) < state.alertRevision) return
  if ((payload.latestAlert?.eventId || 0) < (state.latestAlert?.eventId || 0)) return
  state.alertRevision = payload.alertRevision || 0
  state.latestAlert = payload.latestAlert ?? null
  state.alertUnread = payload.alertUnread ?? false
}

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
  state.closeAction = payload.closeAction ?? state.closeAction
  state.removeRouteOnMatch = payload.removeRouteOnMatch ?? state.removeRouteOnMatch
  state.notificationPopup = payload.notificationPopup ?? state.notificationPopup
  state.logs = payload.logs ?? state.logs
  state.errorCount = Math.max(state.errorCount, payload.errorCount ?? 0)
  state.isAdmin = payload.isAdmin ?? state.isAdmin
  if ('latestAlert' in payload) mergeAlertState(payload)
  if (payload.health) mergeHealth(payload.health)
  else state.running = payload.running ?? state.running
}

function t(key) {
  const table = translations[state.language] ?? translations.zh
  return table[key] ?? key
}

async function saveSettings(overrides = {}) {
  mergeState(await backend.saveSettings({
    language: state.language,
    interval: state.interval,
    keywordInput: state.keywordInput,
    regions: state.regions,
    apiEnabled: state.apiEnabled,
    customApiUrl: state.customApiUrl,
    closeAction: overrides.closeAction ?? state.closeAction,
    removeRouteOnMatch: overrides.removeRouteOnMatch ?? state.removeRouteOnMatch,
    notificationPopup: overrides.notificationPopup ?? state.notificationPopup,
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

async function hideToTray() {
  await backend.hideToTray()
}

async function quitApp() {
  await backend.quitApp()
}

export function useAppState() {
  const listeners = []
  onMounted(async () => {
    const { addNotification } = useNotification()
    listeners.push(backend.onState((payload) => mergeState(payload)))
    listeners.push(backend.onHealth((payload) => mergeHealth(payload)))
    listeners.push(backend.onLogs((payload, errorCount) => {
      state.logs = payload ?? []
      state.errorCount = Math.max(state.errorCount, errorCount ?? 0)
    }))
    listeners.push(backend.onAlert((payload) => {
      if (!state.notificationPopup && !payload?.historical) return
      if (payload?.runId && !payload.historical &&
          (payload.runId < state.health.runId ||
           (payload.runId === state.health.runId && !state.running && state.health.disconnectState !== 'completed'))) return
      const message = payload?.message ?? ''
      if (message) {
        addNotification(message, payload?.type ?? 'error', 6000,
          payload.historical ? { eventId: `history-${payload.eventId}` } : { runId: payload.runId, eventId: payload.eventId })
      }
    }))
    listeners.push(backend.onAlertState?.(mergeAlertState))
    const reportForeground = () => {
      const request = backend.setWindowForeground?.(document.visibilityState === 'visible' && document.hasFocus(), ++windowReportRevision)
      request?.catch(() => {})
    }
    window.addEventListener('focus', reportForeground)
    window.addEventListener('blur', reportForeground)
    document.addEventListener('visibilitychange', reportForeground)
    listeners.push(() => {
      window.removeEventListener('focus', reportForeground)
      window.removeEventListener('blur', reportForeground)
      document.removeEventListener('visibilitychange', reportForeground)
      backend.setWindowForeground?.(false, ++windowReportRevision)?.catch(() => {})
    })
    mergeState(await backend.getInitialState())
    reportForeground()
  })
  onUnmounted(() => {
    for (const remove of listeners) {
      if (typeof remove === 'function') remove()
    }
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
    hideToTray,
    quitApp,
  }
}
