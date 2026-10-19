import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import LogPanel from './LogPanel.vue'

describe('LogPanel.vue', () => {
  const translateMock = (key) => key

  it('renders section title correctly', () => {
    const wrapper = mount(LogPanel, {
      props: {
        logs: [],
        translate: translateMock
      }
    })
    expect(wrapper.find('.sectionTitle').text()).toBe('runLog')
  })

  it('renders logs and ensures scrolling properties', () => {
    const logs = ['Log line 1', 'Log line 2', 'Log line 3']
    const wrapper = mount(LogPanel, {
      props: {
        logs,
        translate: translateMock
      }
    })
    
    const textarea = wrapper.find('.logsArea')
    expect(textarea.element.value).toBe('Log line 1\nLog line 2\nLog line 3')
    expect(textarea.attributes('readonly')).toBeDefined()
    
    // Verify CSS classes are applied for layout
    expect(wrapper.classes()).toContain('logPanel')
    expect(textarea.classes()).toContain('logsArea')
  })
})
