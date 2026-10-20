import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import LogPanel from './LogPanel.vue'

const clearLogs = vi.fn()
const addNotification = vi.fn()

vi.mock('../services/backend', () => ({
  backend: { clearLogs: (...args) => clearLogs(...args) },
}))

vi.mock('../composables/useNotification', () => ({
  useNotification: () => ({ addNotification: (...args) => addNotification(...args) }),
}))

const translate = (key) => key

describe('LogPanel', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders logs as read-only text in chronological order', () => {
    const wrapper = mount(LogPanel, { props: { logs: ['first', 'second'], translate } })
    const area = wrapper.get('textarea')

    expect(wrapper.get('.sectionTitle').text()).toBe('runningLogs')
    expect(area.element.value).toBe('first\nsecond')
    expect(area.attributes('readonly')).toBeDefined()
  })

  it('scrolls to the latest log while the user is at the bottom', async () => {
    const wrapper = mount(LogPanel, { props: { logs: ['first'], translate } })
    const area = wrapper.get('textarea').element
    Object.defineProperties(area, {
      scrollHeight: { configurable: true, value: 500 },
      clientHeight: { configurable: true, value: 100 },
    })

    await wrapper.setProps({ logs: ['first', 'second'] })
    await nextTick()

    expect(area.scrollTop).toBe(500)
  })

  it('keeps the reader position when they have scrolled up', async () => {
    const wrapper = mount(LogPanel, { props: { logs: ['first'], translate } })
    const area = wrapper.get('textarea').element
    Object.defineProperties(area, {
      scrollHeight: { configurable: true, value: 500 },
      clientHeight: { configurable: true, value: 100 },
    })
    area.scrollTop = 100
    area.dispatchEvent(new Event('scroll'))

    await wrapper.setProps({ logs: ['first', 'second'] })
    await nextTick()

    expect(area.scrollTop).toBe(100)

    area.scrollTop = 400
    area.dispatchEvent(new Event('scroll'))
    await wrapper.setProps({ logs: ['first', 'second', 'third'] })
    await nextTick()
    expect(area.scrollTop).toBe(500)
  })

  it('clears logs and reports success or failure', async () => {
    clearLogs.mockResolvedValueOnce(undefined)
    const wrapper = mount(LogPanel, { props: { logs: [], translate } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(clearLogs).toHaveBeenCalledOnce()
    expect(addNotification).toHaveBeenCalledWith('logsCleared', 'success')

    clearLogs.mockRejectedValueOnce(new Error('offline'))
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(addNotification).toHaveBeenLastCalledWith('logsClearFailed: Error: offline', 'error')
  })
})
