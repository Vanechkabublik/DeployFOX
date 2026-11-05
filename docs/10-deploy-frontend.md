# React, Vite и Vue

Для статического frontend результат сборки обычно лежит в `dist`. Проверьте `npm run build` в вашем проекте. Next.js и Nuxt с серверным рендерингом не становятся статическими автоматически.

Пример Dockerfile для Vite/Vue:

```dockerfile
FROM node:24-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build
FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
```

Порт 80, healthcheck `/`. Для SPA с client-side router добавьте nginx-конфигурацию `try_files $uri $uri/ /index.html;`, иначе прямое открытие `/profile` может дать 404. API должен иметь свой HTTPS-домен; CORS на backend разрешите только нужному origin frontend.

Переменные VITE_* и NEXT_PUBLIC_* включаются в браузерную сборку и доступны посетителям. Не помещайте туда пароли баз и API private keys. Runtime environment в DeployFox не меняет уже собранные статические JS-файлы. В этой версии интерфейс не передаёт отдельные публичные build variables. Публичный адрес API задайте через безопасную конфигурацию приложения до сборки, затем пересоберите.

Nuxt обычно слушает 3000 и запускается через `.output/server/index.mjs`, но сверяйтесь с вашим preset. Используйте Dockerfile самого проекта. Для Next.js смотрите [первый деплой](08-deploy-first-project.md).
