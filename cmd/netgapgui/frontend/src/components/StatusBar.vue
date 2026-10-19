<script setup>
defineProps({
  isAdmin: { type: Boolean, required: true },
  running: { type: Boolean, required: true },
  translate: { type: Function, required: true },
})
</script>

<template>
  <footer class="statusBar">
    <span>
      {{ translate('currentPermission') }}
      <strong :class="isAdmin ? 'adminText' : 'userText'">
        {{ isAdmin ? translate('admin') : translate('nonAdmin') }}
      </strong>
    </span>
    <span>
      {{ translate('runningState') }}
      <strong>{{ running ? translate('running') : translate('notRunning') }}</strong>
    </span>
  </footer>
</template>
