# Жизненный цикл VM: PostgreSQL и Temporal

Запускаемое демо облачного control plane на Go. PostgreSQL хранит VM, ресурсы,
операции, журнал шагов и outbox; Temporal выполняет create/start/stop/delete/cleanup.
Инфраструктура симулируется отдельным HTTP-сервисом, настоящие VM не создаются.

[Продуктовые возможности, выбор Temporal, outbox и гарантии reconciler](PRODUCT_AND_ARCHITECTURE.md).

## Демо для показа

[Пошаговая инструкция и сценарий показа](../README.md) ·
[Видеодемонстрация админки](../artifacts/demo/iqhost-admin-demo.mp4).

Нужен только запущенный Docker с Compose v2. Из корня репозитория выполните:

```sh
docker compose up -d --build
```

После завершения команды откройте **http://localhost:8082** — админка уже заполнена.
Compose сам создаст базы, выполнит миграции, добавит инвентарь, настроит Temporal
и дождётся выполнения демонстрационных операций. Установка Go, Node.js,
ручной запуск seed и команды создания VM не нужны. Первая сборка требует интернета.
Для готового демо используется отдельный volume `demo-postgres-data`;
старый volume `postgres-data` не подключается и не удаляется.

| VM | Что показывать |
| --- | --- |
| `demo-gpu` | Работающая VM: 4 CPU, 8 GiB RAM, GPU, диск 40 GiB и выделенный IP. |
| `demo-stopped` | Остановленная VM: вычислительные ресурсы освобождены, диск 20 GiB и IP сохранены. Можно запустить из админки. |
| `demo-deleted` | Удалённая VM и история создания/удаления; ресурсы освобождены. |
| `demo-no-capacity` | Неуспешное создание с запросом 100 CPU: ошибка нехватки ресурсов и откат. |

Во вкладках админки доступны операции, их шаги, журнал команд провайдера и ресурсы.
В **http://localhost:8080** доступны шесть настоящих workflow этих операций.
Все демо-команды подписаны `demo-customer`.

Одноразовый сервис `demo` создаёт данные через обычный приём команд и workflow.
Он использует постоянные ID запросов: повторный запуск не создаёт дубли и не
сбрасывает изменения, сделанные во время показа. Прерванная подготовка продолжится
при следующем запуске. Не меняйте `FAIL_AT` во время первой подготовки.
Если подготовка не завершилась, причина видна в `docker compose logs demo worker outbox`.

Остановка через `docker compose down` сохраняет БД, историю Temporal и состояние
симулятора в volumes. Для следующего показа достаточно той же команды запуска.
Чтобы начать показ заново, удалите данные командой `docker compose down -v`, затем
снова запустите Compose. **Это удалит также все изменения, сделанные в демо.**

## С чего начать чтение кода

Для чтения достаточно основ Go: функций, структур, интерфейсов и обработки ошибок.
Сначала проследите одну команду `create`, затем переходите к повторам и очистке.

| Порядок | Где читать | Что выяснить |
| --- | --- | --- |
| 1 | [Модели VM](../internal/vm/types.go) | `Command` — запрос пользователя, `Operation` — ход его выполнения; состояние VM и статус операции различаются. |
| 2 | [Команды CLI](../cmd/iqhost/commands.go), `submitCommand` | Как параметры становятся командой; `waitForOperation` ждёт результат в БД. Параметры описаны в [options.go](../cmd/iqhost/options.go). |
| 3 | [Приём операции](../internal/postgres/operations.go), `Accept` | Как в одной транзакции создаются операция, право владения VM и запись outbox. |
| 4 | [Доставка](../internal/outbox/dispatcher.go), `Once` | Как сохранённая команда запускает workflow и почему повтор доставки безопасен. |
| 5 | [Сценарий](../internal/orchestration/lifecycle.go), `Lifecycle` | Порядок шагов, завершение и переход к `rollbackOperation` при ошибке. |
| 6 | [Activities](../internal/orchestration/activities/service.go), `Step`; [выполнение](../internal/execution/service.go) | Activities добавляют контекст Temporal и классифицируют ошибки; execution выполняет локальные шаги и команды провайдера. |
| 7 | [Локальные шаги](../internal/postgres/steps.go), `LocalStep`; [ресурсы](../internal/postgres/resources.go), `allocateResources` | Как проверяется владение, сохраняется результат шага и выбираются ресурсы. |
| 8 | [Команды провайдера](../internal/postgres/commands.go), `Prepare` и `Commit` | Почему запрос сохраняется до внешнего эффекта, а результат проверяется после него. |
| 9 | [Симулятор](../internal/provider/simulator/simulator.go), `run` | Как карты под mutex хранят фактическое состояние, поколения и результаты команд; задержки выполняются без блокировки. |

