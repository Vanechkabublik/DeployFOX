# Разработка и проверки

Основной стек запускается одной командой из корня:

```bash
docker compose -f docker-compose.local.yml up -d --build
```

В обычном local mode нет Docker socket. Одноразовые секреты генерирует init, они хранятся в отдельном local_secrets volume. Основной URL — http://localhost:3000. Go-сборка контейнеризована, поэтому Go на Windows не обязателен.

## Frontend

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm test
npm run build
```

Для dev-сервера отдельно установите API_INTERNAL_URL на внутренний адрес API, который действительно доступен вашему процессу. Production API по умолчанию не публикует порт. Не делайте публичную публикацию backend ради dev-прокси.

## Backend unit и реальная интеграция PostgreSQL/Redis

С установленным Go: `cd backend && go test ./... && go vet ./...`. Интеграционный тест явно пропускается без TEST_DATABASE_URL — пропуск не считается прохождением интеграции.

Создайте disposable базу deployfox_test на local PostgreSQL:

```bash
docker compose -f docker-compose.local.yml exec postgres createdb -U deployfox deployfox_test
```

Linux/Git Bash (для PowerShell путь замените на абсолютный Windows path):

```bash
docker run --rm --network deployfox-local_control \
  -v "$PWD/backend:/src" -v deployfox_gomod:/go/pkg/mod \
  -w /src \
  -e 'TEST_DATABASE_URL=postgres://deployfox@postgres:5432/deployfox_test?sslmode=disable' \
  -e 'TEST_REDIS_URL=redis://redis:6379/15' \
  golang:1.27.3-alpine sh -c 'go test -count=1 ./... && go vet ./...'
```

**Тест очищает таблицы указанной deployfox_test. Не направляйте его на важные данные.** Он использует настоящие PostgreSQL и Redis, а lifecycle mock применяется только к deployment provider. Покрываются setup, auth, CSRF, encrypted env, конкурентные insert, worker, webhook и 2FA/recovery.

Для golangci-lint 2: `cd backend && golangci-lint run`. Конфигурация в backend/.golangci.yml включает govet, staticcheck, ineffassign, unused и gofmt.

## Браузерные проверки отдельно от основной установки

```bash
docker compose -p deployfox-qa -f docker-compose.local.yml -f docker-compose.qa.yml up -d --no-build --wait
cd frontend
npx playwright install chromium
cd ..
node infrastructure/scripts/qa.mjs
```

QA использует порт 3001 и собственные volumes. Скрипт генерирует случайный пароль и сохраняет его в игнорируемом .qa-credentials.json; не коммитьте этот файл. Только в этом изолированном стеке он очищает rate counters и снова включает wizard для повторяемой проверки. Проверяются мастер, desktop lifecycle, сохранение настроек и мобильная навигация; screenshots находятся в frontend/test-results. После завершения можно остановить QA через ту же compose-команду с `down` без -v.

## Реальные Docker-тесты (только доверенная среда)

Сначала соберите agent:

```bash
docker build --target agent -t deployfox-agent:local backend
```

На Windows:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File infrastructure/scripts/test-docker.ps1
```

Тест монтирует socket только в изолированный тестовый agent, запускает Caddy на 127.0.0.1:8088, выполняет настоящие BuildKit builds собственного маленького fixture, healthcheck, routing, failed-candidate retention, rollback и pg_dump/pg_restore с проверкой строки данных. Созданные тестом контейнеры и PostgreSQL volume удаляются адресно по случайному UUID; пользовательские volumes не затрагиваются. Чужой deployfox-caddy тест не переиспользует.

Если отдельный BuildKit daemon не скачивается из registry, можно отдельно проверить lifecycle с Docker Desktop builder: задайте `DOCKER_TEST_BUILDER=default` перед запуском. Это **не проверяет лимиты production builder deployfox**. Такой режим использован в текущей локальной проверке, потому что загрузка moby/buildkit не завершилась вовремя. Дополнительно проверены фоновый процесс, смена/удаление домена, ограничения контейнера, метрики, Redis RDB export и восстановление ключа в отдельный Redis.

Полный тест скриптов управляющей базы на отдельном QA-стеке:

```bash
node infrastructure/scripts/test-backup.mjs
```

Он делает dump, добавляет маркер, восстанавливает копию с явным RESTORE в изолированной базе, проверяет количество проектов и отсутствие маркера. На Windows нужен Git Bash. Production-данные не затрагиваются.

Для opt-in работы всей локальной панели с Docker используйте:

```bash
docker compose -f docker-compose.local.yml -f docker-compose.real-local.yml up -d --build
```

Перед этим сгенерируйте AGENT_TOKEN в .env (64 случайных hex-символа; например `openssl rand -hex 32` в Git Bash/Linux). Этот режим меняет реальные контейнеры Docker Engine и предназначен только для доверенной машины. Не смешивайте его с обычным mock-развитием незаметно.

OpenAPI: backend/internal/openapi/openapi.json, защищённый endpoint `/api/v1/openapi.json`. Этот файл можно импортировать в Swagger Editor. Секретных значений спецификация не содержит.
