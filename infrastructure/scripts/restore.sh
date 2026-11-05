#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $# -eq 1 ]] || { echo 'Использование: restore.sh /полный/путь/к/копии'; exit 1; }
restore_dir=$(realpath "$1")
[[ -f "$restore_dir/SHA256SUMS" && -f "$restore_dir/deployfox.dump" ]] || { echo 'Неполная копия'; exit 1; }
(cd "$restore_dir" && sha256sum -c SHA256SUMS)
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
compose=(docker compose -f "${COMPOSE_FILE:-docker-compose.prod.yml}")
if [[ -n "${COMPOSE_OVERRIDE_FILE:-}" ]]; then compose+=(-f "$COMPOSE_OVERRIDE_FILE"); fi
printf 'ВНИМАНИЕ: данные управляющей базы будут заменены. Пользовательские контейнеры автоматически не восстанавливаются.\n'
read -rp 'Введите RESTORE для подтверждения: ' confirm
[[ "$confirm" == RESTORE ]] || exit 1
"${compose[@]}" stop api worker frontend
trap 'echo "При ошибке api/worker/frontend остаются остановленными: проверьте восстановление перед запуском." >&2' ERR
"${compose[@]}" exec -T postgres pg_restore -U deployfox -d deployfox --clean --if-exists --exit-on-error < "$restore_dir/deployfox.dump"
"${compose[@]}" up -d --wait api worker frontend
printf 'База восстановлена. Проверьте ключ шифрования, вход, проекты и работоспособность приложений.\n'
