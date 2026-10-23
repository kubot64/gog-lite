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

# ref_is_full_sha reports whether the pin is a full-length commit SHA.
# GitHub accepts 40 hex digits (SHA-1) and 64 hex digits (SHA-256).
ref_is_full_sha() {
  case "$1" in
    *[!0-9a-fA-F]*) return 1 ;;
  esac
  [ "${#1}" -eq 40 ] || [ "${#1}" -eq 64 ]
}

# extract_action_refs prints "spec|comment" lines for uses: pins under dir.
# The comment is empty when the line has no "# vN" pin comment.
# Exit status is non-zero when the directory or the extractor cannot be read.
# Uses POSIX awk and sort so the check does not depend on rg.
extract_action_refs() {
  local dir="$1"
  local files=()
  local f
  if [ ! -d "$dir" ]; then
    echo "workflow directory not found: ${dir}" >&2
    return 1
  fi
  for f in "$dir"/*.yml "$dir"/*.yaml; do
    if [ -f "$f" ]; then
      files+=("$f")
    fi
  done
  if [ "${#files[@]}" -eq 0 ]; then
    echo "no workflow files in ${dir}" >&2
    return 1
  fi
  awk '
    {
      line = $0
      sub(/\r$/, "", line)
      if (!match(line, /uses:[[:space:]]*[^[:space:]]+/)) {
        next
      }
      spec = substr(line, RSTART, RLENGTH)
      sub(/^uses:[[:space:]]*/, "", spec)
      rest = substr(line, RSTART + RLENGTH)
      comment = ""
      if (match(rest, /^[[:space:]]+#[[:space:]]*[^[:space:]]+/)) {
        comment = substr(rest, RSTART, RLENGTH)
        sub(/^[[:space:]]*#[[:space:]]*/, "", comment)
      }
      if (spec != "") {
        printf "%s|%s\n", spec, comment
      }
    }
  ' "${files[@]}" | LC_ALL=C sort -u
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
    *@*)
      path="${spec%@*}"
      ref="${spec##*@}"
      ;;
    *)
      echo "[invalid] ${spec} (ref is not a full commit SHA)"
      failed=1
      return 0
      ;;
  esac

  if ! ref_is_full_sha "$ref"; then
    echo "[invalid] ${spec} (ref is not a full commit SHA)"
    failed=1
    return 0
  fi

  repo="$(action_repo "$path")"

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

# check_workflow_dir validates every uses: reference under dir.
# Extraction failure and zero references both fail the check.
check_workflow_dir() {
  local dir="$1"
  local extracted line spec comment
  failed=0
  if ! extracted="$(extract_action_refs "$dir")"; then
    echo "failed to extract action references from ${dir}" >&2
    return 1
  fi
  if [ -z "$extracted" ]; then
    echo "no action references found in ${dir}" >&2
    return 1
  fi
  while IFS= read -r line || [ -n "$line" ]; do
    [ -n "$line" ] || continue
    spec="${line%%|*}"
    if [ "$line" = "$spec" ]; then
      comment=""
    else
      comment="${line#*|}"
    fi
    check_spec "$spec" "$comment"
  done <<EOF
$extracted
EOF
  if [ "$failed" -ne 0 ]; then
    echo "One or more action references are not resolvable or do not match their version comment."
    return 1
  fi
  echo "All external action references are resolvable and match their version comments."
  return 0
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

if ! check_workflow_dir ".github/workflows"; then
  exit 1
fi
