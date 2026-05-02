#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env.worktree}"

if [ -f "$ENV_FILE" ] && [ "${FORCE:-0}" != "1" ]; then
  echo "Refusing to overwrite existing $ENV_FILE. Re-run with FORCE=1 if you want to regenerate it."
  exit 1
fi

worktree_name="${WORKTREE_NAME:-$(basename "$PWD")}"
slug="$(printf '%s' "$worktree_name" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/_/g; s/__*/_/g; s/^_//; s/_$//')"
if [ -z "$slug" ]; then
  slug="multica"
fi

hash_value="$(printf '%s' "$PWD" | cksum | awk '{print $1}')"
offset=$((hash_value % 1000))

postgres_db="multica_${slug}_${offset}"
postgres_port=5432
postgres_user="${POSTGRES_USER:-}"
postgres_password="${POSTGRES_PASSWORD:-}"
backend_port=$((18080 + offset))
frontend_port=$((13000 + offset))
frontend_origin="http://localhost:${frontend_port}"

docker_cmd=(docker)

resolve_docker_access() {
  if docker version > /dev/null 2>&1; then
    return 0
  fi

  if command -v sudo > /dev/null 2>&1 && sudo -n docker version > /dev/null 2>&1; then
    docker_cmd=(sudo -n docker)
    return 0
  fi

  return 1
}

run_docker() {
  "${docker_cmd[@]}" "$@"
}

load_shared_postgres_credentials() {
  local container_id shared_env line

  if [ -n "$postgres_user" ] && [ -n "$postgres_password" ]; then
    return 0
  fi

  if ! resolve_docker_access; then
    return 0
  fi

  container_id="$(run_docker compose ps -q postgres 2> /dev/null || true)"
  if [ -z "$container_id" ]; then
    return 0
  fi

  shared_env="$(run_docker inspect "$container_id" --format '{{range .Config.Env}}{{println .}}{{end}}' 2> /dev/null || true)"
  if [ -z "$shared_env" ]; then
    return 0
  fi

  while IFS= read -r line; do
    case "$line" in
      POSTGRES_USER=*)
        if [ -z "$postgres_user" ]; then
          postgres_user="${line#POSTGRES_USER=}"
        fi
        ;;
      POSTGRES_PASSWORD=*)
        if [ -z "$postgres_password" ]; then
          postgres_password="${line#POSTGRES_PASSWORD=}"
        fi
        ;;
    esac
  done <<EOF
$shared_env
EOF
}

load_shared_postgres_credentials
postgres_user="${postgres_user:-multica}"
postgres_password="${postgres_password:-multica}"

cat > "$ENV_FILE" <<EOF
POSTGRES_DB=${postgres_db}
POSTGRES_USER=${postgres_user}
POSTGRES_PASSWORD=${postgres_password}
POSTGRES_PORT=${postgres_port}
DATABASE_URL=postgres://${postgres_user}:${postgres_password}@localhost:${postgres_port}/${postgres_db}?sslmode=disable

PORT=${backend_port}
JWT_SECRET=change-me-in-production
MULTICA_SERVER_URL=ws://localhost:${backend_port}/ws
MULTICA_APP_URL=${frontend_origin}

GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
GOOGLE_REDIRECT_URI=${frontend_origin}/auth/callback

FRONTEND_PORT=${frontend_port}
FRONTEND_ORIGIN=${frontend_origin}
NEXT_PUBLIC_API_URL=http://localhost:${backend_port}
NEXT_PUBLIC_WS_URL=ws://localhost:${backend_port}/ws
EOF

echo "Generated $ENV_FILE for worktree '$worktree_name'"
echo "  Shared Postgres: localhost:${postgres_port}"
echo "  Database: ${postgres_db}"
echo "  Backend:  http://localhost:${backend_port}"
echo "  Frontend: ${frontend_origin}"
echo ""
echo "Next steps:"
echo "  make setup-worktree"
echo "  make start-worktree"
