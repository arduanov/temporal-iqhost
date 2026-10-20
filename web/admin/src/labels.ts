export const labels: Record<string, string> = {
  'not-created': 'Не создана',
  stopped: 'Остановлена',
  running: 'В работе',
  deleted: 'Удалена',
  unknown: 'Неизвестно',
  pending: 'Ожидает выполнения',
  succeeded: 'Завершена',
  failed: 'Ошибка',
  canceled: 'Отменена',
  'rollback-blocked': 'Откат заблокирован',
  create: 'Создание',
  start: 'Запуск',
  stop: 'Остановка',
  delete: 'Удаление',
  cleanup: 'Восстановление',
  forward: 'Выполнение',
  rollback: 'Откат',
}
export const label = (value: string) => labels[value] || value || '—'
export const date = (value: string) => (value ? new Date(value).toLocaleString('ru-RU') : '—')
export const vmLink = (id: string) => '/vm?vm_id=' + encodeURIComponent(id)
