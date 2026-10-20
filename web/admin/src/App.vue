<script setup lang="ts">
import { watch } from 'vue'
import { useRouter } from 'vue-router'
import { userID, pending, sending, commandError, send, discard } from './commands'
const router = useRouter()
watch(userID, (value) => {
  localStorage.setItem('iqhost.user', value)
})
async function retry() {
  const operation = await send()
  if (operation) await router.push('/operations/' + operation.id)
}
</script>
<template>
  <div class="shell">
    <aside>
      <div class="brand">
        <span class="brand-icon">iq</span> IQHost <small>CLOUD CONSOLE</small>
      </div>
      <div class="nav-caption">УПРАВЛЕНИЕ</div>
      <nav>
        <RouterLink to="/vms"><i class="pi pi-server" />Виртуальные машины</RouterLink>
        <RouterLink to="/operations"><i class="pi pi-history" />Операции</RouterLink>
        <RouterLink to="/resources"><i class="pi pi-database" />Ресурсы</RouterLink>
      </nav>
      <a class="temporal" href="http://localhost:8080" target="_blank" rel="noreferrer"
        >Temporal UI <i class="pi pi-external-link"
      /></a>
      <p class="sidebar-note">PostgreSQL → outbox → Temporal<br />Подтверждённое состояние</p>
    </aside>
    <div class="workspace">
      <header>
        <span class="demo"><span />Локальное демо</span
        ><label class="identity"
          >Инициатор <InputText v-model="userID" aria-label="Инициатор" maxlength="256" /><small
            >Подпись команды, без проверки личности</small
          ></label
        >
      </header>
      <main>
        <div v-if="pending" class="notice warning">
          <strong
            >Незавершённая отправка: {{ pending.command.kind }} ·
            {{ pending.command.vm_id }}</strong
          >
          <p>
            Ответ мог потеряться. Повтор использует прежний UUID и неизменное тело:
            {{ pending.request_id }}.
          </p>
          <div class="actions">
            <PButton label="Повторить отправку" :loading="sending" @click="retry" /><PButton
              label="Убрать запрос"
              outlined
              :disabled="sending"
              @click="discard"
            />
          </div>
        </div>
        <div v-if="commandError" class="notice danger" role="alert">{{ commandError }}</div>
        <RouterView />
      </main>
    </div>
  </div>
</template>
