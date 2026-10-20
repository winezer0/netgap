import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TitleBar from './TitleBar.vue'

const {
  WindowGetSize,
  WindowSetSize,
  WindowGetPosition,
  WindowSetPosition,
  WindowIsMaximised,
  ScreenGetAll,
} = vi.hoisted(() => ({
  WindowGetSize: vi.fn(),
  WindowSetSize: vi.fn(),
  WindowGetPosition: vi.fn(),
  WindowSetPosition: vi.fn(),
  WindowIsMaximised: vi.fn(),
  ScreenGetAll: vi.fn(),
}))

vi.mock('../../wailsjs/runtime/runtime', () => ({
  WindowGetSize,
  WindowSetSize,
  WindowGetPosition,
  WindowSetPosition,
  WindowIsMaximised,
  ScreenGetAll,
}))

describe('TitleBar', () => {
  beforeEach(() => {
    WindowGetSize.mockResolvedValue({ w: 640, h: 480 })
    WindowSetSize.mockResolvedValue(undefined)
    WindowGetPosition.mockResolvedValue({ x: 80, y: 90 })
    WindowSetPosition.mockResolvedValue(undefined)
    WindowIsMaximised.mockResolvedValue(false)
    ScreenGetAll.mockResolvedValue([{ isCurrent: true, width: 1600, height: 900 }])
  })

  it('emits settings and close actions from the top right buttons', async () => {
    const wrapper = mount(TitleBar, { props: { language: 'en', closeAction: 'tray' } })
    const buttons = wrapper.findAll('.windowActions button')
    await buttons[0].trigger('click')
    await buttons[2].trigger('click')

    expect(wrapper.emitted('open-settings')).toHaveLength(1)
    expect(wrapper.emitted('close-window')).toHaveLength(1)
    expect(buttons[2].attributes('title')).toBe('Minimize to tray')
  })

  it('preserves and restores the window bounds across maximize and restore', async () => {
    const wrapper = mount(TitleBar, { props: { language: 'en', closeAction: 'quit' } })
    const maximize = wrapper.findAll('.windowActions button')[1]

    await maximize.trigger('click')
    await flushPromises()
    expect(WindowSetSize).toHaveBeenLastCalledWith(1200, 720)
    expect(WindowSetPosition).toHaveBeenLastCalledWith(200, 90)

    await maximize.trigger('click')
    await flushPromises()
    expect(WindowSetSize).toHaveBeenLastCalledWith(640, 480)
    expect(WindowSetPosition).toHaveBeenLastCalledWith(80, 90)
  })
})
