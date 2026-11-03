#!/bin/sh
# Railway / Docker entrypoint — generates /app/config.yml from environment variables,
# then exec's the application binary.
set -e

# ---------------------------------------------------------------------------
# Parse Redis URL (Railway format: redis://:password@host:port/db)
# ---------------------------------------------------------------------------
if [ -n "$REDIS_URL" ]; then
  _redis_auth=$(echo "$REDIS_URL" | sed 's|redis://\(.*\)@.*|\1|')
  _redis_host=$(echo "$REDIS_URL" | sed 's|redis://.*@\(.*\):[0-9]*.*|\1|')
  _redis_port=$(echo "$REDIS_URL" | sed 's|redis://.*@.*:\([0-9]*\).*|\1|')
  _redis_pass=$(echo "$_redis_auth" | sed 's|^:||')
  REDIS_HOST="${REDIS_HOST:-$_redis_host}"
  REDIS_PORT="${REDIS_PORT:-$_redis_port}"
  REDIS_PASSWORD="${REDIS_PASSWORD:-$_redis_pass}"
fi

# ---------------------------------------------------------------------------
# Resolve database — prefer explicit DSN, fall back to Railway's DATABASE_URL
# ---------------------------------------------------------------------------
if [ -z "$DATABASE_DSN" ] && [ -n "$DATABASE_URL" ]; then
  DATABASE_DSN="$DATABASE_URL"
  DATABASE_DRIVER="${DATABASE_DRIVER:-postgres}"
fi

# Validate required secrets
: "${APP_SECRET_KEY:?APP_SECRET_KEY env var is required (min 32 chars, unique)}"
: "${JWT_SECRET:?JWT_SECRET env var is required (min 32 chars, unique)}"
: "${USER_JWT_SECRET:?USER_JWT_SECRET env var is required (min 32 chars, unique)}"
: "${DATABASE_DSN:?DATABASE_DSN (or DATABASE_URL) env var is required}"

cat > /app/config.yml << YAML
app:
  secret_key: "${APP_SECRET_KEY}"
  totp_issuer: "${APP_TOTP_ISSUER:-Dujiao-Next}"

server:
  host: "0.0.0.0"
  port: "${PORT:-8080}"
  mode: "${SERVER_MODE:-release}"
  # Trust all upstream proxies — correct for Railway's edge network.
  # Tighten to a specific CIDR if you expose the container directly.
  trusted_proxies:
    - "0.0.0.0/0"
    - "::/0"

log:
  filename: app.log
  max_size_mb: 100
  max_backups: 7
  max_age_days: 30
  compress: true

database:
  driver: "${DATABASE_DRIVER:-postgres}"
  dsn: "${DATABASE_DSN}"
  pool:
    max_open_conns: ${DB_MAX_OPEN_CONNS:-25}
    max_idle_conns: ${DB_MAX_IDLE_CONNS:-10}
    conn_max_lifetime_seconds: 300
    conn_max_idle_time_seconds: 60

jwt:
  secret: "${JWT_SECRET}"
  expire_hours: ${JWT_EXPIRE_HOURS:-24}

user_jwt:
  secret: "${USER_JWT_SECRET}"
  expire_hours: ${USER_JWT_EXPIRE_HOURS:-24}
  remember_me_expire_hours: ${USER_JWT_REMEMBER_ME_HOURS:-168}

bootstrap:
  default_admin_username: "${ADMIN_USERNAME:-}"
  default_admin_password: "${ADMIN_PASSWORD:-}"

redis:
  enabled: ${REDIS_ENABLED:-true}
  host: "${REDIS_HOST:-127.0.0.1}"
  port: ${REDIS_PORT:-6379}
  password: "${REDIS_PASSWORD:-}"
  db: ${REDIS_DB:-0}
  prefix: "dj"

queue:
  enabled: ${QUEUE_ENABLED:-true}
  host: "${REDIS_HOST:-127.0.0.1}"
  port: ${REDIS_PORT:-6379}
  password: "${REDIS_PASSWORD:-}"
  db: ${QUEUE_DB:-1}
  concurrency: 10
  queues:
    default: 10
    critical: 5
  upstream_sync_interval: "5m"

upload:
  max_size: 10485760
  allowed_types:
    - image/jpeg
    - image/png
    - image/gif
    - image/webp
  allowed_extensions:
    - .jpg
    - .jpeg
    - .png
    - .gif
    - .webp
  max_width: 4096
  max_height: 4096

cors:
  allowed_origins:
    - "*"
  allow_credentials: true
  max_age: 600

security:
  login_rate_limit:
    window_seconds: 300
    max_attempts: 5
    block_seconds: 900
  password_policy:
    min_length: 8
    require_upper: true
    require_lower: true
    require_number: true
    require_special: false

email:
  enabled: ${EMAIL_ENABLED:-false}
  host: "${EMAIL_HOST:-}"
  port: ${EMAIL_PORT:-587}
  username: "${EMAIL_USERNAME:-}"
  password: "${EMAIL_PASSWORD:-}"
  from: "${EMAIL_FROM:-}"
  from_name: "${EMAIL_FROM_NAME:-}"
  use_tls: true
  use_ssl: false
  verify_code:
    expire_minutes: 10
    send_interval_seconds: 60
    max_attempts: 5
    length: 6

order:
  payment_expire_minutes: 15
  max_refund_days: 30

web:
  admin_path: "${WEB_ADMIN_PATH:-/admin}"

reseller:
  enabled: false
  main_hosts:
    - localhost
    - 127.0.0.1
    - "::1"
  trusted_forwarded_host: false
  subdomain_base: ""
  self_apply_enabled: true
  settlement_confirm_days: 7
YAML

exec ./dujiao-next "$@"
