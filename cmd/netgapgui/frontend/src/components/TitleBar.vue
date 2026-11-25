<script setup>
import appIcon from '../assets/images/title-icon.png'
import AdminRestartButton from './AdminRestartButton.vue'
import { Window, Screens } from '@wailsio/runtime'

const appVersion = import.meta.env.VITE_APP_VERSION || 'dev'

defineProps({
  language: { type: String, required: true },
  closeAction: { type: String, required: true },
  isAdmin: { type: Boolean, default: false },
  translate: { type: Function, default: (key) => key },
})

defineEmits([
  'toggle-language',
  'open-settings',
  'close-window',
  'hide-to-tray',
])

let isMaximised = false
let previousSize = { width: 600, height: 800 }
let previousPosition = { x: 0, y: 0 }

const readNumber = (object, upperKey, lowerKey, fallback = 0) => {
  const value = object?.[upperKey] ?? object?.[lowerKey]
  return Number.isFinite(Number(value)) ? Number(value) : fallback
}

const toggleMaximise = async () => {
  try {
    const maximised = isMaximised || await Window.IsMaximised()
    
    if (maximised) {
      // 还原窗口
      await Window.SetSize(previousSize.width, previousSize.height)
      await Window.SetPosition(previousPosition.x, previousPosition.y)
      isMaximised = false
    } else {
      // 保存当前状态
      const size = await Window.Size()
      const currentWidth = readNumber(size, 'Width', 'width')
      const currentHeight = readNumber(size, 'Height', 'height')
      previousSize = {
        width: currentWidth > 0 ? currentWidth : 600,
        height: currentHeight > 0 ? currentHeight : 800,
      }
      const position = await Window.Position()
      previousPosition = {
        x: readNumber(position, 'X', 'x'),
        y: readNumber(position, 'Y', 'y'),
      }
      
      // 获取当前屏幕信息
      const screens = await Screens.GetAll()
      const currentScreen = screens.find(s => s.IsCurrent ?? s.isCurrent)
        || screens.find(s => s.IsPrimary ?? s.isPrimary)
        || screens[0]
      
      if (currentScreen) {
        // 计算窗口大小（屏幕的80%）
        const area = currentScreen.WorkArea ?? currentScreen.workArea ?? currentScreen.Bounds ?? currentScreen.bounds
        const screenSize = currentScreen.Size ?? currentScreen.size
        const screenWidth = readNumber(area, 'Width', 'width', readNumber(screenSize, 'Width', 'width', readNumber(currentScreen, 'Width', 'width')))
        const screenHeight = readNumber(area, 'Height', 'height', readNumber(screenSize, 'Height', 'height', readNumber(currentScreen, 'Height', 'height')))
        if (screenWidth <= 0 || screenHeight <= 0) return

        const targetWidth = Math.max(600, Math.min(1200, Math.floor(screenWidth * 0.8)))
        const targetHeight = Math.min(900, Math.floor(screenHeight * 0.8))
        
        // 计算居中位置
        const originX = readNumber(area, 'X', 'x')
        const originY = readNumber(area, 'Y', 'y')
        const centerX = originX + Math.floor((screenWidth - targetWidth) / 2)
        const centerY = originY + Math.floor((screenHeight - targetHeight) / 2)
        
        // 设置窗口大小和居中位置
        await Window.SetSize(targetWidth, targetHeight)
        await Window.SetPosition(centerX, centerY)
      }
      
      isMaximised = true
    }
  } catch (error) {
    console.error('Failed to toggle maximise:', error)
  }
}
</script>

<template>
  <header class="titleBar">
    <img class="titleIcon" :src="appIcon" alt="" aria-hidden="true" draggable="false" />
    <div class="titleText">
      <span>netgapgui</span>
      <span class="appVersion" :title="`Version ${appVersion}`">{{ appVersion }}</span>
    </div>
    <button class="langBtn" @click="$emit('toggle-language')">
      {{ language === 'zh' ? 'ZH' : 'EN' }}
    </button>
    <div class="windowActions">
      <AdminRestartButton :is-admin="isAdmin" :translate="translate" />
      <button
        class="iconBtn"
        @click="$emit('open-settings')"
        :title="language === 'zh' ? '设置' : 'Settings'"
        :aria-label="language === 'zh' ? '设置' : 'Settings'"
      >
        ⚙
      </button>
      <button
        class="iconBtn"
        @click="toggleMaximise()"
        :title="language === 'zh' ? '最大化/还原' : 'Maximize/Restore'"
      >
        □
      </button>
      <button
        class="iconBtn"
        @click="$emit('close-window')"
        :title="language === 'zh' ? (closeAction === 'tray' ? '最小化到托盘' : '关闭并退出程序') : (closeAction === 'tray' ? 'Minimize to tray' : 'Quit application')"
      >
        {{ closeAction === 'tray' ? 'o' : '×' }}
      </button>
      <button
        class="iconBtn trayBtn"
        type="button"
        @click="$emit('hide-to-tray')"
        :title="language === 'zh' ? '最小化到托盘' : 'Minimize to tray'"
        :aria-label="language === 'zh' ? '最小化到托盘' : 'Minimize to tray'"
      >
        o
      </button>
    </div>
  </header>
</template>
