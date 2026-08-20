#!/bin/bash

set -eu

DEBUG="${DEBUG:-false}"

write_env() {
    local name=$1
    local value=$2
    [ -n "$value" ] || return 0
    case "$value" in
        *$'\n'*|*$'\r'*)
            echo "ERROR: $name contains a newline" >&2
            exit 1
            ;;
    esac
    printf '%s=%s\n' "$name" "$value"
}

env_tmp=$(mktemp .env.tmp.XXXXXX)
trap 'rm -f "$env_tmp"' EXIT
{
    write_env AWS_ACCESS_KEY_ID "${AWS_ACCESS_KEY_ID:-}"
    write_env AWS_SECRET_ACCESS_KEY "${AWS_SECRET_ACCESS_KEY:-}"
    write_env AWS_SESSION_TOKEN "${AWS_SESSION_TOKEN:-}"
    write_env AWS_REGION "${AWS_REGION:-}"
    write_env AWS_DEFAULT_REGION "${AWS_DEFAULT_REGION:-}"
} > "$env_tmp"
chmod 600 "$env_tmp"
mv -f "$env_tmp" .env
trap - EXIT

if [ "false" != "${DEBUG}" ]; then
    sed -E 's/(.*SECRET.*|.*TOKEN.*|.*PASSWORD.*|.*KEY_ID.*|.*ACCESS_KEY.*|.*PEM.*)=.*/\1=<REDACTED>/' .env
fi
