<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { usePolling } from '../polling'
import { query } from '../api'
import { label } from '../labels'
import type { Operation, Page } from '../types'
import OperationTable from '../components/OperationTable.vue'
import RefreshState from '../components/RefreshState.vue'
import Pager from '../components/Pager.vue'
const vmID = ref(''),
  userID = ref(''),
  status = ref(''),
  offset = ref(0)
watch([vmID, userID, status], () => (offset.value = 0))
const path = computed(
  () =>
    '/operations?' +
    query({ vm_id: vmID.value, user_id: userID.value, status: status.value, offset: offset.value }),
)
const { data, error, updatedAt, loading } = usePolling<Page<Operation>>(path)
</script>
<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">ЖУРНАЛ КОМАНД</div>
      <h1>Операции</h1>
      <p>Доставка, выполнение и откат. Новые команды — первыми.</p>
    </div>
  </div>
  <div class="card">
    <div class="filters">
      <InputText v-model="vmID" placeholder="Точный ID VM" aria-label="ID VM" /><InputText
        v-model="userID"
        placeholder="Инициатор"
        aria-label="Фильтр инициатора"
      /><select v-model="status" aria-label="Статус">
        <option value="">Все статусы</option>
        <option
          v-for="item in [
            'pending',
            'running',
            'succeeded',
            'failed',
            'canceled',
            'rollback-blocked',
          ]"
          :key="item"
          :value="item"
        >
          {{ label(item) }}
        </option>
      </select>
    </div>
    <OperationTable :items="data?.items || []" /><Pager
      :offset="offset"
      :count="data?.items.length || 0"
      @change="offset = $event"
    />
  </div>
  <RefreshState :error="error" :updated-at="updatedAt" :loading="loading" />
</template>
