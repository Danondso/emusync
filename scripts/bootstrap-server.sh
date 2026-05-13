#!/usr/bin/env bash
# One-shot Docker Compose bootstrap for emusync server.
# See README "Quick Start" and tests/bootstrap/server_bootstrap_test.go.
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: bootstrap-server.sh [--force-token]

Options:
  --force-token   Write a new EMUSYNC_AUTH_TOKEN even when .env exists.
                  WARNING: every client must be updated with the new token.

Environment:
  EMUSYNC_REPO_ROOT   Project directory containing docker-compose.yml (default:
                      parent of this script's directory).

Docker must be installed with the Compose v2 plugin ("docker compose").
EOF
}

FORCE_TOKEN=false
while [[ $# -gt 0 ]]; do
	case "$1" in
	--force-token)
		FORCE_TOKEN=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "bootstrap-server.sh: unknown argument: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [[ -n "${EMUSYNC_REPO_ROOT:-}" ]]; then
	REPO_ROOT=$(cd "$EMUSYNC_REPO_ROOT" && pwd)
else
	REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
fi
cd "$REPO_ROOT"

COMPOSE_FILE="$REPO_ROOT/docker-compose.yml"
if [[ ! -f "$COMPOSE_FILE" ]]; then
	echo "bootstrap-server.sh: docker-compose.yml not found in $REPO_ROOT" >&2
	exit 1
fi

check_docker() {
	if ! command -v docker >/dev/null 2>&1; then
		cat <<'EOF' >&2
bootstrap-server.sh: 'docker' not found.

Install Docker Engine for your OS, then ensure your user can run docker
(e.g. membership in the 'docker' group) or run this script with suitable privileges.
EOF
		exit 1
	fi
	if ! docker compose version >/dev/null 2>&1; then
		cat <<'EOF' >&2
bootstrap-server.sh: 'docker compose' is not working.

Install Docker Compose v2 (the "docker compose" plugin). Standalone "docker-compose"
v1 is not invoked by this script — use Docker's plugin or symlink compatibility shim.
EOF
		exit 1
	fi
}

generate_token() {
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex 24
		return 0
	fi
	if command -v od >/dev/null 2>&1; then
		# 24 random bytes as hex (48 chars), no openssl required
		od -An -N24 -tx1 /dev/urandom | tr -d ' \n'
		echo
		return 0
	fi
	echo "bootstrap-server.sh: need openssl or od to generate a token" >&2
	exit 1
}

ensure_env() {
	local env_file="$REPO_ROOT/.env"
	if [[ -f "$env_file" ]] && [[ "$FORCE_TOKEN" != true ]]; then
		return 0
	fi
	if [[ -f "$env_file" ]] && [[ "$FORCE_TOKEN" == true ]]; then
		echo "bootstrap-server.sh: Rotating EMUSYNC_AUTH_TOKEN (--force-token)." >&2
		echo "bootstrap-server.sh: Update auth_token on every client before syncing." >&2
	fi
	local token
	token="$(generate_token)"
	token="$(echo -n "$token" | tr -d '\n\r')"
	printf 'EMUSYNC_AUTH_TOKEN=%s\n' "$token" >"$env_file"
	chmod 600 "$env_file"
}

extract_token() {
	local env_file="$REPO_ROOT/.env"
	grep '^EMUSYNC_AUTH_TOKEN=' "$env_file" | head -n1 | cut -d= -f2-
}

check_docker
ensure_env

echo "bootstrap-server.sh: building and starting stack (docker compose up -d --build)..." >&2
docker compose -f "$COMPOSE_FILE" up -d --build

TOKEN="$(extract_token)"
echo ""
echo "emusync server is starting."
echo "  Health:   http://127.0.0.1:8080/api/v1/health"
echo "  Use this auth token in client config (server.auth_token):"
echo "  $TOKEN"
echo ""
echo "Next: edit ~/.config/emusync/config.toml on each client (see README)."
