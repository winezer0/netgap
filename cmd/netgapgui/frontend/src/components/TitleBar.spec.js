import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TitleBar from './TitleBar.vue'

const {
  windowApi,
  screenApi,
} = vi.hoisted(() => ({
  windowApi: {
    Size: vi.fn(), SetSize: vi.fn(), Position: vi.fn(), SetPosition: vi.fn(), IsMaximised: vi.fn(),
  },
  screenApi: { GetAll: vi.fn() },
}))

vi.mock('@wailsio/runtime', () => ({
  Window: windowApi,
  Screens: screenApi,
}))

vi.mock('../services/backend', () => ({
  backend: { getElevationStatus: vi.fn().mockResolvedValue({ supported: false, isAdmin: false }) },
}))

describe('TitleBar', () => {
  beforeEach(() => {
    windowApi.Size.mockResolvedValue({ width: 640, height: 480 })
    windowApi.SetSize.mockResolvedValue(undefined)
    windowApi.Position.mockResolvedValue({ x: 80, y: 90 })
    windowApi.SetPosition.mockResolvedValue(undefined)
    windowApi.IsMaximised.mockResolvedValue(false)
    screenApi.GetAll.mockResolvedValue([{ isCurrent: true, width: 1600, height: 900 }])
  })

  it('shows the build version beside the application name', () => {
    const wrapper = mount(TitleBar, { props: { language: 'en', closeAction: 'tray' } })

    expect(wrapper.find('.titleText').text()).toContain('netgapgui')
    expect(wrapper.find('.appVersion').text()).toBe(import.meta.env.VITE_APP_VERSION || 'dev')
  })

  it('emits settings and close actions from the top right buttons', async () => {
    const wrapper = mount(TitleBar, { props: { language: 'en', closeAction: 'tray' } })
    const buttons = wrapper.findAll('.windowActions .iconBtn')
    await buttons[0].trigger('click')
    await buttons[2].trigger('click')

    expect(wrapper.emitted('open-settings')).toHaveLength(1)
    expect(wrapper.emitted('close-window')).toHaveLength(1)
    expect(buttons[2].attributes('title')).toBe('Minimize to tray')
  })

  it('preserves and restores the window bounds across maximize and restore', async () => {
    const wrapper = mount(TitleBar, { props: { language: 'en', closeAction: 'quit' } })
    const maximize = wrapper.findAll('.windowActions .iconBtn')[1]

    await maximize.trigger('click')
    await flushPromises()
    expect(windowApi.SetSize).toHaveBeenLastCalledWith(1200, 720)
    expect(windowApi.SetPosition).toHaveBeenLastCalledWith(200, 90)

    await maximize.trigger('click')
    await flushPromises()
    expect(windowApi.SetSize).toHaveBeenLastCalledWith(640, 480)
    expect(windowApi.SetPosition).toHaveBeenLastCalledWith(80, 90)
  })
})
