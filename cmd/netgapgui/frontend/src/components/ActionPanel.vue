<script setup>
import { ref } from 'vue'
import { backend } from '../services/backend'
import { useNotification } from '../composables/useNotification'

const props = defineProps({
  translate: { type: Function, required: true },
})

const emit = defineEmits([
  'start-monitoring',
  'stop-monitoring',
])

const { addNotification } = useNotification()

const loadingStates = ref({
  removeRoute: false,
  restoreRoute: false,
  queryRoute: false,
  queryProxy: false,
  startMonitor: false,
  stopMonitor: false
})

const removeRoute = async () => {
  if (loadingStates.value.removeRoute) return
  loadingStates.value.removeRoute = true
  try {
    await backend.removeDefaultRoute()
    addNotification(props.translate('routeRemovedSuccess'), 'success')
  } catch (err) {
    addNotification(props.translate('routeRemovedFailed') + ': ' + err, 'error')
  } finally {
    loadingStates.value.removeRoute = false
  }
}

const restoreRoute = async () => {
  if (loadingStates.value.restoreRoute) return
  loadingStates.value.restoreRoute = true
  try {
    await backend.restoreDefaultRoute()
    addNotification(props.translate('routeRestoreSuccess'), 'success')
  } catch (err) {
    addNotification(props.translate('routeRestoreFailed') + ': ' + err, 'error')
  } finally {
    loadingStates.value.restoreRoute = false
  }
}

const queryDefaultRoute = async () => {
  if (loadingStates.value.queryRoute) return
  loadingStates.value.queryRoute = true
  try {
    const route = await backend.getDefaultRouteInfo()
    const gateway = route?.gateway ?? ''
    const iface = route?.interface ?? ''
    addNotification(props.translate('defaultRoute') + ': ' + 'gateway=' + gateway + ' interface=' + iface, 'info')
  } catch (err) {
    addNotification(props.translate('queryDefaultRouteFailed') + ': ' + err, 'error')
  } finally {
    loadingStates.value.queryRoute = false
  }
}

const queryProxyConfig = async () => {
  if (loadingStates.value.queryProxy) return
  loadingStates.value.queryProxy = true
  try {
    const info = await backend.getProxyConfigInfo()
    const enabled = info?.enabled ? 'true' : 'false'
    const source = info?.source ?? ''
    const address = info?.address ?? ''
    const port = info?.port ?? ''
    addNotification(props.translate('proxyConfig') + ': ' + 'enabled=' + enabled + ' source=' + source + ' ' + address + (port ? ':' + port : ''), 'info')
  } catch (err) {
    addNotification(props.translate('queryProxyConfigFailed') + ': ' + err, 'error')
  } finally {
    loadingStates.value.queryProxy = false
  }
}

const startMonitoring = async () => {
  if (loadingStates.value.startMonitor) return
  loadingStates.value.startMonitor = true
  try {
    emit('start-monitoring')
    // We let useAppState or backend emit the state change, just show notification
    addNotification(props.translate('startMonitoring') + ' ' + props.translate('success') || 'Success', 'success')
  } catch {
    addNotification(props.translate('startMonitoring') + ' ' + props.translate('failed') || 'Failed', 'error')
  } finally {
    loadingStates.value.startMonitor = false
  }
}

const stopMonitoring = async () => {
  if (loadingStates.value.stopMonitor) return
  loadingStates.value.stopMonitor = true
  try {
    emit('stop-monitoring')
    addNotification(props.translate('stopMonitoring') + ' ' + props.translate('success') || 'Success', 'success')
  } catch {
    addNotification(props.translate('stopMonitoring') + ' ' + props.translate('failed') || 'Failed', 'error')
  } finally {
    loadingStates.value.stopMonitor = false
  }
}
</script>

<template>
  <section class="panel actionPanel">
    <div class="buttonRow">
      <button 
        class="primaryBtn action-btn" 
        :class="{ 'is-loading': loadingStates.removeRoute }"
        :disabled="loadingStates.removeRoute" 
        @click="removeRoute"
      >
        {{ translate('removeDefaultRoute') }}
      </button>
      <button 
        class="primaryBtn action-btn" 
        :class="{ 'is-loading': loadingStates.restoreRoute }"
        :disabled="loadingStates.restoreRoute" 
        @click="restoreRoute"
      >
        {{ translate('restoreDefaultRoute') }}
      </button>
      <button 
        class="primaryBtn action-btn" 
        :class="{ 'is-loading': loadingStates.queryRoute }"
        :disabled="loadingStates.queryRoute" 
        @click="queryDefaultRoute"
      >
        {{ translate('queryDefaultRoute') }}
      </button>
      <button 
        class="primaryBtn action-btn" 
        :class="{ 'is-loading': loadingStates.queryProxy }"
        :disabled="loadingStates.queryProxy" 
        @click="queryProxyConfig"
      >
        {{ translate('queryProxyConfig') }}
      </button>
    </div>
    <div class="buttonRow">
      <button 
        class="primaryBtn successBtn action-btn" 
        :class="{ 'is-loading': loadingStates.startMonitor }"
        :disabled="loadingStates.startMonitor" 
        @click="startMonitoring"
      >
        {{ translate('startMonitoring') }}
      </button>
      <button 
        class="primaryBtn dangerBtn action-btn" 
        :class="{ 'is-loading': loadingStates.stopMonitor }"
        :disabled="loadingStates.stopMonitor" 
        @click="stopMonitoring"
      >
        {{ translate('stopMonitoring') }}
      </button>
    </div>
  </section>
</template>

<style scoped>
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

/* 加载动画特效 */
.is-loading {
  color: transparent !important;
}

.is-loading::after {
  content: "";
  position: absolute;
  top: 50%;
  left: 50%;
  width: 16px;
  height: 16px;
  margin-top: -8px;
  margin-left: -8px;
  border: 2px solid rgba(255,255,255,0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
