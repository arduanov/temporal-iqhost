import { beforeEach, expect, test, vi } from 'vitest'
import type { Machine, Command } from './types'

const storage = new Map<string, string>()
vi.stubGlobal('sessionStorage', {
  getItem: (key: string) => storage.get(key) || null,
  setItem: (key: string, value: string) => storage.set(key, value),
  removeItem: (key: string) => storage.delete(key),
})
vi.stubGlobal('localStorage', { getItem: () => null })
const { allowed, send, pending, userID } = await import('./commands')
const machine = {
  state: 'stopped',
  operation_status: 'succeeded',
  recovery_required: false,
} as Machine
const command: Command = { vm_id: 'vm', user_id: 'tester', kind: 'start' }
beforeEach(() => {
  storage.clear()
  pending.value = null
  vi.unstubAllEnvs()
})
test('допустимые кнопки учитывают активную операцию и восстановление', () => {
  expect(allowed(machine, 'start')).toBe(true)
  expect(allowed(machine, 'stop')).toBe(false)
  for (const kind of ['create', 'start', 'stop', 'delete', 'cleanup'] as const)
    expect(allowed({ ...machine, operation_status: 'pending' }, kind)).toBe(false)
  expect(allowed({ ...machine, recovery_required: true }, 'start')).toBe(false)
  expect(allowed({ ...machine, recovery_required: true }, 'cleanup')).toBe(true)
  expect(allowed({ ...machine, state: 'not-created' }, 'create')).toBe(true)
})
test('потерянный ответ сохраняет UUID и неизменное тело для повтора', async () => {
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new Error('network lost'))
    .mockResolvedValueOnce(new Response(JSON.stringify({ id: 'operation' })))
  vi.stubGlobal('fetch', fetch)
  await send(command)
  expect(pending.value?.request_id).toBeTruthy()
  const saved = storage.get('iqhost.pending-command')
  userID.value = 'changed-user'
  await send()
  expect(fetch.mock.calls[1][1].body).toBe(fetch.mock.calls[0][1].body)
  expect(fetch.mock.calls[0][1].body).toBe(saved)
  expect(pending.value).toBeNull()
})
test('новая команда не перезаписывает неопределённую отправку', async () => {
  const fetch = vi.fn().mockRejectedValue(new Error('network lost'))
  vi.stubGlobal('fetch', fetch)
  await send(command)
  const id = pending.value?.request_id
  await send({ ...command, kind: 'stop' })
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(pending.value?.request_id).toBe(id)
})
test('отказ сервера разрешает создать новую команду с новым ключом', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify({ error: 'conflict' }), { status: 409 }))
  vi.stubGlobal('fetch', fetch)
  await send(command)
  expect(pending.value).toBeNull()
  await send(command)
  expect(JSON.parse(fetch.mock.calls[0][1].body).request_id).not.toBe(
    JSON.parse(fetch.mock.calls[1][1].body).request_id,
  )
})
