<script setup>
import { ref } from 'vue'
import { backend } from '../services/backend'
import { useNotification } from '../composables/useNotification'

defineProps({
  logs: { type: Array, required: true },
  translate: { type: Function, required: true },
})

const { addNotification } = useNotification()
const clearing = ref(false)

const clearLogs = async (translate) => {
  if (clearing.value) return
  clearing.value = true
  try {
    await backend.clearLogs()
    addNotification(translate('logsCleared'), 'success')
  } catch (err) {
    addNotification(translate('logsClearFailed') + ': ' + err, 'error')
  } finally {
    clearing.value = false
  }
}
</script>

<template>
  <section class="panel logPanel">
    <div class="sectionHeader">
      <div class="sectionTitle">{{ translate('runningLogs') }}</div>
      <button
        class="primaryBtn action-btn clearBtn"
        :class="{ 'is-loading': clearing }"
        :disabled="clearing"
        @click="clearLogs(translate)"
      >
        {{ translate('clearLogs') }}
      </button>
    </div>
    <textarea
      :value="logs.join('\n')"
      class="logsArea"
      readonly
    />
  </section>
</template>

<style scoped>
.sectionHeader {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.clearBtn {
  padding: 6px 10px;
  font-size: 12px;
  line-height: 1;
}

.action-btn {
  position: relative;
  overflow: hidden;
  transition: transform 0.2s cubic-bezier(0.4, 0, 0.2, 1), background-color 0.2s;
}

.action-btn:active:not(:disabled) {
  transform: scale(0.95);
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.is-loading {
  color: transparent !important;
}

.is-loading::after {
  content: "";
  position: absolute;
  top: 50%;
  left: 50%;
  width: 14px;
  height: 14px;
  margin-top: -7px;
  margin-left: -7px;
  border: 2px solid rgba(255,255,255,0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
