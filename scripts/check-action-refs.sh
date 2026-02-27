#!/usr/bin/env bash
set -euo pipefail

token_header=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
  token_header=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
fi

tmp_body="$(mktemp)"
trap 'rm -f "$tmp_body"' EXIT

failed=0

while IFS= read -r spec; do
  case "$spec" in
    ./*|docker://*)
      continue
      ;;
  esac

  case "$spec" in
    *@*)
      repo="${spec%@*}"
      ref="${spec#*@}"
      ;;
    *)
      continue
      ;;
  esac

  url="https://api.github.com/repos/${repo}/commits/${ref}"
  http_code="$(
    curl -sS -o "$tmp_body" -w "%{http_code}" \
      -H "Accept: application/vnd.github+json" \
      "${token_header[@]}" \
      "$url"
  )"

  if [ "$http_code" -ge 400 ]; then
    echo "[invalid] ${spec} (HTTP ${http_code})"
    failed=1
  else
    echo "[ok] ${spec}"
  fi
done < <(
  rg -No --no-filename --pcre2 'uses:\s*([^\s#]+)' .github/workflows/*.yml -r '$1' \
    | sort -u
)

if [ "$failed" -ne 0 ]; then
  echo "One or more action references are not resolvable."
  exit 1
fi

echo "All external action references are resolvable."
