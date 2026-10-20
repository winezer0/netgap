import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SettingsDialog from './SettingsDialog.vue'

const translate = (key) => key
const baseProps = {
  visible: true,
  language: 'zh',
  closeAction: 'tray',
  removeRouteOnMatch: true,
  translate,
}

describe('SettingsDialog', () => {
  it('shows the saved defaults in two labeled selectors', () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    const rows = wrapper.findAll('.settingRow')

    expect(wrapper.get('[role="dialog"]').exists()).toBe(true)
    expect(rows).toHaveLength(2)
    expect(rows[0].find('span').text()).toBe('closeAction')
    expect(rows[0].get('select').element.value).toBe('tray')
    expect(rows[1].find('span').text()).toBe('routeMatchDisconnect')
    expect(rows[1].get('select').element.value).toBe('true')
  })

  it('emits both selected values when saved', async () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    const selects = wrapper.findAll('select')
    await selects[0].setValue('quit')
    await selects[1].setValue('false')
    await wrapper.get('.primaryBtn').trigger('click')

    expect(wrapper.emitted('save')).toEqual([[{ closeAction: 'quit', removeRouteOnMatch: false }]])
  })

  it('cancels without saving and resets its draft when reopened', async () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await wrapper.findAll('select')[0].setValue('quit')
    await wrapper.get('.secondaryBtn').trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(wrapper.emitted('save')).toBeUndefined()

    await wrapper.setProps({ visible: false })
    await wrapper.setProps({ visible: true, closeAction: 'tray' })
    expect(wrapper.findAll('select')[0].element.value).toBe('tray')
  })

  it('cancels when the backdrop is clicked but not when the dialog is clicked', async () => {
    const wrapper = mount(SettingsDialog, { props: baseProps })
    await wrapper.get('.settingsDialog').trigger('click')
    expect(wrapper.emitted('cancel')).toBeUndefined()
    await wrapper.get('.settingsBackdrop').trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
  })
})
