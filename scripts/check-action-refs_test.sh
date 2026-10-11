#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck disable=SC1091
source "$root/scripts/check-action-refs.sh"

fail=0

expect_match() {
  local comment="$1" tag="$2" want="$3" got
  if comment_matches_tag "$comment" "$tag"; then
    got=yes
  else
    got=no
  fi
  if [ "$got" != "$want" ]; then
    echo "FAIL comment_matches_tag ${comment} ${tag} got=${got} want=${want}"
    fail=1
  fi
}

expect_match v6 v6 yes
expect_match v6 v6.0.2 yes
expect_match v6 v6-beta no
expect_match v6 v60 no
expect_match v1.7.12 v1.7.12 yes
expect_match v1.7.1 v1.7.12 no
expect_match v4 v4.35.1 yes
expect_match v8.1.1 v8 no
expect_match v8 v8.1.1 yes

expect_repo() {
  local in="$1" want="$2" got
  got="$(action_repo "$in")"
  if [ "$got" != "$want" ]; then
    echo "FAIL action_repo ${in} got=${got} want=${want}"
    fail=1
  fi
}

expect_repo actions/checkout actions/checkout
expect_repo github/codeql-action/init github/codeql-action
expect_repo github/codeql-action/autobuild github/codeql-action
expect_repo github/codeql-action/analyze github/codeql-action

expect_version() {
  local in="$1" want="$2" got
  if version_comment "$in"; then
    got=yes
  else
    got=no
  fi
  if [ "$got" != "$want" ]; then
    echo "FAIL version_comment ${in} got=${got} want=${want}"
    fail=1
  fi
}

expect_version v6 yes
expect_version v1.7.12 yes
expect_version v6.0.2 yes
expect_version latest no
expect_version V6 no
expect_version v no
expect_version 'v6.' no

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "check-action-refs unit checks: ok"
