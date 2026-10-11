#!/usr/bin/env bash
set -euo pipefail

# action_repo turns "owner/repo" or "owner/repo/path" into "owner/repo".
action_repo() {
  local path="$1"
  local owner rest
  owner="${path%%/*}"
  rest="${path#*/}"
  printf '%s/%s' "$owner" "${rest%%/*}"
}

# comment_matches_tag reports whether a pin comment agrees with a tag on that commit.
# "v6" matches tag "v6" and "v6.0.2", and does not match "v60" or "v6-beta".
comment_matches_tag() {
  local comment="$1" tag="$2"
  if [ "$tag" = "$comment" ]; then
    return 0
  fi
  case "$tag" in
    "$comment".*) return 0 ;;
  esac
  return 1
}

version_comment() {
  [[ "$1" =~ ^v[0-9]+(\.[0-9]+)*$ ]]
}

if [ "${BASH_SOURCE[0]}" != "$0" ]; then
  return 0
fi

token_header=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
  token_header=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
fi

tmp_body="$(mktemp)"
trap 'rm -f "$tmp_body"' EXIT

failed=0

api_get() {
  local url="$1"
  curl -sS -o "$tmp_body" -w "%{http_code}" \
    -H "Accept: application/vnd.github+json" \
    -H "User-Agent: gog-lite-action-ref-check" \
    "${token_header[@]}" \
    "$url"
}

# find_matching_tag prints the tag name and returns 0, or returns 1 when no
# tag on the commit matches the comment. HTTP failures return 2.
find_matching_tag() {
  local repo="$1" sha="$2" comment="$3"
  local page=1 http_code parsed count found_tag tag_sha tag_name

  while [ "$page" -le 20 ]; do
    http_code="$(api_get "https://api.github.com/repos/${repo}/tags?per_page=100&page=${page}")"
    if [ "$http_code" -ge 400 ]; then
      echo "tag lookup HTTP ${http_code} for ${repo}" >&2
      return 2
    fi

    if ! parsed="$(python3 -c '
import json, sys
with open(sys.argv[1]) as f:
    tags = json.load(f)
if not isinstance(tags, list):
    sys.exit(2)
for t in tags:
    name = t.get("name") or ""
    commit = (t.get("commit") or {}).get("sha") or ""
    if name and commit:
        sys.stdout.write(commit + "\t" + name + "\n")
' "$tmp_body")"; then
      echo "tag lookup returned a non-list body for ${repo}" >&2
      return 2
    fi

    count=0
    found_tag=""
    while IFS=$'\t' read -r tag_sha tag_name; do
      [ -n "$tag_sha" ] || continue
      count=$((count + 1))
      if [ "$tag_sha" = "$sha" ] && comment_matches_tag "$comment" "$tag_name"; then
        found_tag="$tag_name"
        break
      fi
    done <<EOF
$parsed
EOF

    if [ -n "$found_tag" ]; then
      printf '%s\n' "$found_tag"
      return 0
    fi
    if [ "$count" -lt 100 ]; then
      return 1
    fi
    page=$((page + 1))
  done
  return 1
}

check_spec() {
  local spec="$1" comment="${2:-}"
  local path ref repo http_code sha tag rc

  case "$spec" in
    ./*|docker://*)
      return 0
      ;;
  esac
  case "$spec" in
    *@*) ;;
    *) return 0 ;;
  esac

  path="${spec%@*}"
  ref="${spec##*@}"
  repo="$(action_repo "$path")"

  http_code="$(api_get "https://api.github.com/repos/${repo}/commits/${ref}")"
  if [ "$http_code" -ge 400 ]; then
    echo "[invalid] ${spec} (HTTP ${http_code})"
    failed=1
    return 0
  fi

  sha="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("sha",""))' < "$tmp_body")"
  if [ -z "$sha" ]; then
    echo "[invalid] ${spec} (commit SHA missing in response)"
    failed=1
    return 0
  fi

  if [ -z "$comment" ]; then
    echo "[invalid] ${spec} (missing version comment)"
    failed=1
    return 0
  fi
  if ! version_comment "$comment"; then
    echo "[invalid] ${spec} (version comment '${comment}' is not vN[.N.N])"
    failed=1
    return 0
  fi

  tag=""
  rc=0
  tag="$(find_matching_tag "$repo" "$sha" "$comment")" || rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "[ok] ${spec} # ${comment} (${tag})"
  elif [ "$rc" -eq 2 ]; then
    echo "[invalid] ${spec} # ${comment} (tag lookup failed)"
    failed=1
  else
    echo "[invalid] ${spec} # ${comment} (no tag on ${sha} matches ${comment})"
    failed=1
  fi
}

while IFS= read -r line; do
  spec="${line%%|*}"
  if [ "$line" = "$spec" ]; then
    comment=""
  else
    comment="${line#*|}"
  fi
  check_spec "$spec" "$comment"
done < <(
  rg -N -o --no-filename --pcre2 'uses:\s+(\S+)(?:\s+#\s*(\S+))?' .github/workflows/*.yml -r '$1|$2' \
    | sort -u
)

if [ "$failed" -ne 0 ]; then
  echo "One or more action references are not resolvable or do not match their version comment."
  exit 1
fi

echo "All external action references are resolvable and match their version comments."
