<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'

const props = defineProps({
  isAdmin: { type: Boolean, required: true },
  running: { type: Boolean, required: true },
  translate: { type: Function, required: true },
  health: { type: Object, default: () => ({}) },
  interval: { type: Number, default: 3 },
})

const now = ref(Date.now())
let timer
onMounted(() => { timer = setInterval(() => { now.value = Date.now() }, 1000) })
onUnmounted(() => clearInterval(timer))
const stale = computed(() => props.running && props.health.startedAt &&
  now.value - (props.health.lastSuccessAt || props.health.startedAt) > Math.max(30000, (props.health.intervalSeconds || props.interval) * 3000))
const phase = computed(() => stale.value ? 'stale' : (props.health.phase || 'stopped'))
const timeText = (value) => value ? new Date(value).toLocaleString() : props.translate('healthNever')
const failureText = (kind) => props.translate(`failure_${kind}`)
</script>

<template>
  <footer class="statusBar">
    <span class="healthPermission">
      {{ translate('currentPermission') }}
      <strong :class="isAdmin ? 'adminText' : 'userText'">
        {{ isAdmin ? translate('admin') : translate('nonAdmin') }}
      </strong>
    </span>
    <span>
      {{ translate('runningState') }}
      <strong>{{ running ? translate('running') : translate('notRunning') }}</strong>
    </span>
    <span class="healthDetail" :class="{ userText: stale || health.queryFailure || health.observationOnly }" role="status" aria-live="polite">
      {{ translate('healthStatus') }} {{ translate(`health_${phase}`) }}
      · {{ translate('healthFailures') }} {{ health.consecutiveFailures || 0 }}
      <template v-if="health.queryFailure"> · {{ failureText(health.queryFailure) }}</template>
    </span>
    <span v-if="health.observationOnly" class="healthDetail userText" role="status">
      {{ translate('healthObservationOnlyHint') }}
    </span>
    <span class="healthDetail">
      {{ translate('healthLastAttempt') }} {{ timeText(health.lastAttemptAt) }}
      · {{ translate('healthLastSuccess') }} {{ timeText(health.lastSuccessAt) }}
    </span>
    <span v-if="health.disconnectState && health.disconnectState !== 'idle'" class="healthDetail"
      :class="{ userText: health.disconnectFailure }">
      {{ translate('healthRouteAction') }} {{ translate(`disconnect_${health.disconnectState}`) }}
      <template v-if="health.disconnectFailure"> · {{ failureText(health.disconnectFailure) }}</template>
      <template v-if="running && health.nextRetryAt">
        · {{ translate('healthNextRetry') }} {{ timeText(health.nextRetryAt) }}
      </template>
    </span>
  </footer>
</template>
