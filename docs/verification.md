# Проверки и оставшаяся работа

Обновлено 6 ноября 2026 года. Это работающая ранняя версия для одного владельца, но не завершённый production-ready релиз. Проверки выполнены локально на Windows с Docker Desktop. Наличие production Compose не означает проверку на настоящем VPS.

## Что реализовано

- Go API и worker, Next.js 16, PostgreSQL 16, Redis 7, миграции goose, OpenAPI.
- Одноразовая инициализация по случайному токену, единственный администратор, семишаговый мастер. После регистрации setup endpoint закрывается. Argon2id, HttpOnly cookies, Secure в production, CSRF и Origin, rate limiting, сессии, смена пароля, аудит, TOTP с защитой от повторного кода и одноразовыми recovery codes.
- Создание, редактирование и подтверждаемое удаление проектов, HTTP-приложения и фоновые процессы, ресурсы, домен, branch, Dockerfile/context, healthcheck, зашифрованные env. Deploy, start/stop/restart и rollback.
- Приватный Docker agent; socket отсутствует у API/frontend и приложений. Приложения имеют CPU/RAM/PID limits, dropped capabilities, no-new-privileges, вращение логов, без privileged и host mounts.
- Dockerfile BuildKit и fallback Railpack 0.43.0 с проверкой бинарника. Секреты панели не наследуются инструментами сборки, production env не передаётся как build arguments.
- Healthcheck кандидата перед переключением Caddy, проверка конфигурации и возврат предыдущей при ошибке. При неоднозначном состоянии маршрута кандидат сохраняется для проверки. Фоновый процесс обновляется с остановкой предыдущего и возвратом при неудаче; возможен простой.
- GitHub App: браузерный Connect, одноразовый state, проверка installation через API и отдельное подтверждение владельцем панели. Репозитории/ветки, временные installation tokens, private clone через askpass, HMAC webhooks, dedup, commit. Последний push во время активной задачи сохраняется для следующего деплоя.
- Durable queue в PostgreSQL, Redis как сигнал worker. Одна тяжёлая сборка одновременно; ограниченные повторы временных ошибок сборки, timeout и отмена на безопасных границах.
- Домены, резервирование маршрутов, DNS/80/443/TLS диагностика по публичному SERVER_IP. Caddy обеспечивает HTTPS в production; локальный тестовый Caddy использует HTTP.
- PostgreSQL, Redis и MySQL с volumes, закрытыми портами и зашифрованными случайными паролями. Readiness при создании/start/restart, явный connection URI, backups с SHA256, подтверждаемое восстановление PostgreSQL/MySQL.
- Текущие Linux/Docker метрики, live output Docker pull/build и подготовки Railpack через agent → worker → SSE, snapshots application logs; поиск, автопрокрутка, лимиты вывода, маскирование известных секретов. SSE закрывается после отзыва сессии.
- Local mock Compose, opt-in real-local, production Compose, установщик Ubuntu, backup/restore управляющей базы и 21 русская инструкция для новичка.

## Подтверждённые проверки

