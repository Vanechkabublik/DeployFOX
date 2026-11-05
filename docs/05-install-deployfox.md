# Установка DeployFox на VPS

Публичного репозитория этого проекта пока нет. Поэтому здесь нет выдуманной команды git clone. Перенесите реальную папку, которую вы получили. Не копируйте node_modules, .next, локальные backups, .env и приватные ключи в общедоступные места.

## 1. Подготовьте архив на Windows

В локальном PowerShell:

```powershell
Set-Location 'C:\Users\Иван\Desktop\gl'
tar --exclude=node_modules --exclude=.next --exclude=.git --exclude='.env*' --exclude=backups --exclude=secrets --exclude=.qa-credentials.json --exclude=.local-admin.env --exclude='*.log' --exclude='*.test' --exclude=test-results -czf deployfox-source.tar.gz deployfox
scp -i "$env:USERPROFILE\.ssh\deployfox_vps" .\deployfox-source.tar.gz deploy@203.0.113.10:/home/deploy/
```

Замените IP. Если ваша папка находится в другом месте, замените путь в Set-Location. `tar` упаковывает файлы, `scp` копирует их по SSH.

## 2. Распакуйте на Ubuntu

```bash
cd /home/deploy
mkdir -p apps
tar -tzf deployfox-source.tar.gz | head
```

Убедитесь, что внутри пути начинаются с `deployfox/`, а не с `/` или `../`. Затем:

```bash
tar -xzf deployfox-source.tar.gz -C /home/deploy/apps
cd /home/deploy/apps/deployfox
```

## 3. Настройте DNS и запустите установку

До запуска Caddy создайте A-запись домена по [следующей главе](06-connect-domain.md). Затем:

```bash
sudo bash infrastructure/scripts/install.sh
```

Скрипт проверит Ubuntu, Docker, Compose, свободные порты и ресурсы, спросит домен и email. Затем создаст `.env` со случайными паролями и отдельным ключом шифрования, настроит ограниченный BuildKit builder и запустит сервисы. Первая сборка может занять несколько минут. Скрипт не устанавливает Docker и не удаляет volumes.

Повторный запуск использует существующий `.env`. Не удаляйте его для «сброса»: потеря ENCRYPTION_KEY сделает сохранённые секреты нечитаемыми. Для ручного просмотра и изменения:

```bash
sudo nano .env
sudo docker compose -f docker-compose.prod.yml config --quiet
sudo docker compose -f docker-compose.prod.yml up -d --build --wait
```

Не запускайте `compose config` без `--quiet` в общедоступных логах: развёрнутый вывод содержит секреты.

## 4. Проверьте состояние

```bash
sudo docker compose -f docker-compose.prod.yml ps
sudo docker compose -f docker-compose.prod.yml logs --tail 100 api worker frontend caddy
```

Ожидаются healthy. API и frontend не публикуются напрямую; доступ идёт через Caddy на 443.

## 5. Создайте администратора

Откройте выведенный URL панели. Получите токен только на своём терминале:

```bash
sudo sed -n 's/^SETUP_TOKEN=//p' .env
```

Введите токен, email и новый пароль минимум 12 символов. Повторное создание администратора блокируется первичным ключом в PostgreSQL, даже если токен сохранился. Не передавайте токен другим людям. После настройки берегите пароль и SSH-ключ. Встроенного сброса через email нет.

После создания администратора откроется мастер из семи шагов: аккаунт, домен, Docker, диск, Caddy, GitHub, первый проект. Docker/диск/Caddy проверяются через отдельный agent; HTTPS панели дополнительно проверьте в браузере. В local mock проверки VPS явно пропускаются. Для Docker image или публичного Git URL шаг GitHub можно пропустить. После создания проекта нажмите Открыть Dashboard. Повторная публичная регистрация при этом остаётся закрытой.
