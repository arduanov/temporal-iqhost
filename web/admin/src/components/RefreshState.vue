<script setup lang="ts">
defineProps<{ error: string; updatedAt: string; loading: boolean }>()
</script>
<template>
  <div class="refresh" :class="{ warning: error }" role="status">
    <template v-if="error"
      >{{ error }}. {{ updatedAt ? 'Показаны данные на ' + updatedAt : 'Данные пока не получены' }}.
      Повторяем автоматически.</template
    >
    <template v-else>{{
      updatedAt
        ? 'Обновлено в ' + updatedAt + ' · опрос каждые 2 с'
        : loading
          ? 'Загрузка…'
          : 'Ожидание обновления'
    }}</template>
  </div>
</template>
