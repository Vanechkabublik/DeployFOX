# Диагностика без потери данных

Команды ниже выполняются на VPS, кроме явно указанных PowerShell. Сначала читайте ошибку, затем меняйте настройки. Не удаляйте volumes и ключи для «починки». Вывод логов может содержать секреты вашего приложения: удалите их перед публикацией.

## SSH connection refused

Причина: SSH не слушает порт, неверный IP/порт, firewall отклоняет соединение.

Проверка на Windows: `Test-NetConnection 203.0.113.10 -Port 22`. В web console провайдера: `sudo systemctl status ssh`, `sudo ss -ltnp`, `sudo ufw status`.

Исправление: проверьте настоящий IP, разрешите именно используемый SSH-порт, выполните `sudo sshd -t`, затем `sudo systemctl start ssh`. Если порт другой, подключайтесь `ssh -p НОМЕР deploy@IP`. Проверка результата: откройте новое SSH-соединение, не закрывая console.

## Permission denied при SSH

Причина: неверное имя пользователя, пароль, приватный ключ или права authorized_keys. В PowerShell: `ssh -v -i "$env:USERPROFILE\.ssh\deployfox_vps" deploy@203.0.113.10`. Не публикуйте весь подробный вывод.

На сервере через сохранившееся соединение: `sudo journalctl -u ssh -n 80`, `ls -ld /home/deploy/.ssh`, `ls -l /home/deploy/.ssh/authorized_keys`. Владелец должен быть deploy, каталог 700, файл 600. Сравните строку публичного ключа. Исправьте chown/chmod и снова проверьте вход ключом. Не отключайте парольный вход до успешного теста ключа.

## Docker daemon unavailable

Причина: Docker не запущен, нет прав или Docker Desktop ещё не готов.

Ubuntu: `sudo systemctl status docker`, `sudo journalctl -u docker -n 100`, `sudo docker info`. Исправление: `sudo systemctl start docker`; ошибки диска сначала исправьте по соответствующему разделу. Windows: запустите Docker Desktop и дождитесь Engine running, затем `docker version`. Проверка: Client и Server доступны, `docker run --rm hello-world` выполняется.

## Port already allocated

Причина: другой контейнер или системная служба использует 80/443/3000.

Проверка: `sudo ss -ltnp`, `sudo docker ps --format 'table {{.Names}}\t{{.Ports}}'`. Windows: `Get-NetTCPConnection -LocalPort 3000 -ErrorAction SilentlyContinue`.

Исправление: выясните владельца службы. Не останавливайте неизвестный production-сервис. Перенесите существующий сервис или выберите другой свободный порт локальной панели и согласованно измените PUBLIC_URL. Для production домены должны идти через один Caddy. Проверка: `docker compose ... ps`, затем curl на нужный URL.

## DNS not resolved

Причина: запись отсутствует, находится не у того DNS-провайдера, ещё кешируется или DNS сервера недоступен.

Windows: `Resolve-DnsName deploy.example.com -Type A`. Ubuntu: `getent ahostsv4 deploy.example.com`, `resolvectl status`. В контейнере: `sudo docker compose -f docker-compose.prod.yml exec agent getent hosts github.com`.

Исправление: проверьте authoritative nameservers у регистратора, корректную A-запись и DNS resolver VPS/Docker. Не подменяйте github.com случайным IP и не отключайте SSRF-проверки. Проверка: имя возвращает ожидаемый публичный адрес, сборка снова получает зависимости.

## HTTPS certificate error

Причина: неверный DNS/AAAA, закрыты 80/443, Caddy ещё проходит ACME, лимит выдачи сертификатов или неверное время сервера.

Проверка: `curl -vI https://deploy.example.com`, `timedatectl`, `sudo docker compose -f docker-compose.prod.yml logs --tail 200 caddy`, `sudo ss -ltnp`. Проверьте firewall провайдера.

Исправление: направьте A на VPS, удалите ошибочную AAAA, откройте порты, настройте синхронизацию времени. При rate limit дождитесь указанного времени; не пересоздавайте аккаунт ACME многократно. Проверка: браузер открывает сайт без предупреждения, curl без -k успешен.

## GitHub webhook failed

Причина: неверный URL, secret, сертификат, App не установлена на репозиторий или выбрана другая ветка.

Проверка: GitHub App → Advanced → Recent deliveries → Request/Response; backend logs через Compose. 401 — подпись, 413 — слишком большой payload, 5xx — инфраструктура, 2xx без деплоя — событие/ветка/активная задача.

Исправление: проверьте GITHUB_WEBHOOK_SECRET с обеих сторон, HTTPS URL с `/api/v1/github/webhook`, installation permissions. Пересоздайте контейнеры после изменения env. Проверка: Redeliver даёт 2xx, новый push при свободном проекте создаёт ровно одну задачу. Повтор одной delivery не должен создавать новую.