[worker.go](../cmd/iqhost-worker/worker.go) соединяет эти части: создаёт HTTP-клиент симулятора,
сервис execution и обёртку Activities, регистрирует workflow.
[Настройка поиска](../internal/visibility/setup.go) регистрирует поля Temporal UI до запуска outbox.
[Подготовка демо](../cmd/iqhost/demo.go) выполняет шесть обычных команд до запуска админки;
[снимок симулятора](../internal/provider/simulator/persistence.go) сохраняет его независимое
состояние, поколения Fence и ответы команд между перезапусками Docker.
[Outbox](../cmd/iqhost-outbox/main.go) и [reconciler](../cmd/iqhost-reconciler/main.go)
открывают собственные подключения и запускаются отдельными процессами.
[operation_outcome.go](../internal/postgres/operation_outcome.go) проверяет итоговое
состояние перед завершением операции, а [transactions.go](../internal/postgres/transactions.go)
содержит правила повторения транзакций после конфликтов PostgreSQL.
SQL-запросы находятся в `queries/`; `dbgen/` генерируется sqlc и вручную не меняется.

### Успешное создание VM

`submitCommand` передаёт команду в `Accept`. После фиксации транзакции dispatcher
запускает `Lifecycle`. Шаг начала проверяет владельца и через Fence/Inspect получает
состояние провайдера. Далее `operationSteps` задаёт последовательность:

```text
reserve → create-instance → attach-network → start-instance → wait-ready → finish
```

`allocateResources` резервирует compute/GPU, диск и IP в одной транзакции.
Для каждого внешнего шага `executeProviderCommand` подтверждает Fence, сохраняет
неизменяемый запрос через `Prepare`, вызывает провайдера и записывает ответ через
`Commit`. Во время вызова провайдера транзакция приложения уже закрыта.
`finishLocalOperation` проверяет наличие работающего экземпляра и всех резервов,
затем сохраняет статус операции `succeeded` и состояние VM `running`.

### Ошибка и откат

Допустим, после `create-instance` шаг `attach-network` завершился постоянной ошибкой.
`Lifecycle` вызывает `rollbackOperation`: сначала `begin-rollback`, затем stop,
detach-network, delete-instance и release. Начало отката повышает поколение владения,
а Fence запрещает старым командам менять VM. Очистка выполняется в контексте,
который продолжает работать даже после отмены исходной операции.

Ресурсы освобождаются только после подтверждения отсутствия экземпляра текущего
поколения. После успешного отката create операция получает `failed`, а VM возвращается
в `not-created`. При ошибке самой очистки операция получает `rollback-blocked`, VM —
`recovery_required`; оставшиеся резервы сохраняются. Отдельная команда `cleanup`
продолжит восстановление. Для start/stop цель восстановления — `stopped`, поэтому
диск и IP остаются закреплены за VM.

### Термины, которые нужны для понимания

| Термин | Что означает в этом проекте |
| --- | --- |
| Workflow | Сценарий, ход которого Temporal записывает в историю. При восстановлении он воспроизводится с ранее полученными результатами Activities. |
| Activity | Шаг с внешним эффектом: работа с БД или провайдером. Temporal может повторить его, если результат не был получен. |
| Идемпотентность | Повтор той же команды возвращает прежний результат. Например, после создания экземпляра ответ потерялся: повтор с тем же ключом не создаёт второй экземпляр. |
| Outbox | Запись о необходимости запустить workflow, сохранённая в той же транзакции, что и операция. Dispatcher доставит её после восстановления связи с Temporal. |
| Lease и token | Временное право dispatcher обрабатывать запись. После истечения срока её может забрать другой worker; token защищает от подтверждения старым обработчиком. |
| Generation и Fence | Generation — номер поколения владения VM. Fence — подтверждение провайдера, что команды старых поколений больше не вызовут эффектов. Commit также отвергает запоздалые ответы. |
| Фактическое и подтверждённое состояние | Первое находится в памяти сервиса симулятора, второе — в БД приложения. Между внешним эффектом и Commit они могут различаться, например при потерянном ответе. |
| Reconciler | Проверяет незавершённые операции по состоянию Temporal. При расхождении требует восстановления и сохраняет резервы; сам ресурсы не освобождает. |

## Запуск через Docker Compose

Нужен Docker с Compose v2. Образы закреплены: PostgreSQL 16.8, Temporal 1.26.2,
UI 2.34.0; сборка использует Go 1.27.2. Проверены на Docker ARM64.

```sh
docker compose up -d --build
```

PostgreSQL инициализирует отдельные базы и пользователей приложения и Temporal.
Temporal auto-setup готовит свои persistence/visibility-схемы и namespace default.
Одноразовые сервисы `migrate` и `seed` завершаются перед стартом worker.
Worker также ожидает успешный healthcheck симулятора.

