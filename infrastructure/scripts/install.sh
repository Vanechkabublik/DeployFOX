#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
trap 'printf "Ошибка установки на строке %s. Данные и volumes не удалялись.\n" "$LINENO" >&2' ERR
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
fail() { printf 'Ошибка: %s\n' "$*" >&2; exit 1; }
[[ -f /etc/os-release ]] || fail 'Нужна Ubuntu 24.04.'
# shellcheck disable=SC1091
source /etc/os-release
[[ "$ID" == ubuntu && "$VERSION_ID" == 24.04 ]] || fail 'Поддерживается Ubuntu 24.04 LTS.'
[[ "$EUID" -eq 0 ]] || fail 'Запустите sudo bash infrastructure/scripts/install.sh'
command -v docker >/dev/null || fail 'Сначала установите Docker по docs/04-install-docker.md'
docker info >/dev/null || fail 'Docker Engine недоступен.'
docker compose version >/dev/null || fail 'Не установлен Compose plugin.'
command -v openssl >/dev/null || fail 'Установите openssl: sudo apt install openssl'
ram_mb=$(awk '/MemTotal/{print int($2/1024)}' /proc/meminfo)
disk_kb=$(df -Pk . | awk 'NR==2 {print $4}')
(( ram_mb >= 3500 )) || fail 'Нужно минимум 4 GB RAM; рекомендуется 8 GB.'
(( disk_kb >= 15728640 )) || fail 'Нужно минимум 15 GB свободного места.'
if [[ ! -f .env ]]; then
  if ss -ltnH '( sport = :80 or sport = :443 )' | grep -q .; then fail 'Порт 80 или 443 занят. Проверьте sudo ss -ltnp; не останавливайте неизвестные службы.'; fi
  read -rp 'Домен панели (например deploy.example.com): ' panel_domain
  [[ "$panel_domain" =~ ^[a-z0-9][a-z0-9.-]+\.[a-z]{2,63}$ ]] || fail 'Неверный домен.'
  read -rp 'Email для HTTPS-сертификатов: ' acme_email
  read -rp 'Публичный IPv4 VPS (из панели провайдера, для проверки DNS): ' public_ip
  [[ "$public_ip" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || fail 'Неверный IPv4.'
  [[ "$acme_email" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]] || fail 'Неверный email.'
  {
    printf 'PANEL_DOMAIN=%s\nPUBLIC_URL=https://%s\nACME_EMAIL=%s\n' "$panel_domain" "$panel_domain" "$acme_email"
    printf 'SERVER_IP=%s\n' "$public_ip"
    for key in POSTGRES_PASSWORD REDIS_PASSWORD ENCRYPTION_KEY SETUP_TOKEN AGENT_TOKEN GITHUB_WEBHOOK_SECRET; do printf '%s=%s\n' "$key" "$(openssl rand -hex 32)"; done
    printf 'GITHUB_APP_ID=\nGITHUB_APP_SLUG=\nGITHUB_INSTALLATION_ID=\nGITHUB_PRIVATE_KEY_FILE=./secrets/github-app.pem\n'
  } > .env
fi
chmod 600 .env
mkdir -p secrets backups
[[ -e secrets/github-app.pem ]] || install -m 600 /dev/null secrets/github-app.pem
compose=(docker compose -f docker-compose.prod.yml)
"${compose[@]}" config --quiet
# A dedicated BuildKit daemon has explicit resource limits. Existing builders are reused.
if ! docker buildx inspect deployfox >/dev/null 2>&1; then
  docker buildx create --name deployfox --driver docker-container --driver-opt memory=2g,cpu-period=100000,cpu-quota=100000 >/dev/null
fi
docker buildx inspect deployfox --bootstrap >/dev/null
"${compose[@]}" up -d --build --wait --wait-timeout 300
printf '\nПанель: %s\n' "$(sed -n 's/^PUBLIC_URL=//p' .env)"
printf 'Токен инициализации (храните в секрете): '
sed -n 's/^SETUP_TOKEN=//p' .env
printf '\nСоздайте администратора. Инструкции: docs/00-start-here.md\n'