## Docker build failed

Причина: ошибка Dockerfile, зависимостей, build context, недостаток памяти или недоступная сеть.

Проверка: Деплои → Логи; `sudo docker compose -f docker-compose.prod.yml logs --tail 100 worker agent`; `sudo docker buildx inspect deployfox`; локально в исходниках `docker build .`.

Исправление: проверьте пути с учётом регистра Linux, COPY, lockfile, npm ci, go.mod, requirements, команду запуска. Для неизвестного Railpack проекта добавьте Dockerfile. Создайте builder по install.sh, если он отсутствует. Runtime secrets не доступны при build: перестройте приложение так, чтобы оно не требовало production-паролей на этапе сборки. Проверка: локальная сборка успешна, затем новый Deploy проходит BUILDING.

## Container exited

Причина: неверная команда запуска, отсутствующая настройка, ошибка приложения или OOM.

Проверка: `sudo docker ps -a --filter label=deployfox.project`, затем `sudo docker logs --tail 100 ИМЯ_КОНТЕЙНЕРА` и `sudo docker inspect --format '{{json .State}}' ИМЯ_КОНТЕЙНЕРА`.

Исправление: исправьте CMD/entrypoint, переменные и права; процесс сервера должен оставаться foreground. Проверка: контейнер Running и health endpoint возвращает 2xx/3xx. Если приложение пишет только в файл, перенастройте вывод в stdout/stderr для Docker logs.

## Database connection refused

Причина: база ещё инициализируется, остановлена, неверный hostname/port либо приложение использует localhost.

Проверка: `sudo docker ps --filter label=deployfox.database`, `sudo docker logs --tail 100 dfdb-UUID`, `sudo docker network inspect deployfox-apps`. PostgreSQL: `sudo docker exec dfdb-UUID pg_isready -U app -d app`.

Исправление: запустите базу, дождитесь готовности, возьмите внутренний hostname из connection, сохраните DATABASE_URL и сделайте Deploy приложения. Не публикуйте порт в интернет для устранения внутренней ошибки. Проверка: приложение успешно делает реальный SELECT/SET, не только TCP-connect.

## Out of memory

Причина: суммарная память контейнеров и build превышает VPS или индивидуальный лимит.

Проверка: `free -h`, `sudo docker stats --no-stream`, `sudo journalctl -k --since '1 hour ago' | grep -i oom`, inspect State.OOMKilled. Exit 137 может указывать на принудительное завершение, но не всегда означает OOM.

Исправление: уменьшите параллельную нагрузку, оптимизируйте сборку/приложение, увеличьте VPS или осознанно лимит конкретного проекта. BuildKit ограничен отдельно, не отключайте лимиты вслепую. Проверка: повторный деплой успешен, OOM-события не появляются, SSH остаётся отзывчивым.

## No space left on device

Причина: образы, cache, данные баз или логи заняли диск/иноды.

Проверка: `df -h`, `df -i`, `sudo docker system df`, `sudo du -h -d 1 /var/lib/docker`. Не редактируйте внутренние файлы Docker вручную.

Исправление: экспортируйте и по политике удалите ненужные старые backups, удалите только явно проверенные неиспользуемые образы/контейнеры или увеличьте диск. Очистка cache `docker buildx prune --builder deployfox` требует понимания, что будут потеряны кеши сборок. Не используйте prune --volumes: можно потерять базы. Проверка: df показывает запас, тестовая запись/сборка выполняется.

## 502 Bad Gateway

Причина: Caddy не достучался до контейнера, неверный порт, приложение слушает localhost или контейнер упал после healthcheck.

Проверка: Caddy logs, docker ps, docker logs приложения, `docker network inspect deployfox-apps`. Проверка HTTP из сети: используйте доверенный диагностический контейнер на deployfox-apps и обращайтесь к имени контейнера и его внутреннему порту.

Исправление: исправьте bind address на 0.0.0.0, порт и health path, убедитесь в работоспособности контейнера, выполните новый Deploy или Rollback. Проверка: реальный домен возвращает ожидаемую страницу по HTTPS, а не только health endpoint.

## Приложение сломалось после обновления

Причина: новая версия несовместима со схемой базы, текущими env или внешним API; healthcheck слишком поверхностный.

Проверка: сравните image/commit в истории, application logs и последние миграции. Не запускайте старую версию поверх несовместимой схемы автоматически.

Исправление: Rollback образа при совместимости данных; иначе восстановите проверенную копию базы на отдельной среде и спланируйте перенос. Исправьте healthcheck, чтобы он проверял критическую готовность. Проверка: выполните основные пользовательские действия и чтение/запись данных, а не только запрос `/health`.
