<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Checkbox from 'primevue/checkbox'
import { query } from '../api'
import { usePolling } from '../polling'
import { send, sending, pending, userID } from '../commands'
import { label, vmLink } from '../labels'
import type { Machine, Page } from '../types'
import RefreshState from '../components/RefreshState.vue'
import Pager from '../components/Pager.vue'
const router = useRouter()
const search = ref(''),
  state = ref(''),
  recovery = ref(false),
  offset = ref(0),
  creating = ref(false)
const form = reactive({ id: '', cpu: 4, memory: 8192, disk: 20, gpu: false })
watch([search, state, recovery], () => (offset.value = 0))
const path = computed(
  () =>
    '/vms?' +
    query({
      search: search.value,
      state: state.value,
      recovery_required: recovery.value,
      offset: offset.value,
    }),
)
const { data, error, updatedAt, loading } = usePolling<Page<Machine>>(path)
const valid = computed(
  () =>
    form.id.trim().length > 0 &&
    [form.cpu, form.memory, form.disk].every(
      (value) => Number.isInteger(value) && value > 0 && value <= 2147483647,
    ),
)
async function create() {
  if (!valid.value) return
  const operation = await send({
    vm_id: form.id.trim(),
    user_id: userID.value,
    kind: 'create',
    cpu: form.cpu,
    memory_mib: form.memory,
    disk_gib: form.disk,
    gpu: form.gpu,
  })
  if (operation) {
    creating.value = false
    await router.push('/operations/' + operation.id)
  }
}
</script>
<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">ОБЛАЧНАЯ ИНФРАСТРУКТУРА</div>
      <h1>Виртуальные машины</h1>
      <p>Конфигурация, подтверждённое состояние и выделенные ресурсы.</p>
    </div>
    <PButton label="Создать VM" icon="pi pi-plus" :disabled="!!pending" @click="creating = true" />
  </div>
  <div class="card">
    <div class="filters">
      <InputText v-model="search" placeholder="Поиск по ID" aria-label="Поиск по ID" /><select
        v-model="state"
        aria-label="Состояние"
      >
        <option value="">Все состояния</option>
        <option
          v-for="item in ['not-created', 'running', 'stopped', 'deleted', 'unknown']"
          :key="item"
          :value="item"
        >
          {{ label(item) }}
        </option></select
      ><label><Checkbox v-model="recovery" binary /> Требуется восстановление</label>
    </div>
    <DataTable :value="data?.items || []" stripedRows
      ><template #empty>Виртуальные машины не найдены</template>
      <Column header="ID"
        ><template #body="{ data }"
          ><RouterLink :to="vmLink(data.id)">{{ data.id }}</RouterLink></template
        ></Column
      >
      <Column header="Состояние"
        ><template #body="{ data }"
          ><span :class="['badge', data.state]">{{ label(data.state) }}</span></template
        ></Column
      >
      <Column header="Текущая операция"
        ><template #body="{ data }"
          ><RouterLink v-if="data.operation_id" :to="'/operations/' + data.operation_id">{{
            label(data.operation_status)
          }}</RouterLink
          ><span v-else>—</span></template
        ></Column
      >
      <Column field="cpu" header="CPU" /><Column field="memory_mib" header="RAM, MiB" />
      <Column header="GPU"
        ><template #body="{ data }">{{ data.gpu ? 'Да' : 'Нет' }}</template></Column
      ><Column field="disk_gib" header="Диск, GiB" /><Column field="ip" header="IP" /><Column
        field="node"
        header="Нода"
      />
      <Column header="Восстановление"
        ><template #body="{ data }"
          ><span :class="{ danger: data.recovery_required }">{{
            data.recovery_required ? 'Требуется' : '—'
          }}</span></template
        ></Column
      > </DataTable
    ><Pager :offset="offset" :count="data?.items.length || 0" @change="offset = $event" />
  </div>
  <RefreshState :error="error" :updated-at="updatedAt" :loading="loading" />
  <Dialog
    v-model:visible="creating"
    modal
    header="Новая виртуальная машина"
    :style="{ width: '480px' }"
    ><form class="form" @submit.prevent="create">
      <label>ID VM<InputText v-model="form.id" required autofocus maxlength="300" /></label>
      <div class="form-grid">
        <label
          >CPU<InputNumber
            v-model="form.cpu"
            :min="1"
            :max="2147483647"
            :use-grouping="false" /></label
        ><label
          >RAM, MiB<InputNumber
            v-model="form.memory"
            :min="1"
            :max="2147483647"
            :use-grouping="false" /></label
        ><label
          >Диск, GiB<InputNumber
            v-model="form.disk"
            :min="1"
            :max="2147483647"
            :use-grouping="false" /></label
        ><label class="check"><Checkbox v-model="form.gpu" binary /> Выделенная GPU</label>
      </div>
      <p>Создание включает запуск VM. Результат появится после подтверждения в БД.</p>
      <PButton
        type="submit"
        label="Создать и запустить"
        :disabled="!valid || !!pending"
        :loading="sending"
      /></form
  ></Dialog>
</template>
