<script setup>
import { ref, watch } from 'vue'
import TitleBar from './components/TitleBar.vue'
import SettingsDialog from './components/SettingsDialog.vue'
import ConfigPanel from './components/ConfigPanel.vue'
import ActionPanel from './components/ActionPanel.vue'
import LogPanel from './components/LogPanel.vue'
import StatusBar from './components/StatusBar.vue'
import NotificationToast from './components/NotificationToast.vue'
import UpdateReadyDialog from './components/UpdateReadyDialog.vue'
import { useAppState } from './composables/useAppState'
import { useNotification } from './composables/useNotification'
import { backend } from './services/backend'

const appVersion = import.meta.env.VITE_APP_VERSION || 'dev'

const {
  state,
  ready,
  t,
  addRegion,
  removeRegion,
  saveSettings,
  startMonitoring,
  stopMonitoring,
  toggleLanguage,
  hideToTray,
  quitApp,
} = useAppState()

const deletedRegion = ref(null)
const regionBusy = ref(false)

async function deleteRegion(region) {
  if (regionBusy.value) return
  const index = state.regions.indexOf(region)
  if (index < 0) return
  regionBusy.value = true
  try {
    await removeRegion(region)
    deletedRegion.value = { region, index }
  } catch (error) {
    useNotification().addNotification(`${t('regionDeleteFailed')}: ${error}`, 'error')
  } finally {
    regionBusy.value = false
  }
}

async function undoRegionDeletion() {
  if (regionBusy.value || !deletedRegion.value) return
  const { region, index } = deletedRegion.value
  regionBusy.value = true
  try {
    const regions = [...state.regions]
    if (!regions.includes(region)) {
      regions.splice(Math.min(index, regions.length), 0, region)
      await saveSettings({ regions })
    }
    deletedRegion.value = null
  } catch (error) {
    useNotification().addNotification(`${t('regionUndoFailed')}: ${error}`, 'error')
  } finally {
    regionBusy.value = false
  }
}

async function addRegionEntry(region) {
  if (regionBusy.value) return
  regionBusy.value = true
  try {
    await addRegion(region)
  } catch (error) {
    useNotification().addNotification(`${t('regionAddFailed')}: ${error}`, 'error')
  } finally {
    regionBusy.value = false
  }
}
const settingsVisible = ref(false)
const updatePhase = ref('idle')
const updateVersion = ref('')
const updateError = ref('')
const updateBusy = ref(false)
const restartPrompt = ref(false)
let startupUpdateCheckStarted = false

async function downloadAvailableUpdate(revealWindow = false) {
  updatePhase.value = 'downloading'
  updateError.value = ''
  const result = await backend.downloadUpdate()
  updateVersion.value = result.latestVersion || updateVersion.value
  updatePhase.value = 'downloaded'
  restartPrompt.value = true
  if (revealWindow) await backend.showWindow()
}

async function runUpdateCheck(autoDownload) {
  if (updateBusy.value) return
  updateBusy.value = true
  updatePhase.value = 'checking'
  updateError.value = ''
  try {
    const result = await backend.checkForUpdates(appVersion)
    updateVersion.value = result.latestVersion || ''
    if (!result.available) {
      updatePhase.value = 'current'
      return
    }
    updatePhase.value = 'available'
    if (autoDownload) await downloadAvailableUpdate(true)
  } catch (error) {
    updateError.value = String(error)
    updatePhase.value = 'error'
  } finally {
    updateBusy.value = false
  }
}

async function checkForUpdates() {
  await runUpdateCheck(Boolean(state.autoCheckUpdates))
}

async function downloadUpdate() {
  if (updateBusy.value) return
  updateBusy.value = true
  try {
    await downloadAvailableUpdate()
  } catch (error) {
    updateError.value = String(error)
    updatePhase.value = 'error'
  } finally {
    updateBusy.value = false
  }
}

async function restartToUpdate() {
  if (updateBusy.value) return
  updateBusy.value = true
  updatePhase.value = 'restarting'
  try {
    await backend.installUpdate()
  } catch (error) {
    updateBusy.value = false
    updatePhase.value = 'downloaded'
    useNotification().addNotification(`${t('updateFailed')}: ${error}`, 'error')
  }
}

watch(ready, async (initialized) => {
  if (!initialized || startupUpdateCheckStarted) return
  startupUpdateCheckStarted = true
  try {
    const pending = await backend.getPendingUpdate(appVersion)
    if (pending?.available && pending.downloaded) {
      updateVersion.value = pending.latestVersion || ''
      updatePhase.value = 'downloaded'
      restartPrompt.value = true
      await backend.showWindow()
      return
    }
    if (state.autoCheckUpdates) await runUpdateCheck(true)
  } catch (error) {
    updateError.value = String(error)
    updatePhase.value = 'error'
  }
}, { immediate: true })

const closeWindow = async () => {
  if (state.closeAction === 'quit') {
    await quitApp()
    return
  }
  await hideToTray()
}

const savePreferences = async (preferences) => {
  try {
    await saveSettings(preferences)
    settingsVisible.value = false
  } catch (error) {
    useNotification().addNotification(`${t('settingsSaveFailed')}: ${error}`, 'error')
  }
}
</script>

<template>
  <div class="shell">
    <TitleBar
      :language="state.language"
      :close-action="state.closeAction"
      :is-admin="state.isAdmin"
      :translate="t"
      @toggle-language="toggleLanguage"
      @open-settings="settingsVisible = true"
      @close-window="closeWindow"
      @hide-to-tray="hideToTray"
    />
    <SettingsDialog
      :visible="settingsVisible"
      :language="state.language"
      :close-action="state.closeAction"
      :remove-route-on-match="state.removeRouteOnMatch"
      :notification-popup="state.notificationPopup"
      :auto-start-monitoring="state.autoStartMonitoring ?? false"
      :start-minimized="state.startMinimized ?? false"
      :auto-check-updates="state.autoCheckUpdates ?? false"
      :update-phase="updatePhase"
      :update-version="updateVersion"
      :update-error="updateError"
      :translate="t"
      @cancel="settingsVisible = false"
      @save="savePreferences"
      @check-updates="checkForUpdates"
      @download-update="downloadUpdate"
      @restart-update="restartToUpdate"
    />
    <ConfigPanel
      v-model:interval="state.interval"
      v-model:keyword-input="state.keywordInput"
      v-model:api-enabled="state.apiEnabled"
      v-model:custom-api-url="state.customApiUrl"
      :regions="state.regions"
      :deleted-region="deletedRegion?.region ?? ''"
      :region-busy="regionBusy"
      :translate="t"
      @add-region="addRegionEntry"
      @remove-region="deleteRegion"
      @undo-region="undoRegionDeletion"
      @save-settings="saveSettings"
    />
    <ActionPanel
      :translate="t"
      :start-action="startMonitoring"
      :stop-action="stopMonitoring"
    />
    <LogPanel :logs="state.logs" :translate="t" />
    <StatusBar
      :is-admin="state.isAdmin"
      :running="state.running"
      :health="state.health"
      :interval="state.interval"
      :alert-unread="state.alertUnread"
      :error-count="state.errorCount"
      :translate="t"
    />
    <NotificationToast />
    <UpdateReadyDialog
      :visible="restartPrompt"
      :version="updateVersion"
      :translate="t"
      @later="restartPrompt = false"
      @restart="restartToUpdate"
    />
  </div>
</template>
