<script setup>
import { computed, ref } from 'vue'
import TitleBar from './components/TitleBar.vue'
import SettingsDialog from './components/SettingsDialog.vue'
import ConfigPanel from './components/ConfigPanel.vue'
import ActionPanel from './components/ActionPanel.vue'
import LogPanel from './components/LogPanel.vue'
import StatusBar from './components/StatusBar.vue'
import NotificationToast from './components/NotificationToast.vue'
import { useAppState } from './composables/useAppState'

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

const regionText = computed(() => state.regions.join('\n'))
const settingsVisible = ref(false)

const closeWindow = async () => {
  if (state.closeAction === 'quit') {
    await quitApp()
    return
  }
  await hideToTray()
}

const savePreferences = async (preferences) => {
  await saveSettings(preferences)
  settingsVisible.value = false
}
</script>

<template>
  <div class="shell">
    <TitleBar
      :language="state.language"
      :close-action="state.closeAction"
      @toggle-language="toggleLanguage"
      @open-settings="settingsVisible = true"
      @close-window="closeWindow"
    />
    <SettingsDialog
      :visible="settingsVisible"
      :language="state.language"
      :close-action="state.closeAction"
      :remove-route-on-match="state.removeRouteOnMatch"
      :translate="t"
      @cancel="settingsVisible = false"
      @save="savePreferences"
    />
    <ConfigPanel
      v-model:interval="state.interval"
      v-model:keyword-input="state.keywordInput"
      v-model:api-enabled="state.apiEnabled"
      v-model:custom-api-url="state.customApiUrl"
      :all-regions="regionText"
      :translate="t"
      @add-region="addRegion"
      @remove-region="removeRegion"
      @save-settings="saveSettings"
    />
    <ActionPanel
      :translate="t"
      @start-monitoring="startMonitoring"
      @stop-monitoring="stopMonitoring"
    />
    <LogPanel :logs="state.logs" :translate="t" />
    <StatusBar
      :is-admin="state.isAdmin"
      :running="state.running"
      :translate="t"
    />
    <NotificationToast />
  </div>
</template>
