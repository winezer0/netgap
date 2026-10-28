import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SettingsDialog from './SettingsDialog.vue'
import { backend } from '../services/backend'

vi.mock('../services/backend', () => ({ backend: { getStartupStatus: vi.fn(), setStartupEnabled: vi.fn() } }))

const translate = (key) => key
const baseProps = {
  visible: true,
  language: 'zh',
  closeAction: 'tray',
  removeRouteOnMatch: true,
  notificationPopup: true,
  translate,
}

describe('SettingsDialog', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    backend.getStartupStatus.mockResolvedValue({ supported: true, registered: false, needsRepair: false })
  })
  it('shows the saved defaults in the labeled selectors', () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    const selects = wrapper.findAll('select')

    expect(wrapper.get('[role="dialog"]').exists()).toBe(true)
    expect(selects).toHaveLength(2)
    expect(selects[0].element.value).toBe('tray')
    expect(selects[1].element.value).toBe('true')
  })

  it('shows the saved network match notification preference', () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })

    expect(wrapper.get('[aria-label="networkMatchNotification"]').element.checked).toBe(true)
  })

  it('emits the selected values when saved', async () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    const selects = wrapper.findAll('select')
    await selects[0].setValue('quit')
    await selects[1].setValue('false')
    await wrapper.get('[aria-label="networkMatchNotification"]').setValue(false)
    await wrapper.get('.primaryBtn').trigger('click')

    expect(wrapper.emitted('save')).toEqual([[
      {
        closeAction: 'quit',
        removeRouteOnMatch: false,
        notificationPopup: false,
        autoStartMonitoring: false,
        startMinimized: false,
      },
    ]])
  })

  it('adds and removes a startup entry immediately', async () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await flushPromises()
    backend.setStartupEnabled.mockResolvedValueOnce({ supported: true, registered: true })
    await wrapper.get('.startupBtn').trigger('click')
    await flushPromises()
    expect(backend.setStartupEnabled).toHaveBeenLastCalledWith(true)
    expect(wrapper.get('.startupBtn').text()).toBe('startupRemove')
    backend.setStartupEnabled.mockResolvedValueOnce({ supported: true, registered: false })
    await wrapper.get('.startupBtn').trigger('click')
    await flushPromises()
    expect(backend.setStartupEnabled).toHaveBeenLastCalledWith(false)
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('repairs a moved executable and displays errors', async () => {
    backend.getStartupStatus.mockResolvedValue({ supported: true, registered: true, needsRepair: true })
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await flushPromises()
    expect(wrapper.get('.startupBtn').text()).toBe('startupRepair')
    backend.setStartupEnabled.mockRejectedValue(new Error('denied'))
    await wrapper.get('.startupBtn').trigger('click')
    await flushPromises()
    expect(backend.setStartupEnabled).toHaveBeenCalledWith(true)
    expect(wrapper.get('[role="alert"]').text()).toContain('denied')
    expect(wrapper.get('.startupBtn').element.disabled).toBe(false)
  })

  it('disables unsupported registration and saves launch preferences', async () => {
    backend.getStartupStatus.mockResolvedValue({ supported: false })
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await flushPromises()
    expect(wrapper.get('.startupBtn').element.disabled).toBe(true)
    await wrapper.get('[aria-label="autoStartMonitoring"]').setValue(true)
    await wrapper.get('[aria-label="startMinimized"]').setValue(true)
    await wrapper.get('.primaryBtn').trigger('click')
    expect(wrapper.emitted('save')[0][0]).toMatchObject({ autoStartMonitoring: true, startMinimized: true })
  })

  it('shows read failures and refreshes registration when reopened', async () => {
    backend.getStartupStatus.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('unavailable')
    expect(wrapper.get('.startupBtn').element.disabled).toBe(true)
    await wrapper.setProps({ visible: false })
    await wrapper.setProps({ visible: true })
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('.startupBtn').text()).toBe('startupAdd')
  })

  it('prevents duplicate registration requests while an operation is pending', async () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await flushPromises()
    let finish
    backend.setStartupEnabled.mockReturnValue(new Promise((resolve) => { finish = resolve }))
    await wrapper.get('.startupBtn').trigger('click')
    expect(wrapper.get('.startupBtn').element.disabled).toBe(true)
    await wrapper.get('.startupBtn').trigger('click')
    expect(backend.setStartupEnabled).toHaveBeenCalledOnce()
    finish({ supported: true, registered: true })
    await flushPromises()
    expect(wrapper.get('.startupBtn').text()).toBe('startupRemove')
  })

  it('cancels without saving and resets its draft when reopened', async () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await wrapper.findAll('select')[0].setValue('quit')
    await wrapper.get('.secondaryBtn').trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(wrapper.emitted('save')).toBeUndefined()

    await wrapper.setProps({ visible: false })
    await wrapper.setProps({ visible: true, closeAction: 'tray', notificationPopup: true })
    expect(wrapper.findAll('select')[0].element.value).toBe('tray')
    expect(wrapper.get('[aria-label="networkMatchNotification"]').element.checked).toBe(true)
  })

  it('cancels when the backdrop is clicked but not when the dialog is clicked', async () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await wrapper.get('.settingsDialog').trigger('click')
    expect(wrapper.emitted('cancel')).toBeUndefined()
    await wrapper.get('.settingsBackdrop').trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
  })
})
