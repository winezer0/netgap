import { reactive } from 'vue'
import { backend } from '../services/backend'

const notifications = reactive([])
const timers = new Map()
let nextId = 1

function removeNotification(id) {
  clearTimeout(timers.get(id))
  timers.delete(id)
  const index = notifications.findIndex(item => item.id === id)
  if (index !== -1) notifications.splice(index, 1)
}

function addNotification(message, type = 'info', duration = 2000, metadata = {}) {
  if (metadata.eventId && notifications.some(item => item.eventId === metadata.eventId)) return
  if (type === 'error') {
    // Logging failures must not hide the original error or create another toast.
    try {
      Promise.resolve(backend.logUIError(String(message))).catch(error => console.error('Failed to log UI error', error))
    } catch (error) {
      console.error('Failed to log UI error', error)
    }
  }
  while (notifications.length >= 5) removeNotification(notifications[0].id)
  const id = nextId++
  notifications.push({ ...metadata, id, message, type })
  if (duration > 0) timers.set(id, setTimeout(() => removeNotification(id), duration))
  return id
}

function removeStaleMatchNotifications(runId, running) {
  for (const item of [...notifications]) {
    if (item.runId && (item.runId !== runId || !running)) removeNotification(item.id)
  }
}

export function useNotification() {
  return { notifications, addNotification, removeNotification, removeStaleMatchNotifications }
}
