<script setup lang="ts">
import type { Operation } from '../types'
import { label, date, vmLink } from '../labels'
defineProps<{ items: Operation[] }>()
</script>
<template>
  <DataTable :value="items" stripedRows
    ><template #empty>Операций нет</template>
    <Column header="Время"
      ><template #body="{ data }"
        ><RouterLink :to="'/operations/' + data.id">{{
          date(data.created_at)
        }}</RouterLink></template
      ></Column
    >
    <Column header="VM"
      ><template #body="{ data }"
        ><RouterLink :to="vmLink(data.vm_id)">{{ data.vm_id }}</RouterLink></template
      ></Column
    >
    <Column header="Команда"
      ><template #body="{ data }">{{ label(data.kind) }}</template></Column
    ><Column field="user_id" header="Инициатор" />
    <Column header="Статус"
      ><template #body="{ data }"
        ><span :class="['badge', data.status]">{{ label(data.status) }}</span></template
      ></Column
    >
    <Column header="Фаза"
      ><template #body="{ data }">{{ label(data.phase) }}</template></Column
    ><Column field="step" header="Последний шаг" />
  </DataTable>
</template>
