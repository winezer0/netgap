import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { backend } from '../services/backend'
import { useNotification } from './useNotification'

vi.mock('../services/backend', () => ({ backend: { logUIError: vi.fn().mockResolvedValue() } }))

afterEach(() => {
  const { notifications, removeNotification } = useNotification()
  for (const item of [...notifications]) removeNotification(item.id)
  vi.clearAllMocks()
})

describe('error notification logging', () => {
  it('persists the complete error message without logging other notifications', () => {
    const { addNotification } = useNotification()
    addNotification('操作失败: permission denied (100%)', 'error')
    addNotification('success', 'success')
    expect(backend.logUIError).toHaveBeenCalledTimes(1)
    expect(backend.logUIError).toHaveBeenCalledWith('操作失败: permission denied (100%)')
  })

  it('does not log a duplicate event twice', () => {
    const { addNotification } = useNotification()
    addNotification('match', 'error', 0, { eventId: 42 })
    addNotification('match', 'error', 0, { eventId: 42 })
    expect(backend.logUIError).toHaveBeenCalledTimes(1)
  })

  it('keeps the original notification when logging fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    backend.logUIError.mockRejectedValueOnce(new Error('offline'))
    const { addNotification, notifications } = useNotification()
    addNotification('original failure', 'error')
    await flushPromises()
    expect(notifications[0].message).toBe('original failure')
    expect(consoleError).toHaveBeenCalledTimes(1)
    consoleError.mockRestore()
  })
})
