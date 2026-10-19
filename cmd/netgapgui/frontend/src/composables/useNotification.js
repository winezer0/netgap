import { reactive } from 'vue'

const notifications = reactive([])
let nextId = 1

export function useNotification() {
  const addNotification = (message, type = 'info', duration = 2000) => {
    const id = nextId++
    notifications.push({ id, message, type })
    
    if (duration > 0) {
      setTimeout(() => {
        removeNotification(id)
      }, duration)
    }
  }

  const removeNotification = (id) => {
    const index = notifications.findIndex(n => n.id === id)
    if (index !== -1) {
      notifications.splice(index, 1)
    }
  }

  return {
    notifications,
    addNotification,
    removeNotification
  }
}
