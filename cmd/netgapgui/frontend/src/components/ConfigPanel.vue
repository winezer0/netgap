<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps({
  interval: { type: Number, required: true },
  keywordInput: { type: String, required: true },
  allRegions: { type: String, required: true },
  apiEnabled: { type: Array, required: true },
  customApiUrl: { type: String, required: true },
  translate: { type: Function, required: true },
})

const emit = defineEmits([
  'update:interval',
  'update:keyword-input',
  'update:api-enabled',
  'update:custom-api-url',
  'add-region',
  'remove-region',
  'save-settings',
])

const apiList = [
  { key: 'ipconfig', label: 'ipconfig.io' },
  { key: 'ipify', label: 'api.ipify.org' },
  { key: 'seeip', label: 'api.seeip.org' },
  { key: 'ipapi', label: 'ip-api.com' },
  { key: 'pcjson', label: 'pconline' },
  { key: 'ip2location', label: 'ip2location' },
  { key: 'customip', label: 'custom-ipapi' },
]

const customApiHelpOpen = ref(false)
const customApiHelpRoot = ref(null)

function onCustomApiHelpFocusOut(event) {
  if (!event.currentTarget.contains(event.relatedTarget)) {
    customApiHelpOpen.value = false
  }
}

function onDocumentPointerDown(event) {
  if (!customApiHelpRoot.value?.contains(event.target)) {
    customApiHelpOpen.value = false
  }
}

function onWindowBlur() {
  customApiHelpOpen.value = false
}

onMounted(() => {
  document.addEventListener('pointerdown', onDocumentPointerDown)
  window.addEventListener('blur', onWindowBlur)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocumentPointerDown)
  window.removeEventListener('blur', onWindowBlur)
})

function onIntervalChange(event) {
  const value = Number(event.target.value || 0)
  emit('update:interval', value)
  emit('save-settings')
}

function onKeywordChange(event) {
  emit('update:keyword-input', event.target.value)
}

function onAddRegion() {
  emit('add-region', props.keywordInput)
}

function onRemoveRegion() {
  emit('remove-region', props.keywordInput)
}

function onApiToggle(apiKey) {
  if (apiKey === 'customip' && !props.customApiUrl.trim()) {
    return
  }

  const currentEnabled = [...props.apiEnabled]
  const index = currentEnabled.indexOf(apiKey)
  if (index === -1) {
    currentEnabled.push(apiKey)
  } else {
    currentEnabled.splice(index, 1)
  }
  emit('update:api-enabled', currentEnabled)
  emit('save-settings')
}

function isApiEnabled(apiKey) {
  return props.apiEnabled.includes(apiKey)
}

function onCustomApiUrlChange(event) {
  const customApiUrl = event.target.value
  emit('update:custom-api-url', customApiUrl)
  if (!customApiUrl.trim() && isApiEnabled('customip')) {
    emit('update:api-enabled', props.apiEnabled.filter((apiKey) => apiKey !== 'customip'))
  }
  emit('save-settings')
}
</script>

<template>
  <section class="panel configPanel">
    <label class="formRow">
      <span class="labelText">{{ translate('interval') }}</span>
      <input
        :value="interval"
        class="intervalInput"
        type="number"
        min="1"
        max="60"
        @change="onIntervalChange"
      />
      <span class="unitBox">{{ translate('secondsUnit') }}</span>
    </label>
    <div class="formRow">
      <span class="labelText">{{ translate('regionKeyword') }}</span>
      <input
        :value="keywordInput"
        class="textInput"
        type="text"
        @input="onKeywordChange"
      />
      <button class="smallBtn" @click="onAddRegion">{{ translate('add') }}</button>
      <button class="smallBtn dangerBtn" @click="onRemoveRegion">{{ translate('remove') }}</button>
    </div>
    <label class="textAreaBlock">
      <span class="labelText">{{ translate('allRegions') }}</span>
      <textarea :value="allRegions" class="regionsArea" readonly />
    </label>
    <div class="apiSection">
      <span class="labelText">{{ translate('apiSelection') || 'API选择' }}</span>
      <div class="apiCheckboxes">
        <label
          v-for="api in apiList"
          :key="api.key"
          class="apiCheckboxItem"
        >
          <input
            type="checkbox"
            :checked="isApiEnabled(api.key)"
            :disabled="api.key === 'customip' && !customApiUrl.trim()"
            :title="api.key === 'customip' && !customApiUrl.trim() ? translate('customApiUrlRequired') : ''"
            @change="onApiToggle(api.key)"
          />
          <span>{{ api.label }}</span>
        </label>
      </div>
    </div>
    <div class="customApiSection">
      <div
        ref="customApiHelpRoot"
        class="customApiLabelRow"
        @focusout="onCustomApiHelpFocusOut"
        @keydown.esc="customApiHelpOpen = false"
      >
        <span class="labelText">{{ translate('customApiUrl') || '自定义API地址' }}</span>
        <button
          class="helpIconBtn"
          type="button"
          :aria-expanded="customApiHelpOpen"
          :aria-label="translate('customApiHelpButton')"
          @click="customApiHelpOpen = !customApiHelpOpen"
        >
          ?
        </button>
        <div v-if="customApiHelpOpen" class="customApiHelpPopover" role="dialog">
          <p>{{ translate('customApiHelp') }}</p>
          <p>
            {{ translate('customApiProject') }}:
            <a
              href="https://github.com/winezer0/ipinfo"
              target="_blank"
              rel="noopener noreferrer"
            >https://github.com/winezer0/ipinfo</a>
          </p>
          <p>{{ translate('customApiSponsor') }}</p>
        </div>
      </div>
      <input
        :value="customApiUrl"
        class="customApiInput"
        type="text"
        placeholder="http://your-server:8080/ip?token=your-token"
        @input="onCustomApiUrlChange"
      />
    </div>
  </section>
</template>
