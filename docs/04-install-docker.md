# Установка Docker Engine и Compose

Выполняйте в Ubuntu от пользователя deploy через sudo. Инструкция рассчитана на чистый VPS. Если Docker уже установлен, сначала выполните проверки версий ниже; не удаляйте существующие пакеты и данные вслепую.

Используется официальный репозиторий пакетов, а не `curl ... | sh`. Основа — [инструкция Docker для Ubuntu](https://docs.docker.com/engine/install/ubuntu/).

```bash
sudo apt update
sudo apt install ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
```

Затем скопируйте весь блок целиком:

```bash
sudo tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: noble
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
```

Установите движок, CLI, Buildx и Compose plugin:

```bash
sudo apt update
sudo apt install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
sudo docker version
sudo docker compose version
sudo docker buildx version
sudo docker run --rm hello-world
```

`hello-world` должен напечатать сообщение об успешном запуске. Если Docker сообщает о конфликтующих пакетах, сначала разберитесь, какие приложения от них зависят. Список конфликтов есть в официальной инструкции; автоматического удаления здесь нет.

Основные команды:

```bash
sudo docker ps
sudo docker ps -a
sudo docker compose -f docker-compose.prod.yml ps
sudo docker compose -f docker-compose.prod.yml logs --tail 100 api
sudo docker compose -f docker-compose.prod.yml restart api
```

Последние три выполняются из папки DeployFox. `ps` показывает контейнеры, `logs` — сообщения процесса, `restart` перезапускает. Доступ к Docker даёт высокие привилегии на VPS. Не добавляйте недоверенных людей в группу docker. В этой инструкции все административные Docker-команды выполняются через sudo.
