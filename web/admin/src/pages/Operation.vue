<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { usePolling } from '../polling'
import { label, date, vmLink } from '../labels'
import type { OperationDetail } from '../types'
import RefreshState from '../components/RefreshState.vue'
const route = useRoute()
const path = computed(() => '/operations/' + encodeURIComponent(String(route.params.id)))
const { data: operation, error, updatedAt, loading } = usePolling<OperationDetail>(path)
const temporalURL = computed(() =>
  operation.value
    ? 'http://localhost:8080/namespaces/default/workflows/' +
      encodeURIComponent(operation.value.workflow_id) +
      '/' +
      encodeURIComponent(operation.value.run_id) +
      '/history'
    : '',
)
const stage = computed(() =>
  !operation.value
    ? ''
    : operation.value.status === 'rollback-blocked'
      ? 'Откат заблокирован'
      : !operation.value.delivery.delivered_at
        ? 'Ожидает доставки в Temporal'
        : operation.value.phase === 'rollback'
          ? 'Откат'
          : 'Выполнение',
)
</script>
<template>
  <RouterLink to="/operations" class="back">← Операции</RouterLink>
  <div class="page-heading">
    <div>
      <div class="eyebrow">ХОД ВЫПОЛНЕНИЯ</div>
      <h1>{{ operation ? label(operation.kind) : 'Операция' }}</h1>
      <p class="mono">{{ route.params.id }}</p>
    </div>
    <span v-if="operation" :class="['badge', operation.status]">{{ label(operation.status) }}</span>
  </div>
  <RefreshState :error="error" :updated-at="updatedAt" :loading="loading" />
  <template v-if="operation"
    ><div v-if="operation.status === 'rollback-blocked'" class="notice danger">
      <strong>Откат заблокирован — ресурсы удерживаются</strong>
      <p>
        Безопасная очистка не подтверждена. Для продолжения откройте VM и выполните восстановление.
      </p>
      <RouterLink :to="vmLink(operation.vm_id)">Перейти к восстановлению VM →</RouterLink>
    </div>
    <div class="card details">
      <div>
        <small>VM</small
        ><RouterLink :to="vmLink(operation.vm_id)">{{ operation.vm_id }}</RouterLink>
      </div>
      <div>
        <small>Инициатор</small><strong>{{ operation.user_id }}</strong>
      </div>
      <div>
        <small>Создана</small><strong>{{ date(operation.created_at) }}</strong>
      </div>
      <div>
        <small>Этап</small><strong>{{ stage }}</strong>
      </div>
      <div>
        <small>Последний сохранённый шаг</small><strong>{{ operation.step || 'Ещё нет' }}</strong>
      </div>
      <div>
        <small>Цель восстановления</small><strong>{{ label(operation.recovery_target) }}</strong>
      </div>
      <div v-if="operation.source_operation_id">
        <small>Исходная операция</small
        ><RouterLink :to="'/operations/' + operation.source_operation_id">Открыть →</RouterLink>
      </div>
    </div>
    <div v-if="operation.error" class="notice danger">
      <strong>Сохранённая ошибка</strong>
      <p class="error-text">{{ operation.error }}</p>
    </div>
    <div class="columns">
      <section class="card padded">
        <h2>Доставка команды</h2>
        <p>
          <span class="badge">{{
            operation.delivery.delivered_at ? 'Доставлена' : 'Ожидание доставки'
          }}</span>
        </p>
        <dl>
          <dt>Попыток доставки</dt>
          <dd>{{ operation.delivery.attempts }}</dd>
          <dt>{{ operation.delivery.delivered_at ? 'Доставлена в' : 'Следующая попытка' }}</dt>
          <dd>{{ date(operation.delivery.delivered_at || operation.delivery.next_attempt_at) }}</dd>
        </dl>
        <a
          v-if="operation.delivery.delivered_at && operation.run_id"
          :href="temporalURL"
          target="_blank"
          rel="noreferrer"
          >Открыть workflow в Temporal UI ↗</a
        >
        <p class="muted">Подробные попытки Activities и диагностика доступны в Temporal UI.</p>
      </section>
      <section class="card padded">
        <h2>Завершённые шаги</h2>
        <p v-if="!operation.steps.length" class="muted">Завершённых шагов пока нет.</p>
        <ol class="steps">
          <li v-for="step in operation.steps" :key="step.step">
            <i class="pi pi-check-circle" />
            <div>
              <strong>{{ step.step }}</strong
              ><small>{{ date(step.completed_at) }}</small>
            </div>
          </li>
        </ol>
      </section>
    </div></template
  >
</template>
