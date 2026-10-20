// Запуск из корня проекта: playwright-cli -s=iqhost-demo run-code --filename=scripts/record-admin-demo.js
// Скрипт создаёт одну настоящую VM. Для повторной записи нужен свободный инвентарь.
async (page) => {
  const vmID = 'customer-web-' + new Date().toISOString().slice(11, 19).replaceAll(':', '');
  const chapters = [];
  let startedAt;
  let caption;
  const pause = (milliseconds) => page.waitForTimeout(milliseconds);
  async function chapter(title, description) {
    if (caption) await caption.dispose();
    chapters.push({ seconds: Math.round((Date.now() - startedAt) / 1000), title });
    await page.screencast.showChapter(title, { description, duration: 2400 });
  }
  async function explain(text) {
    if (caption) await caption.dispose();
    caption = await page.screencast.showOverlay(
      `<div style="position:absolute;bottom:18px;left:290px;right:28px;background:#132238ed;color:white;padding:16px 22px;border-radius:12px;font:18px/1.45 system-ui;box-shadow:0 4px 24px #0003;border-left:4px solid #34d399">${text}</div>`,
    );
  }

  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('http://localhost:8082/vms');
  await page.getByRole('link', { name: 'demo-gpu', exact: true }).waitFor();
  await page.screencast.start({ path: 'artifacts/demo/iqhost-admin-demo.webm', size: { width: 1440, height: 1000 }, fps: 30 });
  startedAt = Date.now();
  await page.screencast.showActions({
    duration: 650,
    style: {
      point: 'width:16px;height:16px;border-radius:50%;background:#10b98199',
      highlight: 'outline:2px solid #10b981;outline-offset:3px',
      title: 'display:none',
    },
  });
  let operationURL;
  let temporalURL;
  try {
    await chapter('IQHost · Демонстрация админки', 'Создание VM → выполнение в Temporal → обзор страниц');
    await explain('После запуска Docker Compose уже доступны VM, история операций и инвентарь.');
    await pause(3500);
    await page.getByRole('textbox', { name: 'Инициатор', exact: true }).fill('customer-demo');
    await chapter('1. Создаём виртуальную машину', '2 CPU · 2 GiB RAM · диск 10 GiB');
    await page.getByRole('button', { name: 'Создать VM', exact: true }).click();
    await page.getByRole('textbox', { name: 'ID VM', exact: true }).pressSequentially(vmID, { delay: 75 });
    await page.getByRole('spinbutton', { name: 'CPU', exact: true }).fill('2');
    await page.getByRole('spinbutton', { name: 'RAM, MiB', exact: true }).fill('2048');
    await page.getByRole('spinbutton', { name: 'Диск, GiB', exact: true }).fill('10');
    await explain('Нажатие «Создать и запустить» отправит команду. Создание включает запуск VM.');
    await pause(3000);
    await page.getByRole('button', { name: 'Создать и запустить', exact: true }).click();
    await page.waitForURL('**/operations/*');
    operationURL = page.url();
    await explain('Операция выполняется асинхронно. Страница автоматически обновляет статус и завершённые шаги.');
    await page.locator('.page-heading .badge.succeeded').waitFor({ timeout: 90000 });
    await explain('Операция завершена успешно. Ресурсы выделены, экземпляр создан, сеть подключена, VM запущена.');
    await pause(5000);
    await page.getByRole('heading', { name: 'Завершённые шаги', exact: true }).scrollIntoViewIfNeeded();
    await pause(3000);

    await chapter('2. Смотрим pipeline в Temporal', 'История именно той операции, которую только что создали');
    const temporalLink = page.getByRole('link', { name: 'Открыть workflow в Temporal UI ↗', exact: true });
    temporalURL = await temporalLink.getAttribute('href');
    // Открываем ту же ссылку в текущей вкладке, чтобы запись была одним непрерывным видео.
    await temporalLink.evaluate((element) => element.removeAttribute('target'));
    await temporalLink.click();
    await page.getByRole('heading', { name: 'Event History', exact: true }).waitFor();
    await page.getByText('Completed', { exact: true }).first().waitFor();
    await explain('Completed — workflow успешно завершён. Диаграмма показывает порядок и длительность Activities.');
    await pause(6500);
    await page.getByRole('button', { name: 'Compact', exact: true }).click();
    await page.mouse.wheel(0, 560);
    await explain('Шаги: begin → reserve → create → attach-network → start → wait-ready → finish.');
    await pause(6500);
    await page.mouse.wheel(0, -1000);
    await pause(2000);

    await chapter('3. Карточка созданной VM', 'Подтверждённое состояние, ресурсы и история команд');
    await page.goBack();
    await page.getByRole('link', { name: vmID, exact: true }).click();
    await page.getByRole('heading', { name: vmID, exact: true }).waitFor();
    await page.getByText('В работе', { exact: true }).first().waitFor();
    await explain('VM в работе. В карточке видны нода, IP, диск и действия: остановить или удалить.');
    await pause(6000);

    await chapter('4. Журнал операций', 'Все команды, результаты и фильтры');
    await page.getByRole('navigation').getByRole('link', { name: 'Операции' }).click();
    await page.getByRole('heading', { name: 'Операции', exact: true }).waitFor();
    await explain('В журнале есть новая успешная операция и заранее подготовленные примеры, включая ошибку.');
    await pause(4500);
    await page.getByRole('textbox', { name: 'ID VM', exact: true }).pressSequentially(vmID, { delay: 65 });
    await pause(3500);
    await page.getByRole('textbox', { name: 'ID VM', exact: true }).fill('');

    await chapter('5. Ресурсы платформы', 'Вычислительные ноды, GPU, хранилища и IP-пулы');
    await page.getByRole('navigation').getByRole('link', { name: 'Ресурсы' }).click();
    await page.getByRole('heading', { name: 'Вычислительные ноды', exact: true }).waitFor();
    await explain('Для нод показаны общая ёмкость, резерв и свободные CPU/RAM. Ниже — владельцы GPU.');
    await pause(5000);
    await page.getByRole('heading', { name: 'Хранилища', exact: true }).scrollIntoViewIfNeeded();
    await pause(2500);
    await page.getByText('Адреса и привязки', { exact: true }).click();
    await page.getByRole('heading', { name: 'IP-пулы', exact: true }).scrollIntoViewIfNeeded();
    await explain('Диски и IP связаны с конкретными VM. Здесь видны резервы приложения, а не нагрузка гипервизора.');
    await pause(5500);

    await page.getByRole('navigation').getByRole('link', { name: 'Виртуальные машины' }).click();
    await page.getByRole('link', { name: vmID, exact: true }).waitFor();
    await explain('Новая VM осталась в списке со статусом «В работе». Демо готово для дальнейших действий.');
    await pause(4500);
    await chapter('Демонстрация завершена', 'Админка: localhost:8082 · Temporal: localhost:8080');
  } finally {
    await page.screencast.stop();
  }
  return { vmID, operationURL, temporalURL, chapters };
}
