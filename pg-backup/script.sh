#!/usr/bin/env bash
set -euo pipefail

# Где лежит docker-compose.yml (важно!)
WORKDIR="/root/Repos/family-app-go"

# Postgres в docker-compose
SERVICE="db"
DB_USER="admin"
DB_NAME="family_app"

# Локально
BACKUP_DIR="/opt/family-app/backups"
RETENTION_DAYS_LOCAL=14

# Backblaze B2 (rclone remote)
RCLONE_REMOTE="b2"
B2_BUCKET_PATH="fmapp-pg-backups/family-app-postgres"
RETENTION_DAYS_REMOTE=30

TS="$(date +'%Y-%m-%d_%H-%M-%S')"
FILE="${BACKUP_DIR}/${DB_NAME}_${TS}.dump"

mkdir -p "$BACKUP_DIR"
cd "$WORKDIR"

echo "[INFO] Creating dump: $FILE"

# 1) Дамп (custom format)
docker compose exec -T "$SERVICE" pg_dump -U "$DB_USER" -Fc "$DB_NAME" > "$FILE"

echo "[INFO] Uploading to B2: ${RCLONE_REMOTE}:${B2_BUCKET_PATH}"

# 3) Upload в Backblaze B2
rclone copy "$FILE" "${RCLONE_REMOTE}:${B2_BUCKET_PATH}" \
  --transfers 2 --checkers 4 --retries 3 --low-level-retries 10 --stats 0

echo "[INFO] Local retention: ${RETENTION_DAYS_LOCAL} days"
# 4) Ротация локально
find "$BACKUP_DIR" -type f -name "*.dump" -mtime +"$RETENTION_DAYS_LOCAL" -delete

echo "[INFO] Remote retention: ${RETENTION_DAYS_REMOTE} days"
# 5) Ротация в B2 (удалить файлы старше N дней) + убрать пустые папки
rclone delete "${RCLONE_REMOTE}:${B2_BUCKET_PATH}" --min-age "${RETENTION_DAYS_REMOTE}d"
rclone rmdirs "${RCLONE_REMOTE}:${B2_BUCKET_PATH}" || true

echo "[OK] Backup done: $FILE"