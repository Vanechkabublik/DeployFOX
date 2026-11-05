#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
compose_file=${COMPOSE_FILE:-docker-compose.prod.yml}
compose=(docker compose -f "$compose_file")
if [[ -n "${COMPOSE_OVERRIDE_FILE:-}" ]]; then compose+=(-f "$COMPOSE_OVERRIDE_FILE"); fi
backup_dir="backups/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$backup_dir"
"${compose[@]}" exec -T postgres pg_dump -U deployfox -d deployfox -Fc > "$backup_dir/deployfox.dump.part"
mv "$backup_dir/deployfox.dump.part" "$backup_dir/deployfox.dump"
"${compose[@]}" exec -T postgres pg_restore --list < "$backup_dir/deployfox.dump" > "$backup_dir/contents.txt"
if [[ -f .env ]]; then cp .env "$backup_dir/config.env"; fi
if [[ "$compose_file" == docker-compose.local.yml ]]; then
  MSYS_NO_PATHCONV=1 "${compose[@]}" exec -T api cat /run/deployfox/env > "$backup_dir/local-secrets.env"
fi
cp "$compose_file" "$backup_dir/compose.yml"
cp -a infrastructure/caddy "$backup_dir/"
[[ ! -d secrets ]] || cp -a secrets "$backup_dir/"
(cd "$backup_dir" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS)
printf 'Копия: %s\n' "$backup_dir"
printf 'Это копия управляющей базы и конфигурации. Пользовательские volumes и маршруты Caddy сюда не входят.\n'
printf 'Restore не считается проверенным до запуска restore.sh на отдельной базе.\n'
