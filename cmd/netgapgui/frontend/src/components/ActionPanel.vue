<script setup>
import { ref } from 'vue'
import { backend } from '../services/backend'
import { useNotification } from '../composables/useNotification'

const props = defineProps({
  translate: { type: Function, required: true },
  startAction: { type: Function, required: true },
  stopAction: { type: Function, required: true },
})

const { addNotification } = useNotification()
const confirmRemoveRoute = ref(false)
const backupHistoryOpen = ref(false)
const backupHistory = ref({ records: [], invalidLines: 0 })
const selectedBackup = ref(null)
const backupDetail = ref(null)
const backupError = ref('')
const historyLoading = ref(false)
const detailLoading = ref(false)
const restoreLoading = ref(false)
const backupRestoreResult = ref(null)
const confirmHistoryRestore = ref(false)
let detailRequestID = 0

const loadingStates = ref({
  removeRoute: false,
  restoreRoute: false,
  queryRoute: false,
  queryProxy: false,
  startMonitor: false,
  stopMonitor: false
})

const removeRoute = async () => {
  confirmRemoveRoute.value = false
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
    const result = await backend.restoreDefaultRoute()
    addNotification(
      props.translate(result?.noop ? 'routeRestoreNotNeeded' : 'routeRestoreSuccess'),
      result?.noop ? 'info' : 'success',
    )
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

const openBackupHistory = async () => {
  backupHistoryOpen.value = true
  backupError.value = ''
  historyLoading.value = true
  try {
    backupHistory.value = await backend.listRouteBackups()
  } catch (error) {
    backupError.value = String(error)
  } finally {
    historyLoading.value = false
  }
}

const selectBackup = async (record) => {
  const requestID = ++detailRequestID
  selectedBackup.value = record
  backupDetail.value = null
  backupRestoreResult.value = null
  backupError.value = ''
  detailLoading.value = true
  try {
    const detail = await backend.getRouteBackupDetail(record.id)
    if (requestID === detailRequestID) backupDetail.value = detail
  } catch (error) {
    if (requestID === detailRequestID) backupError.value = String(error)
  } finally {
    if (requestID === detailRequestID) detailLoading.value = false
  }
}

const restoreSelectedBackup = async () => {
  if (!selectedBackup.value || restoreLoading.value) return
  restoreLoading.value = true
  backupError.value = ''
  backupRestoreResult.value = null
  try {
    const allowConflicts = backupDetail.value.routes.some(route => route.state === 'conflict')
    const result = await backend.restoreRouteBackup(selectedBackup.value.id, allowConflicts)
    confirmHistoryRestore.value = false
    const summary = `${props.translate('historyRestoreDone')}: ${result.restored}, ${props.translate('historyAlreadyPresent')} ${result.alreadyPresent}, ${props.translate('historyReplaced')} ${result.replaced}, ${props.translate('historySkipped')} ${result.skipped}, ${props.translate('historyFailed')} ${result.failed}`
    addNotification(summary, result.complete ? 'success' : 'warning', 9000)
    backupRestoreResult.value = result
    detailLoading.value = true
    backupDetail.value = await backend.getRouteBackupDetail(selectedBackup.value.id)
  } catch (error) {
    backupError.value = String(error)
    confirmHistoryRestore.value = false
    if (selectedBackup.value) {
      detailLoading.value = true
      try { backupDetail.value = await backend.getRouteBackupDetail(selectedBackup.value.id) } catch { /* Keep the original restore error visible. */ }
    }
  } finally {
    detailLoading.value = false
    restoreLoading.value = false
  }
}

const startMonitoring = async () => {
  if (loadingStates.value.startMonitor || loadingStates.value.stopMonitor) return
  loadingStates.value.startMonitor = true
  try {
    await props.startAction()
    addNotification(`${props.translate('startMonitoring')} ${props.translate('success')}`, 'success')
  } catch (error) {
    addNotification(`${props.translate('startMonitoringFailed')}: ${error instanceof Error ? error.message : String(error)}`, 'error', 6000)
  } finally {
    loadingStates.value.startMonitor = false
  }
}

const stopMonitoring = async () => {
  if (loadingStates.value.startMonitor || loadingStates.value.stopMonitor) return
  loadingStates.value.stopMonitor = true
  try {
    await props.stopAction()
    addNotification(`${props.translate('stopMonitoring')} ${props.translate('success')}`, 'success')
  } catch (error) {
    addNotification(`${props.translate('stopMonitoringFailed')}: ${error instanceof Error ? error.message : String(error)}`, 'error', 6000)
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
        @click="confirmRemoveRoute = true"
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
      <button class="primaryBtn action-btn" type="button" @click="openBackupHistory">
        {{ translate('routeBackupHistory') }}
      </button>
    </div>
    <div v-if="backupHistoryOpen" class="confirmBackdrop" role="presentation" @click.self="backupHistoryOpen = false">
      <section class="historyDialog" role="dialog" aria-modal="true" :aria-label="translate('routeBackupHistory')">
        <header class="historyHeader"><h2>{{ translate('routeBackupHistory') }}</h2><button type="button" :aria-label="translate('cancel')" @click="backupHistoryOpen = false">&times;</button></header>
        <p v-if="backupHistory.invalidLines" class="historyWarning">{{ translate('historyInvalidLines') }}: {{ backupHistory.invalidLines }}</p>
        <p v-if="backupError" class="historyError">{{ backupError }}</p>
        <div class="historyLayout">
          <div class="historyList" :aria-busy="historyLoading">
            <p v-if="historyLoading" class="historyLoadingLabel">{{ translate('loading') }}</p>
            <button v-for="record in backupHistory.records" :key="record.id" type="button" :disabled="restoreLoading" :class="{ selected: selectedBackup?.id === record.id }" @click="selectBackup(record)">
              <strong>{{ new Date(record.savedAt).toLocaleString() }}</strong>
              <span>{{ record.interfaces.join(', ') }} | IPv4 {{ record.ipv4Count }} | IPv6 {{ record.ipv6Count }}</span>
            </button>
            <p v-if="!backupHistory.records.length">{{ translate('historyEmpty') }}</p>
          </div>
          <div class="historyDetails" :aria-busy="detailLoading || restoreLoading">
            <div v-if="detailLoading" class="historyDetailLoading"><span class="historySpinner" />{{ translate('historyDetailLoading') }}</div>
            <template v-else-if="backupDetail">
            <h3>{{ new Date(backupDetail.summary?.savedAt || selectedBackup.savedAt).toLocaleString() }}</h3>
            <p v-if="backupRestoreResult" class="historyWarning">{{ translate('historyRestoreDone') }}: {{ backupRestoreResult.restored }} {{ translate('historyRestored') }}, {{ backupRestoreResult.alreadyPresent }} {{ translate('historyAlreadyPresent') }}, {{ backupRestoreResult.replaced }} {{ translate('historyReplaced') }}, {{ backupRestoreResult.skipped }} {{ translate('historySkipped') }}, {{ backupRestoreResult.failed }} {{ translate('historyFailed') }}</p>
            <p v-for="(failure, index) in backupRestoreResult?.failures || []" :key="`failure-${index}`" class="historyError">{{ failure }}</p>
            <p v-for="(route, index) in backupDetail.routes" :key="`${route.saved.interfaceGuid}-${route.saved.destination}-${route.saved.store}-${index}`" :class="`route-${route.state}`">
              <strong>{{ route.saved.destination }}</strong> | {{ route.saved.interfaceAlias || route.saved.interfaceGuid }} | {{ route.saved.nextHop }} | {{ translate(`historyState_${route.state}`) }}
              <span v-if="route.current"> ({{ route.current.nextHop }})</span>
              <small v-if="route.reason">{{ route.reason }}</small>
            </p>
            <p v-if="backupDetail.routes.some(route => route.state === 'conflict')" class="historyWarning">{{ translate('historyConflictWarning') }}</p>
            <button v-if="backupDetail.routes.some(route => route.state === 'missing' || route.state === 'conflict')" class="dangerBtn historyRestoreButton" type="button" :disabled="restoreLoading" @click="confirmHistoryRestore = true">{{ translate('historyRestoreConfirm') }}</button>
            <p v-if="backupDetail.routes.every(route => route.state === 'present')" class="historyNoop">{{ translate('historyAllRoutesPresent') }}</p>
            <p v-else-if="backupDetail.routes.some(route => route.state === 'unavailable')" class="historyNoop">
              {{ translate('historyUnavailableSummary') }}: {{ backupDetail.routes.filter(route => route.state === 'unavailable').length }} {{ translate('historyRouteCount') }}; {{ backupDetail.routes.filter(route => route.state === 'present').length }} {{ translate('historyPresentCount') }}
            </p>
            </template>
            <p v-else class="historySelectPrompt">{{ translate(selectedBackup ? 'historyDetailFailed' : 'historySelectPrompt') }}</p>
          </div>
        </div>
      </section>
    </div>
    <div v-if="confirmHistoryRestore" class="confirmBackdrop" role="presentation">
      <section class="confirmDialog" role="alertdialog" aria-modal="true">
        <h2>{{ translate('historyRestoreConfirm') }}</h2>
        <p v-if="backupDetail?.routes.some(route => route.state === 'conflict')" class="historyWarning">{{ translate('historyConflictWarning') }}</p>
        <p v-else>{{ translate('historyRestorePrompt') }}</p>
        <p v-for="(route, index) in backupDetail?.routes.filter(item => item.state === 'conflict') || []" :key="`conflict-${index}`" class="historyWarning">
          {{ route.saved.destination }} | {{ route.saved.interfaceAlias || route.saved.interfaceGuid }}: {{ route.current?.nextHop }} &rarr; {{ route.saved.nextHop }}
        </p>
        <p v-if="backupError" class="historyError">{{ backupError }}</p>
        <div class="confirmActions">
          <button class="secondaryBtn" type="button" :disabled="restoreLoading" @click="confirmHistoryRestore = false">{{ translate('cancel') }}</button>
          <button class="dangerBtn" type="button" :disabled="restoreLoading" @click="restoreSelectedBackup">{{ translate('historyConflictConfirm') }}</button>
        </div>
      </section>
    </div>
    <div v-if="confirmRemoveRoute" class="confirmBackdrop" role="presentation" @click.self="confirmRemoveRoute = false">
      <section class="confirmDialog" role="alertdialog" aria-modal="true" :aria-labelledby="'remove-route-title'" :aria-describedby="'remove-route-warning'">
        <h2 id="remove-route-title">{{ translate('removeRouteConfirmTitle') }}</h2>
        <p id="remove-route-warning">{{ translate('removeRouteConfirmWarning') }}</p>
        <div class="confirmActions">
          <button class="secondaryBtn" type="button" @click="confirmRemoveRoute = false">{{ translate('cancel') }}</button>
          <button class="dangerBtn" type="button" :disabled="loadingStates.removeRoute" @click="removeRoute">{{ translate('removeDefaultRoute') }}</button>
        </div>
      </section>
    </div>
    <div class="buttonRow">
      <button 
        class="primaryBtn successBtn action-btn" 
        :class="{ 'is-loading': loadingStates.startMonitor }"
        :disabled="loadingStates.startMonitor || loadingStates.stopMonitor"
        @click="startMonitoring"
      >
        {{ translate('startMonitoring') }}
      </button>
      <button 
        class="primaryBtn dangerBtn action-btn" 
        :class="{ 'is-loading': loadingStates.stopMonitor }"
        :disabled="loadingStates.startMonitor || loadingStates.stopMonitor"
        @click="stopMonitoring"
      >
        {{ translate('stopMonitoring') }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.confirmBackdrop {
  position: fixed;
  inset: 0;
  z-index: 1100;
  display: grid;
  place-items: center;
  padding: 18px;
  background: rgb(0 0 0 / 65%);
}

.confirmDialog {
  width: min(420px, 100%);
  padding: 20px;
  border: 1px solid #405268;
  border-radius: 10px;
  background: #19202a;
  color: #eff4fb;
  box-shadow: 0 18px 48px rgb(0 0 0 / 48%);
}

.confirmDialog h2 { margin: 0 0 12px; font-size: 17px; }
.confirmDialog p { line-height: 1.55; }
.confirmActions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 20px; }
.confirmActions button { padding: 8px 14px; border: 0; border-radius: 6px; color: white; cursor: pointer; }
.confirmActions .secondaryBtn { background: #354253; }
.confirmActions .dangerBtn { background: #9b2737; }
.confirmActions button:disabled { opacity: .6; cursor: not-allowed; }
.historyDialog { width: min(880px, calc(100vw - 36px)); max-height: 88vh; overflow: auto; padding: 20px; border: 1px solid #405268; border-radius: 10px; background: #19202a; color: #eff4fb; box-shadow: 0 18px 48px rgb(0 0 0 / 48%); scrollbar-color: #526276 #19202a; scrollbar-width: thin; }
.historyHeader { display: flex; justify-content: space-between; align-items: center; }
.historyHeader h2 { margin: 0 0 14px; font-size: 17px; }
.historyHeader button { background: transparent; color: inherit; border: 0; font-size: 24px; cursor: pointer; }
.historyLayout { display: grid; grid-template-columns: minmax(0, .9fr) minmax(0, 1.1fr); gap: 16px; align-items: start; }
.historyList { display: grid; align-content: start; gap: 8px; max-height: 55vh; overflow: auto; padding: 8px; border: 1px solid #303d4d; border-radius: 8px; background: #111820; scrollbar-color: #526276 #111820; scrollbar-width: thin; }
.historyList button { display: grid; gap: 5px; text-align: left; padding: 11px 12px; border: 1px solid #344355; border-radius: 7px; background: #1a2430; color: #eff4fb; cursor: pointer; transition: border-color .15s ease, background-color .15s ease; }
.historyList button:hover { border-color: #6684a8; background: #202d3b; }
.historyList button.selected { border-color: #60a5fa; background: #203752; box-shadow: inset 3px 0 #60a5fa; }
.historyList button:disabled { opacity: .6; cursor: wait; }
.historyLoadingLabel { margin: 2px 4px 4px; color: #a9bad0; font-size: 12px; }
.historyList span, .historyDetails small { color: #a9bad0; font-size: 12px; }
.historyDetails { min-width: 0; padding: 14px; border: 1px solid #4b6077; border-radius: 8px; background: #29394a; box-shadow: 0 3px 12px rgb(0 0 0 / 18%); }
.historyDetails[aria-busy="true"] { min-height: 120px; }
.historyDetailLoading, .historySelectPrompt { display: flex; min-height: 112px; align-items: center; justify-content: center; gap: 10px; color: #b7c7d8; text-align: center; }
.historySpinner { width: 16px; height: 16px; border: 2px solid #60758b; border-top-color: #60a5fa; border-radius: 50%; animation: spin .8s linear infinite; }
.historyDetails h3 { margin: 0 0 10px; font-size: 14px; }
.historyRestoreButton { margin-top: 14px; padding: 9px 16px; border: 1px solid #f87171; border-radius: 6px; background: #b42336; color: #fff; font-weight: 600; cursor: pointer; }
.historyRestoreButton:hover:not(:disabled) { background: #d12f45; }
.historyRestoreButton:disabled { opacity: .6; cursor: not-allowed; }
.historyNoop { margin-top: 12px !important; color: #a9bad0; }
.historyDetails p { padding: 8px 0; margin: 0; border-bottom: 1px solid #344251; overflow-wrap: anywhere; }
.historyDetails small { display: block; }
.historyWarning { color: #ffd166; }
.historyError { color: #ff9c9c; overflow-wrap: anywhere; }
.route-conflict { color: #ffd166; }
.route-unavailable { color: #ff9c9c; }
@media (max-width: 720px) { .historyLayout { grid-template-columns: minmax(0, 1fr); } .historyList { max-height: 26vh; } }
@media (max-width: 720px) { .historyList::-webkit-scrollbar, .historyDialog::-webkit-scrollbar { width: 8px; } .historyList::-webkit-scrollbar-thumb, .historyDialog::-webkit-scrollbar-thumb { border-radius: 8px; background: #526276; } .historyList::-webkit-scrollbar-track, .historyDialog::-webkit-scrollbar-track { background: #111820; } }

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
