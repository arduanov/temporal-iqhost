import { test, expect, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'

async function create(page: Page, id: string) {
  await page.goto('/vms')
  await page.getByRole('button', { name: 'Создать VM', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Создать и запустить' })).toBeDisabled()
  await page.getByRole('dialog').getByRole('textbox').fill(id)
  await page.getByRole('button', { name: 'Создать и запустить' }).click()
  await expect(page).toHaveURL(/\/operations\//)
}
async function completed(page: Page) {
  await expect(page.locator('.page-heading .badge')).toHaveText('Завершена')
}
async function machine(page: Page, id: string) {
  await page.goto('/vm?vm_id=' + encodeURIComponent(id))
}
async function reserves(page: Page, id: string) {
  const response = await page.request.get('/api/v1/resources')
  expect(response.ok()).toBeTruthy()
  const data = await response.json()
  return {
    compute: data.nodes.some((node: any) => node.vm_ids.includes(id)),
    disk: data.storage.some((pool: any) => pool.disks.some((disk: any) => disk.vm_id === id)),
    ip: data.ip_pools.some((pool: any) =>
      pool.addresses.some((address: any) => address.vm_id === id),
    ),
  }
}
test('жизненный цикл через формы и освобождение резервов', async ({ page }) => {
  const id = 'admin-e2e-' + Date.now()
  await create(page, id)
  await completed(page)
  expect(await reserves(page, id)).toEqual({ compute: true, disk: true, ip: true })
  await machine(page, id)
  await expect(page.getByRole('button', { name: 'Остановить', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: 'Остановить', exact: true }).click()
  await completed(page)
  expect(await reserves(page, id)).toEqual({ compute: false, disk: true, ip: true })
  await machine(page, id)
  await page.getByRole('button', { name: 'Запустить', exact: true }).click()
  await completed(page)
  await machine(page, id)
  page.once('dialog', async (dialog) => {
    expect(dialog.message()).toContain(id)
    expect(dialog.message()).toContain('диск и IP')
    await dialog.dismiss()
  })
  await page.getByRole('button', { name: 'Удалить', exact: true }).click()
  await expect(page).toHaveURL(/\/vm\?/)
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Удалить', exact: true }).click()
  await completed(page)
  expect(await reserves(page, id)).toEqual({ compute: false, disk: false, ip: false })
  await machine(page, id)
  await expect(page.locator('.details .badge')).toHaveText('Удалена')
  await page.goto('/resources')
  await expect(page.getByRole('heading', { name: 'IP-пулы' })).toBeVisible()
})

test('потеря ответа и повтор после перезагрузки с тем же UUID', async ({ page }) => {
  const id = 'admin-retry-' + Date.now()
  const bodies: string[] = []
  await page.route('**/api/v1/commands', async (route) => {
    bodies.push(route.request().postData()!)
    if (bodies.length === 1) {
      await route.fetch()
      await route.abort('connectionfailed')
    } else await route.continue()
  })
  await page.goto('/vms')
  await page.getByRole('button', { name: 'Создать VM', exact: true }).click()
  await page.getByRole('dialog').getByRole('textbox').fill(id)
  await page.getByRole('button', { name: 'Создать и запустить' }).click()
  await expect(page.getByRole('button', { name: 'Повторить отправку' })).toBeEnabled()
  await page.reload()
  await page.getByRole('button', { name: 'Повторить отправку' }).click()
  await expect(page).toHaveURL(/\/operations\//)
  expect(bodies[0]).toBe(bodies[1])
  await completed(page)
  await machine(page, id)
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Удалить', exact: true }).click()
  await completed(page)
})

// Эти проверки временно останавливают dispatcher только в явно выбранном демо.
const faults = process.env.ADMIN_FAULT_TESTS === '1'
function compose(...args: string[]) {
  return execFileSync('docker', ['compose', ...args], {
    cwd: '../..',
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  })
}
test('ожидание outbox, Terminate, reconciler и cleanup', async ({ page }) => {
  test.skip(!faults, 'ADMIN_FAULT_TESTS=1 разрешает остановку outbox в локальном демо')
  const id = 'admin-fault-' + Date.now()
  compose('stop', 'outbox')
  try {
    await create(page, id)
    await expect(page.getByText('Ожидает доставки в Temporal', { exact: true })).toBeVisible()
    await machine(page, id)
    await expect(page.getByRole('button', { name: 'Запустить', exact: true })).toBeDisabled()
  } finally {
    compose('start', 'outbox')
  }
  const response = await page.request.get('/api/v1/vm?vm_id=' + encodeURIComponent(id))
  const current = await response.json()
  await page.goto('/operations/' + current.operation_id)
  await expect
    .poll(async () => {
      const response = await page.request.get('/api/v1/operations/' + current.operation_id)
      return (await response.json()).run_id
    })
    .not.toBe('')
  const operation = await (
    await page.request.get('/api/v1/operations/' + current.operation_id)
  ).json()
  compose(
    'exec',
    '-T',
    'temporal',
    'temporal',
    'workflow',
    'terminate',
    '--address',
    'temporal:7233',
    '--workflow-id',
    operation.workflow_id,
    '--run-id',
    operation.run_id,
    '--reason',
    'admin-e2e',
  )
  await expect(page.getByText('Откат заблокирован — ресурсы удерживаются')).toBeVisible({
    timeout: 90000,
  })
  await page.getByRole('link', { name: 'Перейти к восстановлению VM →' }).click()
  await expect(page.getByRole('button', { name: 'Восстановить', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Удалить', exact: true })).toBeDisabled()
  page.once('dialog', async (dialog) => {
    expect(dialog.message()).toContain('Не создана')
    await dialog.accept()
  })
  await page.getByRole('button', { name: 'Восстановить', exact: true }).click()
  await completed(page)
  await expect(page.getByRole('link', { name: 'Открыть →' })).toBeVisible()
  expect(await reserves(page, id)).toEqual({ compute: false, disk: false, ip: false })
  await machine(page, id)
  await page.getByRole('button', { name: 'Повторить создание' }).click()
  await completed(page)
  await machine(page, id)
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Удалить', exact: true }).click()
  await completed(page)
})

test('ошибка опроса сохраняет данные, скрытая вкладка приостанавливает запросы', async ({
  page,
}) => {
  let requests = 0
  await page.route('**/api/v1/vms?*', async (route) => {
    requests++
    if (requests === 1) {
      await route.fulfill({
        json: {
          items: [
            {
              id: 'stale-demo',
              cpu: 4,
              memory_mib: 8192,
              disk_gib: 20,
              gpu: false,
              state: 'stopped',
              recovery_required: false,
              operation_id: '',
              operation_status: '',
              recovery_target: '',
              node: '',
              ip: '',
              gpu_id: 0,
              disk_id: 0,
            },
          ],
          limit: 50,
          offset: 0,
        },
      })
    } else {
      await route.fulfill({ status: 503, json: { error: 'Временная ошибка' } })
    }
  })
  await page.goto('/vms')
  await expect(page.getByRole('link', { name: 'stale-demo' })).toBeVisible()
  await expect(page.getByRole('status')).toContainText('Показаны данные на')
  await expect(page.getByRole('link', { name: 'stale-demo' })).toBeVisible()
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  const paused = requests
  await page.waitForTimeout(2500)
  expect(requests).toBe(paused)
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, value: false })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await expect.poll(() => requests).toBeGreaterThan(paused)
})
