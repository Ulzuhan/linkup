#!/usr/bin/env bash
# Historical transfer verifier. Publication uses the gated oci-release.py only.
set -euo pipefail
if [[ $# != 1 || "$1" != '--verify-only' ]]; then
  echo 'Direct publication disabled; use the reviewed oci-release.py bootstrap lane.' >&2
  exit 2
fi

: "${RELEASE_LAYOUT:?OCI layout required}"
: "${RELEASE_DIGEST:?BuildKit digest required}"
: "${RELEASE_REPOSITORY:?Destination repository required}"
: "${RELEASE_TAGS:?At least one tag required}"

[[ -d "$RELEASE_LAYOUT" && ! -L "$RELEASE_LAYOUT" && -f "$RELEASE_LAYOUT/index.json" ]] || exit 2
[[ "$RELEASE_DIGEST" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 2
[[ "$RELEASE_REPOSITORY" =~ ^ghcr\.io/[a-z0-9][a-z0-9_.-]*/[a-z0-9][a-z0-9_.-]*$ ]] || exit 2
while IFS= read -r tag; do
  [[ "$tag" == "$RELEASE_REPOSITORY:"* ]] || exit 2
  suffix="${tag#"$RELEASE_REPOSITORY:"}"
  [[ "$suffix" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || exit 2
done <<< "$RELEASE_TAGS"

actual="sha256:$(skopeo inspect --raw "oci:$RELEASE_LAYOUT" | sha256sum | cut -d ' ' -f 1)"
if [[ "$actual" != "$RELEASE_DIGEST" ]]; then
  echo 'Layout digest does not match the scanned build; refusing publication.' >&2
  exit 1
fi
printf '%s\n' "$actual"
