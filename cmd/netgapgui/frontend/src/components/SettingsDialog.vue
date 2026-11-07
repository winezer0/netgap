<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { backend } from '../services/backend'

const props = defineProps({
  visible: { type: Boolean, required: true },
  language: { type: String, required: true },
  closeAction: { type: String, required: true },
  removeRouteOnMatch: { type: Boolean, required: true },
  notificationPopup: { type: Boolean, required: true },
  autoStartMonitoring: { type: Boolean, default: false },
  startMinimized: { type: Boolean, default: false },
  autoCheckUpdates: { type: Boolean, default: false },
  updatePhase: { type: String, default: 'idle' },
  updateVersion: { type: String, default: '' },
  updateError: { type: String, default: '' },
  lastUpdateCheckAt: { type: String, default: '' },
  translate: { type: Function, required: true },
})

const emit = defineEmits(['cancel', 'save', 'check-updates', 'download-update'])
const draft = reactive({
  closeAction: 'tray',
  removeRouteOnMatch: true,
  notificationPopup: true,
  autoStartMonitoring: false,
  startMinimized: false,
  autoCheckUpdates: false,
})

const startup = ref(null)
const startupBusy = ref(false)
const startupError = ref('')
let startupRequest = 0

const lastUpdateCheckText = computed(() => {
  if (!props.lastUpdateCheckAt) return props.translate('updateNeverChecked')
  const checkedAt = new Date(props.lastUpdateCheckAt)
  if (Number.isNaN(checkedAt.getTime())) return props.translate('updateNeverChecked')
  const locale = props.language === 'en' ? 'en-US' : 'zh-CN'
  return new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'short' }).format(checkedAt)
})

async function loadStartup() {
  const request = ++startupRequest
  startupBusy.value = true
  startupError.value = ''
  startup.value = null
  try {
    const status = await backend.getStartupStatus()
    if (request === startupRequest) startup.value = status
  } catch (error) {
    if (request === startupRequest) startupError.value = String(error)
  } finally {
    if (request === startupRequest) startupBusy.value = false
  }
}

async function updateStartup() {
  if (startupBusy.value || !startup.value?.supported) return
  startupBusy.value = true
  startupError.value = ''
  const request = ++startupRequest
  try {
    const enabled = !startup.value.registered || Boolean(startup.value.needsRepair)
    const status = await backend.setStartupEnabled(enabled)
    if (request === startupRequest) startup.value = status
  } catch (error) {
    if (request === startupRequest) startupError.value = String(error)
  } finally {
    if (request === startupRequest) startupBusy.value = false
  }
}

watch(() => props.visible, (visible) => {
  if (!visible) return
  draft.closeAction = props.closeAction
  draft.removeRouteOnMatch = props.removeRouteOnMatch
  draft.notificationPopup = props.notificationPopup
  draft.autoStartMonitoring = props.autoStartMonitoring
  draft.startMinimized = props.startMinimized
  draft.autoCheckUpdates = props.autoCheckUpdates
  loadStartup()
}, { immediate: true })

const save = () => emit('save', { ...draft })
</script>

