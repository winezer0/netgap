import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ActionPanel from './ActionPanel.vue'

const { restoreDefaultRoute, addNotification } = vi.hoisted(() => ({
  restoreDefaultRoute: vi.fn(),
  addNotification: vi.fn(),
}))
vi.mock('../services/backend', () => ({ backend: { restoreDefaultRoute } }))
vi.mock('../composables/useNotification', () => ({ useNotification: () => ({ addNotification }) }))

describe('route recovery notifications', () => {
  it.each([
    [{ success: true, noop: true }, 'routeRestoreNotNeeded', 'info'],
    [{ success: true, noop: false }, 'routeRestoreSuccess', 'success'],
  ])('reports recovery result %j', async (result, message, level) => {
    restoreDefaultRoute.mockResolvedValue(result)
    const wrapper = mount(ActionPanel, { props: { translate: key => key, startAction: vi.fn(), stopAction: vi.fn() } })
    await wrapper.findAll('button')[1].trigger('click')
    await flushPromises()
    expect(addNotification).toHaveBeenLastCalledWith(message, level)
    wrapper.unmount()
  })

  it('keeps an actual recovery error visible', async () => {
    restoreDefaultRoute.mockRejectedValue('CIM provider failure')
    const wrapper = mount(ActionPanel, { props: { translate: key => key, startAction: vi.fn(), stopAction: vi.fn() } })
    await wrapper.findAll('button')[1].trigger('click')
    await flushPromises()
    expect(addNotification).toHaveBeenLastCalledWith('routeRestoreFailed: CIM provider failure', 'error')
    wrapper.unmount()
  })
})
