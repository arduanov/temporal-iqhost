<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { usePolling } from '../polling'
import { query } from '../api'
import { allowed, send, sending, pending, userID } from '../commands'
import { label } from '../labels'
import type { Machine, Operation, Page, Kind, Command } from '../types'
import OperationTable from '../components/OperationTable.vue'
import RefreshState from '../components/RefreshState.vue'
import Pager from '../components/Pager.vue'
const route = useRoute(),
  router = useRouter(),
  offset = ref(0)
const id = computed(() => String(route.query.vm_id || ''))
watch(id, () => (offset.value = 0))
const path = computed(() => '/vm?' + query({ vm_id: id.value }))
const { data: machine, error, updatedAt, loading } = usePolling<Machine>(path)
const historyPath = computed(
  () => '/operations?' + query({ vm_id: id.value, offset: offset.value }),
)
const history = usePolling<Page<Operation>>(historyPath)
const actions: { kind: Kind; title: string }[] = [
  { kind: 'create', title: 'Повторить создание' },
  { kind: 'start', title: 'Запустить' },
  { kind: 'stop', title: 'Остановить' },
  { kind: 'delete', title: 'Удалить' },
  { kind: 'cleanup', title: 'Восстановить' },
]
async function execute(kind: Kind) {
  const current = machine.value
  if (!current || !allowed(current, kind)) return
  if (
    kind === 'delete' &&
    !confirm(
      `Удалить VM «${current.id}»? После подтверждения удаления будут освобождены диск и IP.`,
    )
  )
    return
  if (
    kind === 'cleanup' &&
    !confirm(
      `Восстановить VM «${current.id}» до сохранённой цели «${label(current.recovery_target)}»? Очистка заблокирует старые команды и освободит ресурсы согласно этой цели.`,
    )
  )
    return
  const command: Command = { vm_id: current.id, kind, user_id: userID.value }
  if (kind === 'create')
    Object.assign(command, {
      cpu: current.cpu,
      memory_mib: current.memory_mib,
      disk_gib: current.disk_gib,
      gpu: current.gpu,
    })
  const operation = await send(command)
  if (operation) await router.push('/operations/' + operation.id)
}
</script>
<template>
  <RouterLink to="/vms" class="back">← Виртуальные машины</RouterLink>
  <div class="page-heading">
    <div>
      <div class="eyebrow">КАРТОЧКА VM</div>
      <h1>{{ id }}</h1>
      <p>Состояние из БД приложения. Команда не меняет его до подтверждения.</p>
    </div>
  </div>
  <RefreshState :error="error" :updated-at="updatedAt" :loading="loading" />
  <template v-if="machine"
    ><div v-if="machine.recovery_required" class="notice warning">
      <strong>Требуется восстановление</strong>
      <p>
        Ресурсы сохранены до безопасной очистки. Цель: {{ label(machine.recovery_target) }}.
        Доступно только восстановление.
      </p>
    </div>
    <div class="card details">
      <div>
        <small>Подтверждённое состояние</small
        ><strong
          ><span :class="['badge', machine.state]">{{ label(machine.state) }}</span></strong
        >
      </div>
      <div>
        <small>CPU / RAM</small><strong>{{ machine.cpu }} / {{ machine.memory_mib }} MiB</strong>
      </div>
      <div>
        <small>Диск / GPU в конфигурации</small
        ><strong>{{ machine.disk_gib }} GiB / {{ machine.gpu ? 'Да' : 'Нет' }}</strong>
      </div>
      <div>
        <small>Нода / резерв GPU</small
        ><strong>{{ machine.node || '—' }} / {{ machine.gpu_id || '—' }}</strong>
      </div>
      <div>
        <small>Действующий диск / IP</small
        ><strong>{{ machine.disk_id || '—' }} / {{ machine.ip || '—' }}</strong>
      </div>
      <div>
        <small>Текущая операция</small
        ><RouterLink v-if="machine.operation_id" :to="'/operations/' + machine.operation_id"
          >{{ label(machine.operation_status) }} →</RouterLink
        >
      </div>
    </div>
    <div class="actions">
      <PButton
        v-for="action in actions"
        :key="action.kind"
        :label="action.title"
        :severity="action.kind === 'delete' ? 'danger' : 'primary'"
        outlined
        :disabled="!allowed(machine, action.kind) || sending || !!pending"
        @click="execute(action.kind)"
      />
    </div>
    <p v-if="['pending', 'running'].includes(machine.operation_status)" class="muted">
      Команды недоступны, пока текущая операция не завершена.
    </p>
    <h2>История операций</h2>
    <div class="card">
      <OperationTable :items="history.data.value?.items || []" /><Pager
        :offset="offset"
        :count="history.data.value?.items.length || 0"
        @change="offset = $event"
      />
    </div>
    <RefreshState
      :error="history.error.value"
      :updated-at="history.updatedAt.value"
      :loading="history.loading.value"
    />
  </template>
</template>
