<script setup>
import { computed } from 'vue'
import TitleBar from './components/TitleBar.vue'
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
  minimiseWindow,
  hideToTray,
  quitApp,
} = useAppState()

const regionText = computed(() => state.regions.join('\n'))
</script>

<template>
  <div class="shell">
    <TitleBar
      :language="state.language"
      @toggle-language="toggleLanguage"
      @minimise-window="minimiseWindow"
      @hide-to-tray="hideToTray"
      @quit-app="quitApp"
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
