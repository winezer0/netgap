<script setup>
import { reactive, watch } from 'vue'

const props = defineProps({
  visible: { type: Boolean, required: true },
  language: { type: String, required: true },
  closeAction: { type: String, required: true },
  removeRouteOnMatch: { type: Boolean, required: true },
  notificationPopup: { type: Boolean, required: true },
  translate: { type: Function, required: true },
})

const emit = defineEmits(['cancel', 'save'])
const draft = reactive({
  closeAction: 'tray',
  removeRouteOnMatch: true,
  notificationPopup: true,
})

watch(() => props.visible, (visible) => {
  if (!visible) return
  draft.closeAction = props.closeAction
  draft.removeRouteOnMatch = props.removeRouteOnMatch
  draft.notificationPopup = props.notificationPopup
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

      <div class="settingRow notificationRow">
        <span>{{ translate('networkMatchNotification') }}</span>
        <label class="settingChoice">
          <input
            v-model="draft.notificationPopup"
            type="checkbox"
            :aria-label="translate('showPopupNotification')"
          />
          <span>{{ translate('showPopupNotification') }}</span>
        </label>
      </div>

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

.settingChoice {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #c8d5e7;
  cursor: pointer;
  white-space: nowrap;
}

.settingChoice input {
  margin: 0;
  accent-color: #3b82f6;
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
  .settingRow {
    grid-template-columns: 1fr;
    gap: 6px;
  }
}
</style>
