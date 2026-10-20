import { onUnmounted, ref, shallowRef, watch, type Ref } from 'vue'
import { api } from './api'

// Следующий опрос начинается только после завершения предыдущего. При смене
// фильтров запоздалый ответ не заменяет данные нового запроса.
export function usePolling<T>(path: Ref<string>) {
  const data = shallowRef<T>()
  const error = ref('')
  const updatedAt = ref('')
  const loading = ref(false)
  let stopped = false
  let timer: ReturnType<typeof setTimeout> | undefined
  let again = false
  const controller = new AbortController()
  async function refresh() {
    clearTimeout(timer)
    if (stopped || document.hidden) return
    if (loading.value) {
      again = true
      return
    }
    loading.value = true
    const currentPath = path.value
    try {
      const result = await api<T>(currentPath, {
        signal: AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
      })
      if (!stopped && path.value === currentPath) {
        data.value = result
        error.value = ''
        updatedAt.value = new Date().toLocaleTimeString('ru-RU')
      }
    } catch (failure) {
      if (!stopped && path.value === currentPath)
        error.value = failure instanceof Error ? failure.message : 'Ошибка обновления'
    } finally {
      loading.value = false
      if (!stopped) {
        timer = setTimeout(refresh, again ? 0 : 2000)
        again = false
      }
    }
  }
  watch(
    path,
    () => {
      data.value = undefined
      updatedAt.value = ''
      error.value = ''
      void refresh()
    },
    { immediate: true },
  )
  function visibility() {
    if (!document.hidden) void refresh()
    else clearTimeout(timer)
  }
  document.addEventListener('visibilitychange', visibility)
  onUnmounted(() => {
    stopped = true
    clearTimeout(timer)
    controller.abort()
    document.removeEventListener('visibilitychange', visibility)
  })
  return { data, error, updatedAt, loading, refresh }
}
