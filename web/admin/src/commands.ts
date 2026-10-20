import { ref } from 'vue'
import { api, APIError } from './api'
import type { Command, CommandRequest, Machine, Operation } from './types'

const key = 'iqhost.pending-command'
function readPending(): CommandRequest | null {
  try {
    return JSON.parse(sessionStorage.getItem(key) || 'null')
  } catch {
    return null
  }
}
export const pending = ref<CommandRequest | null>(readPending())
export const sending = ref(false)
export const commandError = ref('')
export const userID = ref(localStorage.getItem('iqhost.user') || 'unknown')
export function allowed(machine: Machine, kind: Command['kind']): boolean {
  if (['pending', 'running'].includes(machine.operation_status)) return false
  if (machine.recovery_required) return kind === 'cleanup'
  if (kind === 'cleanup') return false
  if (kind === 'create') return machine.state === 'not-created'
  if (kind === 'start') return machine.state === 'stopped'
  if (kind === 'stop') return machine.state === 'running'
  return ['running', 'stopped'].includes(machine.state)
}
export function prepare(command: Command): CommandRequest {
  return { request_id: crypto.randomUUID(), command: { ...command } }
}
export async function send(command?: Command): Promise<Operation | undefined> {
  if (sending.value) return
  commandError.value = ''
  try {
    if (command) {
      if (pending.value) throw new Error('Сначала разрешите незавершённую отправку')
      const request = prepare(command)
      // Без сохранения ключа отправку не начинаем: после потери ответа нужен тот же запрос.
      sessionStorage.setItem(key, JSON.stringify(request))
      pending.value = request
    }
    if (!pending.value) return
    sending.value = true
    const operation = await api<Operation>('/commands', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(pending.value),
    })
    sessionStorage.removeItem(key)
    pending.value = null
    return operation
  } catch (failure) {
    if (failure instanceof APIError && failure.status >= 400 && failure.status < 500) {
      sessionStorage.removeItem(key)
      pending.value = null
    }
    commandError.value = failure instanceof Error ? failure.message : 'Не удалось отправить команду'
  } finally {
    sending.value = false
  }
}
export function discard() {
  if (
    !confirm(
      'Ответ мог потеряться после принятия команды. Сначала проверьте операции VM. Убрать сохранённый запрос?',
    )
  )
    return
  sessionStorage.removeItem(key)
  pending.value = null
  commandError.value = ''
}
