<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps({
  interval: { type: Number, required: true },
  keywordInput: { type: String, required: true },
  regions: { type: Array, required: true },
  deletedRegion: { type: String, default: '' },
  regionBusy: { type: Boolean, default: false },
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
  'undo-region',
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
const selectedRegion = ref('')
watch(() => props.regions, (regions) => {
  if (!regions.includes(selectedRegion.value)) selectedRegion.value = ''
})
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
    <div class="formRow regionInputRow">
      <span class="labelText">{{ translate('regionKeyword') }}</span>
      <input
        :value="keywordInput"
        class="textInput"
        type="text"
        @input="onKeywordChange"
        @keydown.enter.prevent="!regionBusy && onAddRegion()"
      />
      <button class="smallBtn" :disabled="regionBusy" @click="onAddRegion">{{ translate('add') }}</button>
    </div>
    <div class="textAreaBlock">
      <span id="regionsLabel" class="labelText">{{ translate('allRegions') }}</span>
      <div>
        <ul class="regionsArea regionList" aria-labelledby="regionsLabel">
          <li v-for="region in regions" :key="region" class="regionRow" :class="{ selected: selectedRegion === region }">
            <button class="regionSelect" type="button" :aria-pressed="selectedRegion === region" @click="selectedRegion = region">
              {{ region }}
            </button>
            <button class="regionDelete" type="button" :disabled="regionBusy" :title="`${translate('deleteRegion')}: ${region}`" :aria-label="`${translate('deleteRegion')}: ${region}`" @click="$emit('remove-region', region)">×</button>
          </li>
          <li v-if="!regions.length" class="regionEmpty">{{ translate('regionsEmpty') }}</li>
        </ul>
        <div v-if="deletedRegion" class="regionUndo" role="status" aria-live="polite">
          <span>{{ translate('regionDeleted') }}: {{ deletedRegion }}</span>
          <button type="button" class="regionUndoButton" :disabled="regionBusy" @click="$emit('undo-region')">{{ translate('undo') }}</button>
        </div>
      </div>
    </div>
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
