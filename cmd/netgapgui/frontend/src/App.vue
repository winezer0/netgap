<script setup>
import { ref } from 'vue'
import TitleBar from './components/TitleBar.vue'
import SettingsDialog from './components/SettingsDialog.vue'
import ConfigPanel from './components/ConfigPanel.vue'
import ActionPanel from './components/ActionPanel.vue'
import LogPanel from './components/LogPanel.vue'
import StatusBar from './components/StatusBar.vue'
import NotificationToast from './components/NotificationToast.vue'
import { useAppState } from './composables/useAppState'
import { useNotification } from './composables/useNotification'

const {
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
      :translate="t"
      @cancel="settingsVisible = false"
      @save="savePreferences"
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
  </div>
</template>
