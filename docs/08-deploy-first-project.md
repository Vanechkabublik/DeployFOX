# Первый Next.js-проект

## Быстрая проверка образа

В панели: Новый проект → Docker image → имя `hello-nginx`, образ `nginx:alpine`, порт 80, healthcheck `/`. На production добавьте `hello.ВАШ_ДОМЕН` и соответствующую A-запись. Создайте проект, затем Deploy. Mock даёт только имитацию. Настоящий provider скачивает образ, запускает контейнер без публикации порта и проверяет HTTP-ответ. После успешного результата Caddy переключает домен.

## Создание Next.js у себя

На компьютере с Node.js в PowerShell, в папке для ваших проектов:

```powershell
npx create-next-app@16 hello-deployfox --typescript --app --eslint
Set-Location hello-deployfox
```

Ответьте на вопросы генератора. В next.config.ts включите `output: "standalone"` внутри объекта конфигурации. Убедитесь, что у приложения есть маршрут `/`, возвращающий 200 без обязательного входа. Добавьте Dockerfile:

```dockerfile
FROM node:24-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build
FROM node:24-alpine
WORKDIR /app
ENV NODE_ENV=production HOSTNAME=0.0.0.0 PORT=3000
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
USER node
EXPOSE 3000
CMD ["node", "server.js"]
```

Если используете public assets, отдельно добавьте COPY public; не добавляйте строку для отсутствующей папки. Создайте `.dockerignore` с node_modules, .next, .git и .env. Не копируйте секреты в образ.

Проверьте у себя:

```powershell
docker build -t hello-deployfox .
docker run --rm -p 127.0.0.1:3010:3000 hello-deployfox
```

Откройте `http://localhost:3010`. Для остановки Ctrl+C.

Создайте репозиторий на GitHub через интерфейс GitHub. Он покажет **настоящий** URL вашего репозитория. В папке проекта выполните `git add .`, `git commit -m "First app"`, добавьте remote по URL с вашей страницы GitHub и `git push -u origin main`. Не вставляйте выдуманный чужой remote.

В DeployFox создайте GitHub-проект: URL вашего репозитория, ветка main, Dockerfile `Dockerfile`, контекст `.`, порт 3000, healthcheck `/`, домен с A-записью на VPS. Нажмите Deploy, откройте логи, дождитесь SUCCESS, откройте HTTPS-домен.

Если Dockerfile отсутствует, агент использует Railpack 0.43.0 для определения Node.js, Go, Python и других поддерживаемых им проектов. Команда запуска должна быть определена в проекте. Это интеграция стороннего сборщика, а не универсальная гарантия для всех framework-конфигураций. [Механизм Railpack/BuildKit](https://railpack.com/platforms/running-railpack-in-production).
