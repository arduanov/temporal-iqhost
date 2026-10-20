<script setup lang="ts">
import { ref } from 'vue'
import { usePolling } from '../polling'
import { vmLink } from '../labels'
import type { Resources } from '../types'
import RefreshState from '../components/RefreshState.vue'
const { data, error, updatedAt, loading } = usePolling<Resources>(ref('/resources'))
</script>
<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">ИНВЕНТАРЬ · ТОЛЬКО ПРОСМОТР</div>
      <h1>Ресурсы</h1>
      <p>Сохранённые резервы приложения, не фактическая нагрузка оборудования.</p>
    </div>
  </div>
  <RefreshState :error="error" :updated-at="updatedAt" :loading="loading" />
  <template v-if="data"
    ><h2>Вычислительные ноды</h2>
    <div class="card">
      <DataTable :value="data.nodes"
        ><Column field="name" header="Нода" /><Column header="Размещение"
          ><template #body="{ data }"
            ><span :class="['badge', data.available ? 'succeeded' : 'failed']">{{
              data.available ? 'Разрешено' : 'Недоступно'
            }}</span></template
          ></Column
        ><Column header="CPU: всего / резерв / свободно"
          ><template #body="{ data }"
            >{{ data.cpu }} / {{ data.reserved_cpu }} / {{ data.free_cpu }}</template
          ></Column
        ><Column header="RAM, MiB: всего / резерв / свободно"
          ><template #body="{ data }"
            >{{ data.memory_mib }} / {{ data.reserved_memory_mib }} /
            {{ data.free_memory_mib }}</template
          ></Column
        ><Column header="VM"
          ><template #body="{ data }"
            ><div class="links">
              <RouterLink v-for="id in data.vm_ids" :key="id" :to="vmLink(id)">{{ id }}</RouterLink>
            </div></template
          ></Column
        ></DataTable
      >
    </div>
    <h2>GPU</h2>
    <div class="card">
      <DataTable :value="data.gpus"
        ><template #empty>GPU отсутствуют</template><Column field="id" header="ID" /><Column
          field="model"
          header="Модель"
        /><Column field="node" header="Нода" /><Column header="Новый резерв"
          ><template #body="{ data }">{{
            data.available ? 'Доступна' : 'Недоступна'
          }}</template></Column
        ><Column header="Владелец резерва"
          ><template #body="{ data }"
            ><RouterLink v-if="data.vm_id" :to="vmLink(data.vm_id)">{{ data.vm_id }}</RouterLink
            ><span v-else>—</span></template
          ></Column
        ></DataTable
      >
    </div>
    <h2>Хранилища</h2>
    <div class="resource-grid">
      <section v-for="pool in data.storage" :key="pool.id" class="card padded">
        <h3>
          {{ pool.name }}
          <span class="badge">{{ pool.available ? 'Размещение разрешено' : 'Недоступно' }}</span>
        </h3>
        <p>
          Всего {{ pool.capacity_gib }} GiB · резерв {{ pool.reserved_gib }} GiB · свободно
          {{ pool.free_gib }} GiB
        </p>
        <ul class="bindings">
          <li v-for="disk in pool.disks" :key="disk.id">
            Диск #{{ disk.id }} · {{ disk.size_gib }} GiB ·
            <RouterLink :to="vmLink(disk.vm_id)">{{ disk.vm_id }}</RouterLink>
          </li>
        </ul>
        <p v-if="!pool.disks.length" class="muted">Действующих дисков нет</p>
      </section>
    </div>
    <h2>IP-пулы</h2>
    <div class="resource-grid">
      <section v-for="pool in data.ip_pools" :key="pool.id" class="card padded">
        <h3>
          {{ pool.name }} <span class="badge">{{ pool.subnet }}</span>
        </h3>
        <p>Заведено {{ pool.total }} · занято {{ pool.reserved }} · свободно {{ pool.free }}</p>
        <details>
          <summary>Адреса и привязки</summary>
          <ul class="bindings">
            <li v-for="address in pool.addresses" :key="address.address">
              {{ address.address }} ·
              <RouterLink v-if="address.vm_id" :to="vmLink(address.vm_id)">{{
                address.vm_id
              }}</RouterLink
              ><span v-else class="muted">Свободен</span>
            </li>
          </ul>
        </details>
      </section>
    </div></template
  >
</template>
