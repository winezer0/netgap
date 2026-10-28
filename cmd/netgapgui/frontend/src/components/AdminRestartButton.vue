<script setup>
import { computed, onMounted, ref } from 'vue'
import { backend } from '../services/backend'
import { useNotification } from '../composables/useNotification'

const props = defineProps({
  isAdmin: { type: Boolean, required: true },
  translate: { type: Function, required: true },
})

const status = ref(null)
const statusFailed = ref(false)
const busy = ref(false)
const alreadyAdmin = computed(() => props.isAdmin || status.value?.isAdmin)
const disabled = computed(() => busy.value || alreadyAdmin.value || !status.value?.supported)
const title = computed(() => {
  if (busy.value) return props.translate('adminRestartPending')
  if (alreadyAdmin.value) return props.translate('adminAlreadyElevated')
  if (statusFailed.value) return props.translate('adminStatusUnavailable')
  if (!status.value) return props.translate('adminStatusLoading')
  if (!status.value.supported) return props.translate('adminRestartUnsupported')
  return `${props.translate('restartAsAdmin')} — ${props.translate('adminRestartHint')}`
})

onMounted(async () => {
  try {
    status.value = await backend.getElevationStatus()
  } catch {
    statusFailed.value = true
  }
})

async function restart() {
  if (disabled.value) return
  busy.value = true
  const { addNotification } = useNotification()
  try {
    const result = await backend.restartAsAdmin()
    if (result?.cancelled) {
      busy.value = false
      addNotification(props.translate('adminRestartCancelled'), 'info')
    }
  } catch (error) {
    busy.value = false
    addNotification(`${props.translate('adminRestartFailed')}: ${error}`, 'error')
  }
}
</script>

<template>
  <span class="adminRestart" :title="title">
    <button
      class="adminRestartButton"
      type="button"
      :disabled="disabled"
      :aria-label="translate('restartAsAdmin')"
      :aria-busy="busy"
      @click="restart"
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M20 7v5h-5" />
        <path d="M20 12a8 8 0 1 0-2.3 5.7M20 12l-2.3-5.7" />
      </svg>
    </button>
  </span>
</template>

<style scoped>
.adminRestart { --wails-draggable: no-drag; display: inline-flex; }
.adminRestartButton {
  --wails-draggable: no-drag;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 26px;
  padding: 3px;
  border: 1px solid #32404f;
  border-radius: 6px;
  background: #1e2631;
  color: #eff4fb;
  cursor: pointer;
}
.adminRestartButton:hover:not(:disabled) { color: #eff4fb; background: #263549; }
.adminRestartButton:focus-visible { outline: 2px solid #60a5fa; outline-offset: 2px; }
.adminRestartButton:disabled { opacity: 0.45; cursor: default; }
</style>