- Админка с готовыми данными: http://localhost:8082
- Temporal UI: http://localhost:8080
- Temporal API: localhost:7233
- PostgreSQL: localhost:5432, база/пользователь `iqhost`, пароль `iqhost-local`.
- Симулятор: http://localhost:8081, проверка доступности — `GET /healthz`.

Пароли в Compose (`iqhost-local`, `temporal-local`, `postgres-local`) предназначены
только для локального демо. Порты опубликованы на 127.0.0.1.
Seed создаёт две ноды по 8 CPU / 16384 MiB RAM с одним GPU на каждой,
общий storage 200 GiB и IPv4 192.0.2.10–192.0.2.19.
Повтор seed сохраняет назначения и изменения существующего инвентаря.

## CLI

```sh
docker compose run --rm cli create -id vm-001 -cpu 4 -memory 8192 -disk 20 -gpu=true -wait
docker compose run --rm cli get -id vm-001
docker compose run --rm cli list
docker compose run --rm cli stop -id vm-001 -wait
docker compose run --rm cli start -id vm-001 -wait
docker compose run --rm cli delete -id vm-001 -wait
```

Create сразу запускает VM. Stop освобождает CPU/RAM/GPU, сохраняя диск и IP.
Start может разместить VM на другой ноде. Delete сам останавливает VM и
освобождает все ресурсы; запись и ID удалённой VM остаются в БД.
По умолчанию create запрашивает 4 CPU, 8192 MiB RAM, диск 20 GiB, без GPU.
Start/stop/delete/cleanup читают конфигурацию из БД и не принимают ресурсные флаги.

Без `-wait` команда возвращает принятую операцию. `-wait` опрашивает PostgreSQL
до результата; Ctrl-C прекращает ожидание, сохраняя принятую операцию.
Неуспешный терминальный результат даёт код выхода 1 и JSON операции в stdout.
Диагностика CLI идёт в stderr.

Каждая изменяющая команда принимает `-request-id UUID`. Если флаг отсутствует,
UUID выводится в stderr **до отправки**. Повтор с тем же UUID и тем же содержимым
возвращает прежнюю операцию; другое содержимое вызывает конфликт.

```sh
docker compose run --rm cli create -id vm-002 -request-id 74e5e489-2820-4e91-bc41-8afcdfe2cf5c -wait
docker compose run --rm cli operation -id 74e5e489-2820-4e91-bc41-8afcdfe2cf5c
```

Повторные start для running, stop для stopped и delete для deleted — успешные
no-op операции. После полностью откатившегося create разрешён новый create
с тем же VM ID и конфигурацией, но новым request ID. При `recovery_required`
разрешён только cleanup. Одновременно у VM может быть одна активная операция.

Чтобы принять команду при остановленном worker, не запускайте зависимости CLI:

```sh
docker compose stop worker
docker compose run --rm --no-deps cli create -id queued-vm
docker compose up -d worker
```

Outbox доставит команду в Temporal даже без worker. Workflow будет ждать
исполнителя. Если остановить `outbox`, команда останется в БД и будет доставлена
после его запуска. Остановка `reconciler` не мешает штатным операциям, но откладывает
обнаружение прерванных workflow.

## Отказы и восстановление

Нехватка ресурсов завершается сразу после проверки ёмкости, без ожидания их
освобождения. Например, запрос 100 CPU завершится `failed` с VM `not-created`:

```sh
docker compose run --rm cli create -id too-large -cpu 100 -wait
```

Ошибка проверки готовности приводит к полному откату create:

```sh
FAIL_AT=wait-ready docker compose up -d --no-deps worker
docker compose run --rm cli create -id failed-vm -wait
FAIL_AT= docker compose up -d --no-deps worker
```

`FAIL_AT` поддерживает шаги `begin`, `reserve`, `create`, `attach-network`,
`start`, `wait-ready`, `stop`, `detach-network`, `delete-instance`, `release`,
`begin-rollback`, `finish`. Обычно ошибка постоянная и non-retryable.
Префикс `retry:`, например `retry:wait-ready`, включает временную ошибку для
демонстрации повторов и terminate. Временные ошибки Activities повторяет Temporal.

Сбой остановки при delete оставляет ресурс занятым и требует cleanup:

```sh
docker compose run --rm cli create -id blocked-vm -wait
FAIL_AT=stop docker compose up -d --no-deps worker
docker compose run --rm cli delete -id blocked-vm -wait
docker compose run --rm cli get -id blocked-vm
FAIL_AT= docker compose up -d --no-deps worker
docker compose run --rm cli cleanup -id blocked-vm -wait
```

