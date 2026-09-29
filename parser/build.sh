#!/usr/bin/env bash
set -euo pipefail

platform="${1:-linux/amd64}"
platform_name="${platform//\//-}"
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
output_dir="$script_dir/dist"
temporary_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$temporary_dir"
}
trap cleanup EXIT

mkdir -p "$output_dir"

docker buildx build \
  --platform "$platform" \
  --output "type=local,dest=$temporary_dir" \
  --file "$script_dir/Dockerfile" \
  "$script_dir"

mv "$temporary_dir/walkthrough-parser" "$output_dir/walkthrough-parser-$platform_name"
chmod +x "$output_dir/walkthrough-parser-$platform_name"

printf 'Built %s\n' "$output_dir/walkthrough-parser-$platform_name"