| Проверка | Результат и границы |
| --- | --- |
| Локальная сборка | API, worker, frontend, PostgreSQL и Redis healthy; http://localhost:3000. Повторный запуск сохраняет данные. |
| Go | Unit tests, go vet и golangci-lint пройдены: криптография, пути/SSRF, HMAC, TOTP, повторы, graceful shutdown. |
| Live build output | Получение строки до завершения процесса, ограничение длинной строки, agent/worker HTTP-протокол, маскирование GitHub-токена и отказ при оборванном потоке — пройдены. Внешняя GitHub-сборка не проверена. |
| PostgreSQL/Redis integration | Настоящие сервисы: setup/auth/CSRF, шифрование, восемь конкурентных deploy-запросов, queue, webhook/dedup/pending commit, домены, TOTP/recovery, logout и закрытие активного SSE. Только deployment provider этой группы явно mock. |
| Реальный Docker | BuildKit Dockerfile builds, routing, смена домена, неверный Caddy config и healthcheck с сохранением старой версии, rollback, фоновый процесс, удаление маршрута, runtime CPU/RAM/PID limits и метрики — пройдены. |
| PostgreSQL restore | Настоящие pg_dump/pg_restore в отдельную базу и через agent, проверка значения строки — пройдены. |
| Redis backup | RDB export и восстановление ключа в отдельный Redis без AOF — пройдены. Это не восстановление работающего Redis с appendonly. |
| Backup скрипты панели | На изолированном QA: четыре проекта сохранены, добавленный после backup маркер отсутствует после restore. |
| Frontend | TypeScript strict, ESLint, Prettier, component test и production build пройдены. |
| Playwright | Два сценария: мастер/создание/deploy/SSE/редактирование и мобильная навигация без горизонтального переполнения — пройдены. Deployment provider здесь mock. |
| Зависимости | npm audit: 0 advisory, включая dev. govulncheck: 0 достижимых уязвимостей и 0 в импортируемых пакетах; один advisory в неиспользуемом пакете x/crypto (OpenPGP). Это не полный security audit. |
| Конфигурация | Compose config local/production/real-local и синтаксис Bash проверены. Production стек и установщик на Ubuntu не запускались. |

Docker-тесты использовали builder `default` Docker Desktop: отдельный moby/buildkit не скачался. Лимиты production builder `deployfox` **не проверены**, runtime limits приложений проверены. Railpack запускается и сообщает версию; сборки всей матрицы языков не проверены.

MySQL-тест добавлен, но не выполнен: скачивание mysql:8.4 оборвалось из-за DNS Docker registry. Пропущенный тест не считается успешной проверкой MySQL.

Воспроизведение: [development.md](development.md). Не направляйте QA на production-базу.

## Что ещё нужно реализовать

1. Сверку Docker/Caddy/PostgreSQL после аварии worker. QUEUED сохраняется; прерванные активные задачи становятся FAILED с указанием проверить реальное состояние. Автоматического восстановления уже переключённого релиза нет.
2. Улучшить application logs: сейчас это периодические ограниченные snapshots, привязанные к контейнеру при открытии. После нового деплоя окно нужно открыть заново.
3. Redis restore из панели с учётом AOF; автоматические backups важных volumes, Caddy data/routes/certificates, расписания и offsite export. Backup панели не заменяет полный backup VPS.
4. Произвольные Git-hosts/SSH credentials и загрузку локального Dockerfile. Git-импорт ограничен GitHub HTTPS; Dockerfile берётся из репозитория. Списки GitHub ограничены первыми 100 элементами.
5. Редактирование restart policy, пользовательские volumes, декларации обязательных env, историю графиков и внешние уведомления. Системные логи доступны через Docker CLI, отдельного viewer в панели нет.
6. Постоянную сверку статусов приложений и баз: агрегаты отражают последнюю операцию, подробные метрики запрашиваются у Docker. Worker healthcheck проверяет процесс, не прогресс очереди.
7. Более точный brute-force limit по IP/аккаунту: за reverse proxy текущий лимит может быть общим. Нет встроенного Swagger UI и автоматического безопасного GC старых образов/контейнеров.

## Внешние проверки не выполнены

- Настоящая GitHub App: установка, private repository и push до работающего приложения. Код и тесты подписей/callback есть; внешних credentials нет.
- Публичный домен, DNS/ACME и HTTPS на VPS. Локальный HTTP не доказывает выпуск сертификата.
- Чистая Ubuntu 24.04, установщик, production update/rollback и полный disaster recovery на новом сервере.
- Все framework-сборки через Railpack, MySQL lifecycle/dump/restore, production BuildKit limits.
- Независимый security audit и аварии на каждой границе переключения трафика.

Приложения и базы используют общую apps-сеть. Docker API даёт высокие привилегии на хосте; agent не является песочницей для чужого кода. Используйте доверенные проекты одного владельца. Все критерии исходного задания пока не достигнуты.
