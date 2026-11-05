<script setup>
defineProps({
  visible: { type: Boolean, required: true },
  version: { type: String, default: '' },
  translate: { type: Function, required: true },
})

const emit = defineEmits(['restart', 'later'])
</script>

<template>
  <div v-if="visible" class="updateBackdrop" role="presentation">
    <section class="updateDialog" role="alertdialog" aria-modal="true">
      <h2>{{ translate('updateReadyTitle') }}</h2>
      <p>{{ translate('updateReadyMessage') }} <strong>{{ version }}</strong></p>
      <div class="updateDialogActions">
        <button class="secondaryBtn" type="button" @click="emit('later')">
          {{ translate('later') }}
        </button>
        <button class="primaryBtn" type="button" @click="emit('restart')">
          {{ translate('restartToUpdate') }}
        </button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.updateBackdrop {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: grid;
  place-items: center;
  padding: 18px;
  background: rgba(0, 0, 0, 0.68);
}

.updateDialog {
  width: min(400px, 100%);
  padding: 22px;
  border: 1px solid #405268;
  border-radius: 12px;
  background: #19202a;
  color: #eff4fb;
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.48);
}

h2 { margin: 0 0 12px; font-size: 17px; }
p { color: #c8d5e7; font-size: 13px; line-height: 1.5; }
.updateDialogActions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
.updateDialogActions button { min-width: 86px; height: 34px; padding: 0 12px; border-radius: 6px; cursor: pointer; }
.secondaryBtn { border: 1px solid #32404f; background: #1e2631; color: #eff4fb; }
.primaryBtn { border: 1px solid #2563eb; background: #2563eb; color: #fff; }
</style>
