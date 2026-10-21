<script setup>
import { useNotification } from '../composables/useNotification'

const { notifications, removeNotification } = useNotification()
</script>

<template>
  <div class="notification-container">
    <TransitionGroup name="toast">
      <div
        v-for="item in notifications"
        :key="item.id"
        class="toast-item"
        :class="item.type"
        :role="item.type === 'error' ? 'alert' : 'status'"
        @click="removeNotification(item.id)"
      >
        <span>{{ item.message }}</span>
        <button class="close-btn" @click.stop="removeNotification(item.id)">×</button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.notification-container {
  position: fixed;
  top: 40px;
  right: 16px;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: none;
}

.toast-item {
  pointer-events: auto;
  min-width: 240px;
  max-width: min(480px, calc(100vw - 64px));
  overflow-wrap: anywhere;
  padding: 10px 16px;
  border-radius: 8px;
  background: #1e2631;
  color: #fff;
  font-size: 13px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
  cursor: pointer;
  border-left: 4px solid #344251;
}

.toast-item.success {
  border-left-color: #2bd46f;
  background: rgba(43, 212, 111, 0.1);
}

.toast-item.error {
  border-left-color: #ff6b6b;
  background: rgba(255, 107, 107, 0.1);
}

.toast-item.info {
  border-left-color: #3b82f6;
}

.close-btn {
  background: none;
  border: none;
  color: rgba(255, 255, 255, 0.5);
  font-size: 16px;
  cursor: pointer;
  padding: 0;
  margin-left: 8px;
  transition: color 0.2s;
}

.close-btn:hover {
  color: white;
}

/* 动画效果 */
.toast-enter-active,
.toast-leave-active {
  transition: all 0.3s ease;
}

.toast-enter-from {
  opacity: 0;
  transform: translateX(30px);
}

.toast-leave-to {
  opacity: 0;
  transform: translateX(30px);
}
</style>
