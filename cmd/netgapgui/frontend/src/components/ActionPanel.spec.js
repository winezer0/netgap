import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ActionPanel from './ActionPanel.vue'

const { removeDefaultRoute, restoreDefaultRoute, listRouteBackups, getRouteBackupDetail, restoreRouteBackup, addNotification } = vi.hoisted(() => ({
  removeDefaultRoute: vi.fn(),
  restoreDefaultRoute: vi.fn(),
  listRouteBackups: vi.fn(),
  getRouteBackupDetail: vi.fn(),
  restoreRouteBackup: vi.fn(),
  addNotification: vi.fn(),
}))
vi.mock('../services/backend', () => ({ backend: { removeDefaultRoute, restoreDefaultRoute, listRouteBackups, getRouteBackupDetail, restoreRouteBackup } }))
vi.mock('../composables/useNotification', () => ({ useNotification: () => ({ addNotification }) }))

describe('route recovery notifications', () => {
  beforeEach(() => vi.clearAllMocks())
  it('shows conflicts before replacing routes from a selected history record', async () => {
    const record = { id: 'backup-id', savedAt: '2026-11-25T00:00:00Z', interfaces: ['Ethernet'], ipv4Count: 1, ipv6Count: 0 }
    const detail = { routes: [{ saved: { interfaceGuid: 'nic', interfaceAlias: 'Ethernet', destination: '0.0.0.0/0', nextHop: '192.0.2.1', store: 'ActiveStore' }, current: { nextHop: '192.0.2.9' }, state: 'conflict' }] }
    listRouteBackups.mockResolvedValue({ records: [record], invalidLines: 0 })
    getRouteBackupDetail.mockResolvedValue(detail)
    restoreRouteBackup.mockResolvedValue({ restored: 0, alreadyPresent: 0, replaced: 1, skipped: 0, failed: 0, failures: [], complete: true })
    const wrapper = mount(ActionPanel, { props: { translate: key => key, startAction: vi.fn(), stopAction: vi.fn() } })
    await wrapper.findAll('button')[4].trigger('click')
    await flushPromises()
    await wrapper.get('.historyList button').trigger('click')
    await flushPromises()
    await wrapper.get('.historyDetails > button').trigger('click')
    expect(restoreRouteBackup).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alertdialog"]').text()).toContain('192.0.2.9 → 192.0.2.1')
    await wrapper.get('[role="alertdialog"] .dangerBtn').trigger('click')
    await flushPromises()
    expect(restoreRouteBackup).toHaveBeenCalledWith('backup-id', true)
    expect(wrapper.text()).toContain('historyRestoreDone')
    wrapper.unmount()
  })

  it('does not restore a historical route when conflict confirmation is canceled', async () => {
    const record = { id: 'backup-id', savedAt: '2026-11-25T00:00:00Z', interfaces: [], ipv4Count: 1, ipv6Count: 0 }
    listRouteBackups.mockResolvedValue({ records: [record], invalidLines: 0 })
    getRouteBackupDetail.mockResolvedValue({ routes: [{ saved: { destination: '0.0.0.0/0' }, current: { nextHop: '192.0.2.9' }, state: 'conflict' }] })
    const wrapper = mount(ActionPanel, { props: { translate: key => key, startAction: vi.fn(), stopAction: vi.fn() } })
    await wrapper.findAll('button')[4].trigger('click')
    await flushPromises()
    await wrapper.get('.historyList button').trigger('click')
    await flushPromises()
    await wrapper.get('.historyDetails > button').trigger('click')
    await wrapper.get('[role="alertdialog"] .secondaryBtn').trigger('click')
    expect(restoreRouteBackup).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps the backup list mounted while loading selected details', async () => {
    const record = { id: 'backup-id', savedAt: '2026-11-25T00:00:00Z', interfaces: [], ipv4Count: 1, ipv6Count: 0 }
    listRouteBackups.mockResolvedValue({ records: [record], invalidLines: 0 })
    let resolveDetail
    getRouteBackupDetail.mockImplementation(() => new Promise(resolve => { resolveDetail = resolve }))
    const wrapper = mount(ActionPanel, { props: { translate: key => key, startAction: vi.fn(), stopAction: vi.fn() } })
    await wrapper.findAll('button')[4].trigger('click')
    await flushPromises()
    await wrapper.get('.historyList button').trigger('click')
    expect(wrapper.get('.historyList button').exists()).toBe(true)
    expect(wrapper.get('.historyDetailLoading').exists()).toBe(true)
    resolveDetail({ summary: record, routes: [] })
    await flushPromises()
    expect(wrapper.get('.historyDetails h3').exists()).toBe(true)
    wrapper.unmount()
  })

  it.each([
    ['present', 'historyAllRoutesPresent'],
    ['unavailable', 'historyUnavailableSummary'],
  ])('explains the %s state without a combined no-op message', async (state, expectedText) => {
    const record = { id: 'backup-id', savedAt: '2026-11-25T00:00:00Z', interfaces: ['WLAN'], ipv4Count: 1, ipv6Count: 0 }
    listRouteBackups.mockResolvedValue({ records: [record], invalidLines: 0 })
    getRouteBackupDetail.mockResolvedValue({ summary: record, routes: [{ saved: { interfaceGuid: 'nic', interfaceAlias: 'WLAN', destination: '0.0.0.0/0', nextHop: '192.0.2.1' }, state, reason: state === 'unavailable' ? 'interface identity changed' : '' }] })
    const wrapper = mount(ActionPanel, { props: { translate: key => key, startAction: vi.fn(), stopAction: vi.fn() } })
    await wrapper.findAll('button')[4].trigger('click')
    await flushPromises()
    await wrapper.get('.historyList button').trigger('click')
    await flushPromises()
    expect(wrapper.get('.historyDetails').text()).toContain(expectedText)
    expect(wrapper.get('.historyDetails').text()).not.toContain('historyNothingToRestore')
    expect(wrapper.find('.historyRestoreButton').exists()).toBe(false)
    wrapper.unmount()
  })

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