Cleanup принимает только ID VM. Цель и исходную операцию он читает из БД:
после ошибочного create → not-created, start/stop → stopped, delete → deleted.
Диск и IP существовавшей VM при восстановлении start/stop сохраняются.
Cleanup атомарно получает владение VM; Activities старой операции отклоняются.

Terminate/общий workflow timeout обходят код компенсации. Reconciler каждые
10 секунд обнаруживает завершившийся workflow с незавершённой операцией,
блокирует её и выставляет `recovery_required`, сохраняя ресурсы до явного cleanup.
Отсутствующая история тоже требует восстановления. Недоступность Temporal
сама по себе не меняет VM и не освобождает ресурсы.

## Хранение и гарантии

```text
CLI → PostgreSQL: VM + operation + outbox одной транзакцией
    → dispatcher: lease → Temporal workflow
    → Activities: подготовка команды → провайдер вне транзакции → фиксация результата
```

- Прикладные запросы находятся в `internal/postgres/queries`, код sqlc — в `dbgen`.
- Goose v3 применяет embedded SQL-миграции под PostgreSQL session advisory lock.
- Каждый шаг проверяет владельца до возврата сохранённого результата.
- Новая операция и переход к rollback повышают поколение владения. Провайдер
  подтверждает Fence перед командами поколения; поздний ответ не проходит фиксацию.
  Повтор после потерянного ответа использует сохранённый неизменяемый запрос.
- Ресурсы create выделяются атомарно; отказ по GPU/storage/IP откатывает всё.
- Порядок блокировок: VM → операция → ноды → GPU → storage → IP. Маленький
  демо-инвентарь блокируется целиком по ID; это сознательное ограничение пропускной
  способности. Ёмкость проверяется после блокировок, без SKIP LOCKED.
- Outbox использует `FOR UPDATE SKIP LOCKED`, lease 30 секунд, timeout обращения
  к Temporal 10 секунд, backoff с jitter до минуты. Один обработчик захватывает
  одну запись. Обновления защищены lease token. Несколько workers допустимы.
- Workflow ID `vm-operation/<kind>/<escaped-vm-id>/<operation-id>` запрещено переиспользовать. При
  AlreadyStarted диспетчер сверяет тип workflow и operation_id в memo.
- Завершённая операция не доставляется повторно, даже если история Temporal удалена.
- Подтверждённое состояние VM отделено от статуса/фазы/шага операции.
  `current_operation_id` сохраняет ссылку на последнюю операцию для диагностики;
  право изменять VM есть только у текущей pending/running операции.

Старый in-memory demo удалён. CLI и Compose используют PostgreSQL и `orchestration.Lifecycle`.
Совместимость со старыми workflow histories не предусмотрена.

## Пользователи и операции в Temporal UI

Инициатор команды задаётся флагом `--user`, без флага используется `unknown`.
Пробелы по краям имени убираются; максимум — 256 байт UTF-8. Это метка автора
для демо, без аутентификации, владения VM и проверки прав. Разные пользователи
могут отправлять команды одной VM; ограничение одной активной операции сохраняется.

```sh
docker compose run --rm cli create --id vm-alice --user alice --wait
docker compose run --rm cli create --id vm-bob --user bob --wait
docker compose run --rm cli stop --id vm-alice --user alice --wait
```

Workflow ID имеет вид `vm-operation/stop/vm-alice/<uuid>`: операция и VM видны
в списке сразу. Специальные символы VM ID экранируются; длина полного ID ограничена
1000 байтами. Пользователь также возвращается в JSON операции и записывается
в логи принятия и доставки. Повтор с тем же `--request-id` должен содержать
того же автора и ту же команду, иначе возвращается конфликт.

