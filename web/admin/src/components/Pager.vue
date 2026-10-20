<script setup lang="ts">
defineProps<{ offset: number; count: number; limit?: number }>()
defineEmits<{ change: [offset: number] }>()
</script>
<template>
  <div class="pager">
    <PButton
      label="Назад"
      outlined
      :disabled="offset === 0"
      @click="$emit('change', Math.max(0, offset - (limit || 50)))"
    /><span>Строки {{ count ? offset + 1 : 0 }}–{{ offset + count }}</span
    ><PButton
      label="Далее"
      outlined
      :disabled="count < (limit || 50)"
      @click="$emit('change', offset + (limit || 50))"
    />
  </div>
</template>
