<script setup>
import appIcon from '../assets/images/title-icon.png'
import {
  WindowGetSize,
  WindowSetSize,
  WindowGetPosition,
  WindowSetPosition,
  WindowIsMaximised,
  ScreenGetAll,
} from '../../wailsjs/runtime/runtime'

defineProps({
  language: { type: String, required: true },
})

defineEmits([
  'toggle-language',
  'minimise-window',
  'hide-to-tray',
  'quit-app',
])

let isMaximised = false
let previousSize = { w: 500, h: 550 }
let previousPosition = { x: 0, y: 0 }

const toggleMaximise = async () => {
  try {
    const maximised = await WindowIsMaximised()
    
    if (maximised) {
      // 还原窗口
      await WindowSetSize(previousSize.w, previousSize.h)
      await WindowSetPosition(previousPosition.x, previousPosition.y)
      isMaximised = false
    } else {
      // 保存当前状态
      previousSize = await WindowGetSize()
      previousPosition = await WindowGetPosition()
      
      // 获取当前屏幕信息
      const screens = await ScreenGetAll()
      const currentScreen = screens.find(s => s.isCurrent) || screens[0]
      
      if (currentScreen) {
        // 计算窗口大小（屏幕的80%）
        const targetWidth = Math.min(1200, Math.floor(currentScreen.width * 0.8))
        const targetHeight = Math.min(900, Math.floor(currentScreen.height * 0.8))
        
        // 计算居中位置
        const centerX = Math.floor((currentScreen.width - targetWidth) / 2)
        const centerY = Math.floor((currentScreen.height - targetHeight) / 2)
        
        // 设置窗口大小和居中位置
        await WindowSetSize(targetWidth, targetHeight)
        await WindowSetPosition(centerX, centerY)
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
    <div class="titleText">netgapgui</div>
    <button class="langBtn" @click="$emit('toggle-language')">
      {{ language === 'zh' ? 'ZH' : 'EN' }}
    </button>
    <div class="windowActions">
      <button
        class="iconBtn"
        @click="$emit('minimise-window')"
        :title="language === 'zh' ? '最小化到任务栏' : 'Minimize to taskbar'"
      >
        -
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
        @click="$emit('hide-to-tray')"
        :title="language === 'zh' ? '最小化到托盘' : 'Minimize to tray'"
      >
        o
      </button>
      <button
        class="iconBtn closeBtn"
        @click="$emit('quit-app')"
        :title="language === 'zh' ? '关闭程序' : 'Quit app'"
      >
        x
      </button>
    </div>
  </header>
</template>
