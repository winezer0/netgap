import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ActionPanel from './ActionPanel.vue'

const { removeDefaultRoute, restoreDefaultRoute, addNotification } = vi.hoisted(() => ({
  removeDefaultRoute: vi.fn(),
  restoreDefaultRoute: vi.fn(),
  addNotification: vi.fn(),
}))
vi.mock('../services/backend', () => ({ backend: { removeDefaultRoute, restoreDefaultRoute } }))
vi.mock('../composables/useNotification', () => ({ useNotification: () => ({ addNotification }) }))

describe('route recovery notifications', () => {
  it('requires confirmation before removing default routes', async () => {
    removeDefaultRoute.mockResolvedValue()
    const wrapper = mount(ActionPanel, { props: { translate: key => key, startAction: vi.fn(), stopAction: vi.fn() } })
    await wrapper.findAll('button')[0].trigger('click')
    expect(removeDefaultRoute).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alertdialog"]').text()).toContain('removeRouteConfirmWarning')
    await wrapper.get('[role="alertdialog"] .dangerBtn').trigger('click')
    await flushPromises()
    expect(removeDefaultRoute).toHaveBeenCalledOnce()
    expect(addNotification).toHaveBeenLastCalledWith('routeRemovedSuccess', 'success')
    wrapper.unmount()
  })

  it('does not remove routes when confirmation is canceled', async () => {
    const wrapper = mount(ActionPanel, { props: { translate: key => key, startAction: vi.fn(), stopAction: vi.fn() } })
    await wrapper.findAll('button')[0].trigger('click')
    await wrapper.get('[role="alertdialog"] .secondaryBtn').trigger('click')
    expect(removeDefaultRoute).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alertdialog"]').exists()).toBe(false)
    wrapper.unmount()
  })

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