<template>
  <div
    v-if="visible"
    class="settingsBackdrop"
    @click.self="emit('cancel')"
  >
    <section class="settingsDialog" role="dialog" aria-modal="true">
      <h2>{{ translate('programSettings') }}</h2>

      <div class="startupSection">
        <div class="settingRow">
          <span>{{ translate('loginStartup') }}</span>
          <button class="startupBtn loginStartupBtn" type="button" :disabled="startupBusy || !startup?.supported" @click="updateStartup">
            {{ translate(startupBusy ? 'startupLoading' : !startup ? 'startupUnavailable' : !startup.supported ? 'startupUnsupported' : startup.needsRepair ? 'startupRepair' : startup.registered ? 'startupRemove' : 'startupAdd') }}
          </button>
        </div>
        <p v-if="startup?.supported" class="startupHint">
          {{ translate(startup.needsRepair ? 'startupPathOutdated' : startup.registered ? 'startupRegistered' : 'startupNotRegistered') }}
          {{ translate('startupHint') }}
        </p>
        <p v-if="startupError" role="alert" class="startupError">{{ translate('startupFailed') }}: {{ startupError }}</p>
        <button v-if="startupError" type="button" :disabled="startupBusy" @click="loadStartup">{{ translate('retry') }}</button>
      </div>

      <div class="updateSection">
        <div class="updateControls">
          <label class="checkboxRow updateCheckbox">
            <span>{{ translate('autoCheckUpdates') }}</span>
            <input v-model="draft.autoCheckUpdates" type="checkbox" :aria-label="translate('autoCheckUpdates')" />
          </label>
          <div class="updateActions">
            <button
              class="startupBtn"
              type="button"
              :disabled="['checking', 'downloading', 'restarting'].includes(updatePhase)"
              @click="emit('check-updates')"
            >
              {{ translate('checkForUpdates') }}
            </button>
            <button
              class="startupBtn"
              type="button"
              :disabled="updatePhase !== 'available'"
              @click="emit('download-update')"
            >
              {{ translate('downloadUpdate') }}
            </button>
          </div>
        </div>
        <div class="updateMetaRow">
          <p class="updateLastChecked">{{ translate('lastUpdateCheck') }} {{ lastUpdateCheckText }}</p>
          <p v-if="updatePhase === 'checking'" class="updateStatus startupHint">{{ translate('checkingUpdates') }}</p>
          <p v-else-if="updatePhase === 'downloading'" class="updateStatus startupHint">{{ translate('downloadingUpdate') }}</p>
          <p v-else-if="updatePhase === 'restarting'" class="updateStatus startupHint">{{ translate('restartingUpdate') }}</p>
          <p v-else-if="updatePhase === 'current'" class="updateStatus startupHint">{{ translate('upToDate') }}</p>
          <p v-else-if="updatePhase === 'available'" class="updateStatus startupHint">{{ translate('updateAvailable') }} {{ updateVersion }}</p>
          <p v-else-if="updatePhase === 'downloaded'" class="updateStatus startupHint">{{ translate('updateDownloaded') }} {{ updateVersion }}</p>
          <p v-else-if="updatePhase === 'error'" role="alert" class="updateStatus startupError">{{ translate('updateFailed') }}: {{ updateError }}</p>
        </div>
      </div>

      <label class="settingRow">
        <span>{{ translate('closeAction') }}</span>
        <select v-model="draft.closeAction">
          <option value="tray">{{ translate('minimizeToTray') }}</option>
          <option value="quit">{{ translate('quitOnClose') }}</option>
        </select>
      </label>

      <label class="settingRow">
        <span>{{ translate('routeMatchDisconnect') }}</span>
        <select v-model="draft.removeRouteOnMatch">
          <option :value="true">{{ translate('enabled') }}</option>
          <option :value="false">{{ translate('disabled') }}</option>
        </select>
      </label>

      <label class="settingRow checkboxRow notificationRow">
        <span>{{ translate('networkMatchNotification') }}</span>
        <input v-model="draft.notificationPopup" type="checkbox" :aria-label="translate('networkMatchNotification')" />
      </label>

      <label class="settingRow checkboxRow autoMonitoringRow">
        <span>{{ translate('autoStartMonitoring') }}</span>
        <input v-model="draft.autoStartMonitoring" type="checkbox" :aria-label="translate('autoStartMonitoring')" />
      </label>

      <label class="settingRow checkboxRow">
        <span>{{ translate('startMinimized') }}</span>
        <input v-model="draft.startMinimized" type="checkbox" :aria-label="translate('startMinimized')" />
      </label>

      <div class="settingsActions">
        <button class="secondaryBtn" type="button" @click="emit('cancel')">
          {{ translate('cancel') }}
        </button>
        <button class="primaryBtn" type="button" @click="save">
          {{ translate('save') }}
        </button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.settingsBackdrop {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: grid;
  place-items: center;
  padding: 18px;
  background: rgba(0, 0, 0, 0.62);
}

.settingsDialog {
  max-height: calc(100vh - 36px);
  overflow-y: auto;
  width: min(440px, 100%);
  padding: 20px;
  border: 1px solid #405268;
  border-radius: 12px;
  background: #19202a;
  color: #eff4fb;
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.48);
}

h2 {
  margin: 0 0 20px;
  font-size: 17px;
}

.startupSection { margin-bottom: 18px; padding-bottom: 12px; border-bottom: 1px solid #344251; }
.updateSection { margin-bottom: 18px; padding-bottom: 12px; border-bottom: 1px solid #344251; }
.updateControls, .updateMetaRow {
  display: grid;
  grid-template-columns: minmax(130px, 1fr) minmax(170px, 1.2fr);
  align-items: center;
  gap: 12px;
}
.updateCheckbox { display: flex; align-items: center; gap: 8px; margin: 0; white-space: nowrap; }
.updateActions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; width: 100%; }
.updateActions .startupBtn { width: 100%; min-width: 0; }
.updateActions .startupBtn:disabled { border-color: #344251; background: #1e2631; color: #8593a3; opacity: 1; }
.startupHint { font-size: 12px; color: #a9bad0; line-height: 1.5; }
.startupError { font-size: 12px; color: #ff9c9c; overflow-wrap: anywhere; }
.updateMetaRow { align-items: start; margin-top: 8px; }
.updateLastChecked, .updateStatus { min-width: 0; margin: 0; font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
.updateLastChecked { color: #a9bad0; }
.updateStatus { justify-self: end; text-align: right; }
.startupBtn { padding: 7px; border: 1px solid #405268; border-radius: 6px; background: #263549; color: #eff4fb; cursor: pointer; }
.startupBtn:disabled { opacity: 0.6; cursor: default; }
.checkboxRow { cursor: pointer; }
.checkboxRow input { justify-self: start; margin: 0; accent-color: #3b82f6; }

.settingRow {
  display: grid;
  grid-template-columns: minmax(130px, 1fr) minmax(170px, 1.2fr);
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  color: #c8d5e7;
  font-size: 13px;
}

select {
  width: 100%;
  min-width: 0;
  height: 34px;
  padding: 0 8px;
  border: 1px solid #344251;
  border-radius: 6px;
  background: #0f141a;
  color: #eef5ff;
}

.settingsActions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 22px;
}

.settingsActions button {
  min-width: 76px;
  height: 32px;
  padding: 0 12px;
  border-radius: 6px;
  cursor: pointer;
}

.secondaryBtn {
  border: 1px solid #32404f;
  background: #1e2631;
  color: #eff4fb;
}

@media (max-width: 420px) {
  .updateControls, .updateMetaRow { grid-template-columns: 1fr; }
  .updateCheckbox { white-space: normal; }
  .updateStatus { justify-self: start; text-align: left; }
  .settingRow {
    grid-template-columns: 1fr;
    gap: 6px;
  }
}
</style>