В [Temporal UI](http://localhost:8080) откройте namespace `default`.
В карточке workflow доступны Search Attributes `OperationKind`, `VMId`, `UserId`
и Memo с `operation_id`, `operation_kind`, `vm_id`, `user_id`.
Search Attributes — индексируемые поля для фильтрации списка, например:

```sql
UserId = 'alice' AND OperationKind = 'stop'
```

Другой пример: `VMId = 'vm-alice'`. Индекс обновляется асинхронно, поэтому
новая операция может появиться в поиске с небольшой задержкой.

Compose автоматически запускает `temporal-setup` перед outbox. При локальном
запуске выполните `go run ./cmd/iqhost temporal-setup` один раз после запуска
Temporal. Команда использует `--address` / `TEMPORAL_ADDRESS` и namespace
`default`; повторный запуск безопасен, несовместимый тип существующего поля
приводит к явной ошибке.

Версия с авторами рассчитана на чистую БД: изменена начальная схема.
Готовое демо использует новый volume `demo-postgres-data`; данные и истории
из старого `postgres-data` не переносятся и остаются в прежнем volume.

## Локальная админка

После `docker compose up -d --build` откройте **http://localhost:8082**.
Сервис `admin` публикуется только на `127.0.0.1:8082`. В Compose он ждёт PostgreSQL,
миграции, seed и завершение подготовки демо. Сам HTTP-сервер обращается только к БД:
после запуска команда может быть принята, пока исполнители выключены.

Vue 3 + TypeScript + PrimeVue показывают VM, историю операций и ресурсы.
Создание, запуск, остановка, удаление и восстановление доступны через формы.
Инициатор сохраняется в браузере, по умолчанию `unknown`; это подпись команды,
без аутентификации и проверки прав. Админка предназначена для локального демо.

Различайте три вида данных:

- **Состояние VM** — подтверждённое состояние из прикладной БД, не ответ провайдера
  в реальном времени. Принятие команды не означает завершение действия.
- **Операция** — доставка через outbox, выполнение или откат, последний сохранённый
  шаг и завершённые шаги. `rollback-blocked` требует отдельного восстановления;
  cleanup использует сохранённую цель и ссылку на исходную операцию.
- **Ресурсы** — действующие резервы, не фактическая загрузка CPU/RAM.
  Stop освобождает compute, сохраняя диск и IP; delete освобождает всё после
  подтверждения удаления. Недоступные ноды/хранилища остаются недоступными даже
  при свободной ёмкости.

Списки обновляются каждые 2 секунды без пересечения запросов; скрытая вкладка
приостанавливает опрос. При ошибке остаются последние данные и время обновления.
UUID и тело команды сохраняются в `sessionStorage` до отправки. После потери
ответа используйте «Повторить отправку»: повтор с тем же ключом возвращает прежнюю
операцию. Перед удалением сохранённого запроса проверьте историю VM.
Подробные попытки Activities доступны по ссылке в Temporal UI на порту 8080.

Маршрут чтения: [запуск сервера](../cmd/iqhost-admin/main.go) →
[HTTP API](../internal/admin/server.go), [обработчики команд](../internal/admin/commands.go),
[чтение](../internal/admin/reads.go) и [DTO](../internal/admin/dto.go) →
[приём команды](../internal/postgres/operations.go).
[Запросы чтения sqlc](../internal/postgres/queries/admin.sql) и
[согласованные снимки](../internal/postgres/admin.go) не блокируют размещение ресурсов.
Frontend начинается с [App.vue](../web/admin/src/App.vue), страницы — в
`web/admin/src/pages`, опрос — в `polling.ts`, повторы команд — в `commands.ts`.
Существующий JSON CLI, схема БД и workflow не меняются.

HTTP API: `GET /api/v1/vms`, `GET /api/v1/vm?vm_id=…`,
`GET /api/v1/operations`, `GET /api/v1/operations/{uuid}`,
`GET /api/v1/resources`, `POST /api/v1/commands`, `GET /healthz`.
Для списков `limit` — 1–200 (по умолчанию 50), `offset` — от 0.
VM фильтруются по `search`, `state`, `recovery_required`; операции — по точным
`vm_id`, `status`, `user_id`. Ответ списка: `{items, limit, offset}`.
Команда: `{request_id, command}`, где `command` имеет формат `vm.Command`.
Успех возвращает 202 и операцию; ошибки — 400, 404, 409, внутренние — 500
без SQL/DSN. Изменяющие запросы требуют JSON и допустимого Origin.

Разработка (Node.js 22.12+):

```sh
npm ci --prefix web/admin
# В двух терминалах:
go run ./cmd/iqhost-admin --dev-origin http://127.0.0.1:5173
npm run dev --prefix web/admin
# Открыть http://127.0.0.1:5173; Vite проксирует /api на Go :8082.
# Статическая сборка, доступная на :8082:
npm run build --prefix web/admin
```

Проверки админки:

```sh
npm run test --prefix web/admin
npm run build --prefix web/admin
# Интеграционные Go-тесты входят в make integration (нужна отдельная тестовая БД).
cd web/admin
npx playwright install chromium
npm run test:e2e
# Только в локальном демо: временно остановить outbox, Terminate тестового
# workflow, дождаться reconciler, выполнить cleanup и повтор создания.
ADMIN_FAULT_TESTS=1 npm run test:e2e
```

Playwright использует запущенный сервис на `http://127.0.0.1:8082`; другой адрес
задаётся через `ADMIN_URL`. Сценарии создают VM с префиксом `admin-`, удаляют их
после успешной проверки и оставляют историю операций для диагностики.

## Диагностика

```sh
docker compose logs -f worker
docker compose logs -f temporal
docker compose logs --no-log-prefix worker | rg '74e5e489-2820-4e91-bc41-8afcdfe2cf5c'
```

Приложение использует JSON `slog`: time, level, msg, component, event и доступные
vm_id/operation_id/workflow_id/run_id/step/attempt/duration_ms.
`LOG_LEVEL` принимает DEBUG/INFO/WARN/ERROR, по умолчанию INFO.
Логи worker, outbox и reconciler идут в stdout; Temporal SDK подключён к slog, workflow-логи при replay
отключены стандартной настройкой SDK. SQL-параметры и DSN не логируются.
Docker-логи ротируются: 10 MiB × 3 файла на контейнер.
Откройте workflow_id из ответа CLI в Temporal UI для просмотра шагов и повторов.

```sh
docker compose exec postgres psql -U iqhost -d iqhost -c 'SELECT id,state,recovery_required,current_operation_id FROM vms;'
docker compose exec postgres psql -U iqhost -d iqhost -c 'SELECT id,vm_id,kind,status,phase,step,error FROM vm_operations ORDER BY created_at;'
docker compose exec postgres psql -U iqhost -d iqhost -c 'SELECT * FROM compute_allocations WHERE released_at IS NULL;'
docker compose exec postgres psql -U iqhost -d iqhost -c 'SELECT * FROM vm_disks WHERE deleted_at IS NULL;'
docker compose exec postgres psql -U iqhost -d iqhost -c 'SELECT host(address),vm_id FROM ip_addresses ORDER BY address;'
```

## Локальная разработка и проверки

Для Go CLI при поднятом Compose достаточно настроек по умолчанию. Переменные:
`DATABASE_URL`, `TEMPORAL_ADDRESS`, `LOG_LEVEL`; фоновые бинарники принимают `--address`.
Worker дополнительно использует `SIMULATOR_URL` (по умолчанию `http://localhost:8081`),
`FAIL_AT` и `--fail-at`. В Compose URL равен `http://simulator:8081`.
Симулятор принимает `--listen` (по умолчанию `127.0.0.1:8081`).

```sh
go run ./cmd/iqhost migrate
go run ./cmd/iqhost seed
go run ./cmd/iqhost temporal-setup
go run ./cmd/iqhost get -id vm-001
# Каждый процесс запускайте в отдельном терминале вместо соответствующего Compose-сервиса:
go run ./cmd/iqhost-simulator
go run ./cmd/iqhost-worker
go run ./cmd/iqhost-outbox
go run ./cmd/iqhost-reconciler
# Только явный откат одной версии схемы:
go run ./cmd/iqhost migrate down
```

Не запускайте локальный worker с инъекцией ошибок одновременно с обычным
Compose-worker: задания может забрать любой из них.

```sh
make tools                 # sqlc v1.31.1 и golangci-lint v2.15.0 в .tools/
make generate
make vet-sql
# Повторная генерация не должна менять сохранённый dbgen:
git diff --exit-code -- internal/postgres/dbgen
go test -race ./...
make lint                  # запускает golangci-lint из .tools/
make build
docker compose config --quiet
```

Интеграционные тесты создают уникальные схемы в явно указанной тестовой БД приложения
и удаляют только свои схемы. Каждый пул и миграции используют фиксированный
`search_path`, поэтому тестовые пакеты можно запускать параллельно. Не используйте
рабочие БД. Интеграционные пакеты в `make integration` запускаются последовательно
(`-p 1`), чтобы не исчерпать лимит соединений локального PostgreSQL. Тесты Activities и хранилища требуют
`TEST_DATABASE_URL` и используют `httptest` с нулевыми задержками симулятора.
Без этой переменной интеграционные тесты пропускаются. Тесты симулятора не требуют БД.
Temporal-сервер для этих тестов не нужен.
Сценарии взаимодействия Activities с хранилищем и провайдером находятся в
`internal/orchestration/activities/integration_test.go` (пакет `activities_test`).
Тесты хранилища остаются в `internal/postgres`, симулятора — в
`internal/provider/simulator`.

```sh
docker compose exec postgres psql -U postgres -c 'CREATE DATABASE iqhost_test OWNER iqhost;'
export TEST_DATABASE_URL='postgres://iqhost:iqhost-local@localhost:5432/iqhost_test?sslmode=disable'
make integration
go test -race ./...
# Проверка симулятора без прикладной БД и Temporal:
env -u TEST_DATABASE_URL go test -race ./internal/provider/simulator -count=1
```

Если PostgreSQL сообщает `remaining connection slots are reserved` (`53300`),
запустите `GOFLAGS="-p=1" make integration`: пакеты будут проверяться последовательно,
а конкурентные сценарии внутри тестов сохранятся.

Тесты проверяют миграции/seed, идемпотентность, ограничения ёмкости, конкурентные
резервы, ожидание блокировки, lease, fencing, цели cleanup и сохранение ресурсов.
Temporal testsuite покрывает успех, ошибки шагов, отмену и rollback-blocked;
mock-тесты — повторную доставку и сверку закрывшихся/отсутствующих workflow.

```sh
python3 scripts/smoke.py
```

Smoke-тест предназначен для отдельного демо: он перезапускает сервисы и меняет
FAIL_AT worker. Проверяет жизненный цикл, сохранность при перезапуске worker
с работающим симулятором,
отложенную доставку, нехватку ресурсов, откат, cleanup и terminate/reconciler.
Созданные диагностические записи с префиксом `smoke-` остаются в БД.

Replay завершённой истории Lifecycle и отсутствие повторных workflow-логов:

```sh
TEST_WORKFLOW_ID='vm-operation/create/vm-001/<operation-id>' go test ./internal/orchestration -run TestLifecycleReplay -count=1
```

При необходимости задайте `TEST_TEMPORAL_ADDRESS` (по умолчанию localhost:7233).
Для replay сохранённой JSON-истории задайте `TEST_WORKFLOW_HISTORY=/path/history.json`
вместо `TEST_WORKFLOW_ID`; это позволяет повторять проверку на одном файле без сервера.


Обычные `docker compose restart` и `docker compose down` сохраняют данные
PostgreSQL, историю Temporal и отдельный volume `simulator-data` с состоянием симулятора.
Текущие миграции рассчитаны на чистую установку и **несовместимы с прежней общей
схемой симулятора и приложения**. Перенос данных не реализован. Для перехода
с той старой общей схемы Compose использует новый volume `demo-postgres-data`,
сохраняя прежний `postgres-data` отдельно.
Полный сброс **удаляет все VM, операции, данные симулятора и истории Temporal этого демо**:

```sh
docker compose down -v
```

Реальных гипервизоров, API, multi-tenancy, биллинга, миграции работающей VM,
Loki/Grafana и production-оповещений здесь нет. Для реального провайдера барьер
отката должен учитывать уже принятые асинхронные команды инфраструктуры:
одного флага в PostgreSQL недостаточно.

## Структура пакетов

```text
internal/
  admin/                  # HTTP API, DTO и статический frontend
  orchestration/          # Lifecycle, очередь и настройки Activities
    activities/           # контекст Temporal, логи, инъекция сбоев и ошибки SDK
  execution/              # выполнение шагов через Store и Provider без Temporal
  outbox/                 # доставка операций в Temporal
  visibility/             # регистрация полей поиска Temporal UI
  reconcile/              # сверка незавершённых операций с Temporal
  provider/               # Provider, Request, State и ошибки контракта
    simulator/            # Состояние в памяти, задержки, HTTP-сервер и клиент
  vm/                     # модели VM, операций и шагов без Temporal
  postgres/               # прикладное хранилище
  logging/
  testdb/
```

Реальные адаптеры добавляются в `internal/provider/<имя>` рядом с симулятором
и реализуют `provider.Provider`. Общий пакет `provider` не импортирует реализации.
Execution использует интерфейс; конкретный провайдер передаётся при сборке
бинарника iqhost-worker. Для подключения нового адаптера не требуется менять workflow.

Перегруппировка пакетов не меняет SQL-схемы и не требует сброса данных.
Workflow называется `Lifecycle`, очередь — `iqhost-vm`.
Каждый шаг имеет отдельное имя Activity в Temporal UI:
`vm.begin-operation`, `vm.reserve-resources`, `vm.create-instance`,
`vm.attach-network`, `vm.start-instance`, `vm.wait-ready`, `vm.stop-instance`,
`vm.detach-network`, `vm.delete-instance`, `vm.release-resources`,
`vm.begin-rollback`, `vm.finish-operation`.
Переименование Activities несовместимо со старыми историями `vm.step`;
для просмотра новых имён пересоберите worker и запустите новую операцию.

## Подключение инфраструктурного адаптера

Контракт `internal/provider.Provider` не зависит от PostgreSQL или Temporal.
Worker явно передаёт хранилище и провайдер в `execution.Service`,
который вызывается через `activities.Service`; по умолчанию
это `simulator.Client` из `internal/provider/simulator`. Для нового адаптера замените зависимость Provider при
создании сервиса в `cmd/iqhost-worker/worker.go`. HTTP-транспорт симулятора реализован; SSH и локальные команды не реализованы.

Каждая команда содержит VM ID, operation ID, фазу, поколение, ключ
`operation/phase/step`, конфигурацию и выделенные compute/GPU/disk/IP.
Ключ и содержимое нужно сохранять устойчиво: повтор возвращает прежний результат,
изменённое содержимое — `provider.PermanentError`. Прочие ошибки повторяются;
Activities преобразуют постоянные ошибки в non-retryable и сохраняют отмену контекста.
Успех означает завершение действия; Delete обязан подтвердить отсутствие экземпляра.
Для асинхронного API адаптер сохраняет ID принятой задачи и при повторах ожидает её.

Fence обязан исключать поздние эффекты всех старых поколений, включая уже принятые
удалённые задачи. Без успешного барьера очистка не продолжается. Cleanup выполняет
Fence и Inspect, затем идемпотентную очистку. Reconciler только блокирует операцию
и сохраняет резервы. Compute освобождается после подтверждённого Delete текущего
поколения; stop/start сохраняют диск и IP.

`infrastructure_commands` хранит запросы и подтверждения приложения,
`confirmed_instances` — подтверждённое состояние. `operation_steps` содержит только
завершённые шаги. Симулятор отдельно хранит фактическое состояние VM, поколения
Fence и результаты команд в картах под одним mutex. Транзакция приложения
не удерживается во время HTTP-вызова.

Сервис `iqhost-simulator` обслуживает всех worker через `POST /commands`.
Обработчик и клиент находятся в [http.go](../internal/provider/simulator/http.go) и
[client.go](../internal/provider/simulator/client.go), запуск — в
[main.go](../cmd/iqhost-simulator/main.go). Симулятор не зависит от прикладной БД или Temporal.

| Команда | Задержка |
| --- | --- |
| Create | 3 с |
| AttachNetwork / DetachNetwork | 1 с |
| Start / Stop / Delete | 2 с |
| WaitReady | 3 с |
| Fence / Inspect | Нет |

Таймер учитывает отмену контекста и не удерживает mutex. До ожидания и после
него проверяются поколение, повтор, конфликт содержимого и допустимость действия.
Одновременные повторы применяют эффект один раз, завершённый повтор возвращается
сразу. Fence немедленно блокирует эффекты ожидающих команд старого поколения.
Отмена до фиксации не меняет состояние; после фиксации потерянный ответ можно
восстановить повтором. `NewWithDelays(Delays{})` отключает задержки в тестах.

HTTP-клиент имеет таймаут 10 секунд и не делает собственных повторов.
Повторами управляют Activities с прежним таймаутом 60 секунд.
Ошибочный JSON и неизвестный шаг возвращают 400, постоянная ошибка провайдера —
422, внутренняя — 500. Ответ содержит `state` или `error`; Fence возвращает
нулевое состояние. Клиент сохраняет постоянную классификацию и отмену контекста.

В Compose симулятор запускается с `--state-file` и сохраняет VM, поколения Fence
и результаты команд в volume `simulator-data` до подтверждения команды. Снимок
записывается во временный файл, затем атомарно заменяет прежний. При ошибке записи
команда не применяется. Перезапуск восстанавливает состояние из этого файла,
без обращения к прикладной БД. Повреждённый снимок останавливает запуск.
Файл предназначен для одного процесса и небольшого объёма демо; промышленное
хранилище и устойчивость к отказу диска здесь не реализованы.
Без `--state-file` симулятор по-прежнему хранит данные только в памяти.

БД и пользователь прежнего SQL-симулятора больше не создаются и не используются.
Существующие базы автоматически не удаляются. Прикладные миграции не изменены.
Старые Launch/Cleanup histories не поддерживаются.
Физическое создание дисков и IPAM остаются будущими интеграциями.

## Независимые процессы

`make build` собирает шесть бинарников в `bin/`: `iqhost` (команды, миграции и seed),
`iqhost-worker` (workflow и Activities), `iqhost-outbox` (доставка),
`iqhost-reconciler` (сверка), `iqhost-simulator` (HTTP-симулятор),
`iqhost-admin` (HTTP API и frontend). Команда `iqhost worker` удалена.
Docker-образ содержит все шесть бинарников; entrypoint по умолчанию — `iqhost`.

Compose запускает по одному экземпляру worker, outbox и reconciler. Outbox и
reconciler ждут основную БД, её миграции и Temporal, независимо от симулятора
и worker. Для двух исполнителей с одним dispatcher и одним reconciler:

```sh
docker compose up -d --scale worker=2
```

Выбора лидера и распределения сверки между reconciler нет: оставляйте один экземпляр.
Масштабирование worker не увеличивает число dispatcher/reconciler.
Все фоновые процессы обрабатывают SIGINT/SIGTERM и закрывают свои подключения;
worker даёт выполняющимся задачам до 30 секунд на остановку.
`postgres` не зависит от SDK Temporal: передаёт `vm.RejectedError`, а Activities
преобразуют её в прежнюю non-retryable ошибку `Rejected`. Диагностические поля
Activities передаются в хранилище через контекст logger из `logging`.
