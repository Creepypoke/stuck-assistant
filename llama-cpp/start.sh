#!/usr/bin/env bash
set -euo pipefail

if ! command -v llama >/dev/null 2>&1; then
  printf 'llama-server was not found in PATH. Install llama.cpp or add it to PATH.\n' >&2
  exit 127
fi

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

exec llama serve \
  --host 127.0.0.1 \
  --port 8081 \
  --models-preset "$script_dir/models.ini" \
  --models-max 1
