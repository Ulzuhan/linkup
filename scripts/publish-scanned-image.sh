#!/usr/bin/env bash
# Publish an already-scanned OCI layout; never rebuild or transform its digest.
# Credentials remain in the Docker login configuration, never in argv or stdout.
set -euo pipefail

: "${RELEASE_LAYOUT:?OCI layout required}"
: "${RELEASE_DIGEST:?BuildKit digest required}"
: "${RELEASE_REPOSITORY:?Destination repository required}"
: "${RELEASE_TAGS:?At least one tag required}"

[[ -d "$RELEASE_LAYOUT" && ! -L "$RELEASE_LAYOUT" && -f "$RELEASE_LAYOUT/index.json" ]] || exit 2
[[ "$RELEASE_DIGEST" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 2
[[ "$RELEASE_REPOSITORY" =~ ^ghcr\.io/[a-z0-9][a-z0-9_.-]*/[a-z0-9][a-z0-9_.-]*$ ]] || exit 2
mapfile -t tags <<< "$RELEASE_TAGS"
for tag in "${tags[@]}"; do
  [[ "$tag" == "$RELEASE_REPOSITORY:"* ]] || exit 2
  suffix="${tag#"$RELEASE_REPOSITORY:"}"
  [[ "$suffix" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || exit 2
done

actual="sha256:$(skopeo inspect --raw "oci:$RELEASE_LAYOUT" | sha256sum | cut -d ' ' -f 1)"
if [[ "$actual" != "$RELEASE_DIGEST" ]]; then
  echo 'Layout digest does not match the scanned build; refusing publication.' >&2
  exit 1
fi
if [[ "${1:-}" == '--verify-only' ]]; then
  printf '%s\n' "$actual"
  exit 0
fi
[[ $# == 0 ]] || exit 2

authfile="${DOCKER_CONFIG:-$HOME/.docker}/config.json"
[[ -f "$authfile" ]] || exit 2
digestfile="$(mktemp "${TMPDIR:-/tmp}/linkup-publish-digest.XXXXXXXX")"
trap 'rm -f -- "$digestfile"' EXIT
for tag in "${tags[@]}"; do
  skopeo copy --all --preserve-digests --authfile "$authfile" \
    --digestfile "$digestfile" "oci:$RELEASE_LAYOUT" "docker://$tag" >&2
  [[ "$(< "$digestfile")" == "$RELEASE_DIGEST" ]] || exit 1
done
printf '%s\n' "$actual"
