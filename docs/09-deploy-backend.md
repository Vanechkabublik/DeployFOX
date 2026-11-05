# NestJS, Go, FastAPI и бот

HTTP-приложение слушает порт и отвечает на запросы браузера. Оно должно слушать `0.0.0.0`, а не только `127.0.0.1` внутри контейнера. Фоновый worker / Telegram-бот с long polling сам обращается к внешнему API и может не иметь HTTP-порта. При создании выберите «Фоновый процесс»: у него нет домена, а старт проверяется по пяти секундам работы без выхода и рестарта. Это проверка процесса, а не доказательство готовности внешнего Telegram API.

## NestJS

В main.ts: `await app.listen(process.env.PORT ?? 3000, '0.0.0.0')`. Добавьте `/health`, возвращающий 200. Dockerfile:

```dockerfile
FROM node:24-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build && npm prune --omit=dev
FROM node:24-alpine
WORKDIR /app
ENV NODE_ENV=production PORT=3000
COPY --from=build --chown=node:node /app/node_modules ./node_modules
COPY --from=build --chown=node:node /app/dist ./dist
USER node
CMD ["node", "dist/main.js"]
```

В панели: порт 3000, healthcheck `/health`. У некоторых NestJS-проектов файл находится в `dist/src/main.js`: проверьте реальный результат `npm run build`. DATABASE_URL добавьте через редактор переменных, не ARG в Dockerfile.

## Go

Приложение должно слушать `:8080`; добавьте `/health` с 200. Для проекта с main package в корне:

```dockerfile
FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app .
FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D app
COPY --from=build /app /app
USER app
CMD ["/app"]
```

В панели порт 8080, healthcheck `/health`. Если main в cmd/server, измените `go build` на соответствующий путь.

## FastAPI

Пусть `main.py` содержит `app = FastAPI()` и `/health`. requirements.txt должен включать fastapi и uvicorn с закреплёнными проверенными вами версиями.

```dockerfile
FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
RUN useradd -m app
USER app
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
```

Порт 8000, healthcheck `/health`.

## Python Telegram-бот

BOT_TOKEN храните в редакторе Environment Variables. Не выводите его в print и traceback URL. В проекте бота с вашим bot.py и requirements.txt используйте тот же Python Dockerfile, но замените CMD на `["python", "bot.py"]`. В панели выберите «Фоновый процесс», оставьте домен пустым, добавьте BOT_TOKEN и выполните Deploy. Значения порта/HTTP path для этого типа не используются.

При long polling должна работать одна копия. Агент останавливает предыдущий фоновый контейнер до запуска нового; при неудаче запускает старый обратно. Это вызывает перерыв и не является zero-downtime. Запуск фонового процесса без HTTP проверен локальным Docker-тестом; внешняя работа Telegram-бота не проверялась.

Для Laravel, Nuxt и других стеков используйте Dockerfile проекта или поддерживаемую конфигурацию Railpack. Миграции баз выполняйте осознанно: rollback образа не отменяет изменения схемы базы.
