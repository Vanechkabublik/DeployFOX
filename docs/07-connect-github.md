# Подключение GitHub App

Используется GitHub App с короткоживущими installation tokens. Персональные access tokens не нужны. Один раз внесите App ID, App slug и PEM-ключ на сервере. Затем Connect GitHub откроет штатную установку App; выбор репозиториев выполняется на GitHub. Installation ID можно привязать через браузер или указать вручную как запасной способ.

Официальная инструкция: [Registering a GitHub App](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app).

1. Войдите на GitHub в браузере.
2. Нажмите аватар → Settings → Developer settings → GitHub Apps → New GitHub App.
3. Задайте уникальное имя, например с вашим именем и случайным суффиксом.
4. Homepage URL: полный HTTPS-адрес вашей панели.
5. Callback URL для OAuth user authorization оставьте пустым, Request user authorization выключите. В **Setup URL** укажите `https://ВАШ_ДОМЕН/api/v1/github/callback`, включите Redirect on update. Это разные поля: используется setup-возврат установки App, а не OAuth user token.
6. Webhook Active включён. URL: `https://ВАШ_ДОМЕН/api/v1/github/webhook`.
7. На VPS получите существующий webhook secret:

```bash
sudo sed -n 's/^GITHUB_WEBHOOK_SECRET=//p' .env
```

8. Вставьте его в Webhook secret. Не отключайте SSL verification.
9. Repository permissions: **Contents → Read-only**, Metadata → Read-only. Остальным не давайте доступ.
10. Subscribe to events: **Push**.
11. Where can this GitHub App be installed: Only on this account.
12. Создайте App. Скопируйте App ID с её страницы.
13. Нажмите Generate a private key. Скачанный `.pem` — секрет. Передайте его на VPS из PowerShell (замените путь к реальному файлу):

```powershell
scp -i "$env:USERPROFILE\.ssh\deployfox_vps" 'C:\Users\Иван\Downloads\ИМЯ_ФАЙЛА.pem' deploy@203.0.113.10:/home/deploy/github-app.pem
```

14. На Ubuntu:

```bash
cd /home/deploy/apps/deployfox
sudo install -m 644 /home/deploy/github-app.pem secrets/github-app.pem
sudo chmod 700 secrets
rm /home/deploy/github-app.pem
```

Файл имеет 644 внутри закрытого каталога 700: контейнер API работает не от root и должен читать смонтированный Docker secret. Каталог не должен быть доступен другим пользователям VPS.

15. В GitHub App откройте Install App → Install. Выберите только доверенные репозитории, которые собираетесь развернуть, включая приватные.
16. После установки адрес браузера заканчивается `/installations/12345678`. Число — Installation ID.
17. На сервере `sudo nano .env`: заполните GITHUB_APP_ID и GITHUB_APP_SLUG. Slug — часть адреса публичной страницы App после `https://github.com/apps/`, маленькими буквами с дефисами. Путь GITHUB_PRIVATE_KEY_FILE оставьте `./secrets/github-app.pem`. GITHUB_INSTALLATION_ID можно оставить пустым для браузерной привязки либо заполнить числом из шага 16 для ручного режима.
18. Примените:

```bash
sudo docker compose -f docker-compose.prod.yml up -d --force-recreate api worker agent
```

19. В панели откройте GitHub → Connect GitHub. На GitHub выберите ваш аккаунт/организацию и только доверенные репозитории, сохраните выбор. После возврата проверьте отображаемое имя аккаунта и нажмите Подтвердить. Redirect сам по себе не включает доступ: server проверяет App installation, одноразовый state и отдельное подтверждение администратора с CSRF. Должен появиться список репозиториев. Теперь в форме GitHub-проекта можно выбрать репозиторий и ветку из списка (первые 100) либо ввести URL/ветку вручную.
20. Для автодеплоя отметьте соответствующий checkbox при создании.

Если App устанавливается напрямую через GitHub без Connect, возврат может показать ошибку отсутствующего state. Это защита, а не повод отключить проверку. Вернитесь в панель и начните Connect либо настройте ручной Installation ID. Механизм state описан в [документации GitHub](https://docs.github.com/en/apps/sharing-github-apps/sharing-your-github-app); нельзя слепо доверять installation_id из redirect — см. [Setup URL](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/about-the-setup-url).

Проверка: в настройках App → Advanced → Recent deliveries посмотрите ответ webhook. Должен быть 2xx. Повторная доставка с тем же X-GitHub-Delivery не создаёт повторную задачу. Push в другую ветку игнорируется. Если проект уже развёртывается, сохраняется последний pending commit: после текущего деплоя worker поставит его в очередь. Быстрые промежуточные push объединяются в последний, а не запускают параллельные сборки.
