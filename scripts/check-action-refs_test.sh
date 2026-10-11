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

expect_sha() {
  local in="$1" want="$2" got
  if ref_is_full_sha "$in"; then
    got=yes
  else
    got=no
  fi
  if [ "$got" != "$want" ]; then
    echo "FAIL ref_is_full_sha ${in} got=${got} want=${want}"
    fail=1
  fi
}

sha40=de0fac2e4500dabe0009e67214ff5f5447ce83dd
sha64="$(printf 'a%.0s' {1..64})"
expect_sha "$sha40" yes
expect_sha "$sha64" yes
expect_sha "$(printf '%s' "$sha40" | tr '[:lower:]' '[:upper:]')" yes
expect_sha "${sha40%?}" no
expect_sha "${sha64}a" no
expect_sha v6 no
expect_sha "" no

expect_invalid() {
  local spec="$1" comment="$2" needle="$3" out
  out="$(mktemp)"
  failed=0
  check_spec "$spec" "$comment" >"$out"
  if [ "$failed" -eq 0 ]; then
    echo "FAIL check_spec ${spec} did not fail"
    fail=1
  fi
  if ! grep -q "$needle" "$out"; then
    echo "FAIL check_spec ${spec} missing ${needle}"
    cat "$out"
    fail=1
  fi
  rm -f "$out"
}

expect_invalid "actions/checkout@v6" "v6" "not a full commit SHA"
expect_invalid "actions/checkout" "" "not a full commit SHA"
expect_invalid "actions/checkout@${sha40}" "" "missing version comment"
expect_invalid "actions/checkout@${sha40}" "latest" "is not vN"

expect_extract() {
  local name="$1" want="$2" dir got_file want_file
  dir="$(mktemp -d)"
  got_file="$(mktemp)"
  want_file="$(mktemp)"
  mkdir -p "$dir"
  printf '%s\n' "$3" >"$dir/ci.yml"
  if [ -n "${4:-}" ]; then
    printf '%s\n' "$4" >"$dir/other.yml"
  fi
  printf '%s\n' "$want" >"$want_file"
  if ! extract_action_refs "$dir" >"$got_file"; then
    echo "FAIL extract ${name} exited non-zero"
    fail=1
  elif ! cmp -s "$want_file" "$got_file"; then
    echo "FAIL extract ${name} mismatch"
    diff -u "$want_file" "$got_file" || true
    fail=1
  fi
  rm -rf "$dir" "$got_file" "$want_file"
}

expect_extract "pins" \
"actions/checkout@${sha40}|v6
github/codeql-action/init@c10b8064de6f491fea524254123dbe5e09572f13|v4" \
"  - uses: actions/checkout@${sha40} # v6
  - uses: actions/checkout@${sha40} # v6
    uses: github/codeql-action/init@c10b8064de6f491fea524254123dbe5e09572f13 # v4"

expect_extract "local and no comment" \
"./.github/actions/local|
docker://alpine:3.20|" \
"uses: ./.github/actions/local
uses: docker://alpine:3.20"

if extract_action_refs "/no/such/workflows-$$" >/dev/null 2>&1; then
  echo "FAIL extract of missing directory succeeded"
  fail=1
fi

# count_unique_uses lists unique uses pins with grep and sed.
# It does not call extract_action_refs. Duplicates collapse, and local
# actions and docker:// pins stay in the count, matching extraction.
count_unique_uses() {
  local dir="$1"
  local files=()
  local f
  for f in "$dir"/*.yml "$dir"/*.yaml; do
    if [ -f "$f" ]; then
      files+=("$f")
    fi
  done
  if [ "${#files[@]}" -eq 0 ]; then
    echo "no workflow files in ${dir}" >&2
    return 1
  fi
  grep -h -E 'uses:[[:space:]]*[^[:space:]]+' "${files[@]}" \
    | sed -E 's/\r$//; s/^.*uses:[[:space:]]*([^[:space:]]+)([[:space:]]+#[[:space:]]*([^[:space:]]+))?.*$/\1|\3/' \
    | LC_ALL=C sort -u
}

line_count() {
  wc -l <"$1" | tr -d ' '
}

expect_counts_match() {
  local name="$1" dir="$2" extracted_file counted_file extracted_n counted_n
  extracted_file="$(mktemp)"
  counted_file="$(mktemp)"
  if ! extract_action_refs "$dir" >"$extracted_file"; then
    echo "FAIL ${name} extract exited non-zero"
    fail=1
  elif ! count_unique_uses "$dir" >"$counted_file"; then
    echo "FAIL ${name} uses count exited non-zero"
    fail=1
  else
    extracted_n="$(line_count "$extracted_file")"
    counted_n="$(line_count "$counted_file")"
    if [ "$extracted_n" -lt 1 ]; then
      echo "FAIL ${name} extracted ${extracted_n} refs"
      fail=1
    fi
    if [ "$extracted_n" != "$counted_n" ]; then
      echo "FAIL ${name} extract=${extracted_n} uses=${counted_n}"
      diff -u "$counted_file" "$extracted_file" || true
      fail=1
    fi
  fi
  rm -f "$extracted_file" "$counted_file"
}

align_dir="$(mktemp -d)"
cat >"$align_dir/ci.yml" <<EOF
uses: actions/checkout@${sha40} # v6
uses: actions/checkout@${sha40} # v6
uses: ./.github/actions/local
uses: docker://alpine:3.20
EOF
align_raw="$(grep -h -E 'uses:[[:space:]]*[^[:space:]]+' "$align_dir"/*.yml | wc -l | tr -d ' ')"
align_unique="$(count_unique_uses "$align_dir" | wc -l | tr -d ' ')"
if [ "$align_raw" -le "$align_unique" ]; then
  echo "FAIL fixture uses count did not drop duplicates raw=${align_raw} unique=${align_unique}"
  fail=1
fi
expect_counts_match "fixture with duplicates and local actions" "$align_dir"
rm -rf "$align_dir"

expect_counts_match "live workflows" "$root/.github/workflows"

if command -v rg >/dev/null 2>&1; then
  rg_got="$(mktemp)"
  awk_got="$(mktemp)"
  (
    cd "$root"
    rg -N -o --no-filename --pcre2 'uses:\s+(\S+)(?:\s+#\s*(\S+))?' .github/workflows/*.yml -r '$1|$2' \
      | LC_ALL=C sort -u
  ) >"$rg_got"
  extract_action_refs "$root/.github/workflows" >"$awk_got"
  if ! cmp -s "$rg_got" "$awk_got"; then
    echo "FAIL awk extract differs from rg"
    diff -u "$rg_got" "$awk_got" || true
    fail=1
  fi
  rm -f "$rg_got" "$awk_got"
fi

script_out=""
script_err=""

run_script() {
  local dir="$1" path_prefix="${2:-}"
  script_out="$(mktemp)"
  script_err="$(mktemp)"
  script_status=0
  if [ -n "$path_prefix" ]; then
    (
      cd "$dir"
      PATH="${path_prefix}:$PATH" "$root/scripts/check-action-refs.sh"
    ) >"$script_out" 2>"$script_err" || script_status=$?
  else
    (
      cd "$dir"
      "$root/scripts/check-action-refs.sh"
    ) >"$script_out" 2>"$script_err" || script_status=$?
  fi
}

assert_script_fails() {
  local name="$1" needle="$2"
  if [ "$script_status" -eq 0 ]; then
    echo "FAIL ${name} exited 0"
    fail=1
  fi
  if ! grep -q "$needle" "$script_out" "$script_err"; then
    echo "FAIL ${name} missing ${needle}"
    echo "stdout:"
    cat "$script_out"
    echo "stderr:"
    cat "$script_err"
    fail=1
  fi
  if grep -q "All external action references are resolvable" "$script_out" "$script_err"; then
    echo "FAIL ${name} printed the success message"
    fail=1
  fi
  rm -f "$script_out" "$script_err"
}

zero_dir="$(mktemp -d)"
mkdir -p "$zero_dir/.github/workflows"
printf 'name: empty\n' >"$zero_dir/.github/workflows/blank.yml"
run_script "$zero_dir"
assert_script_fails "zero refs" "no action references found"
rm -rf "$zero_dir"

missing_dir="$(mktemp -d)"
run_script "$missing_dir"
assert_script_fails "missing workflow dir" "failed to extract action references"
rm -rf "$missing_dir"

awk_bin="$(mktemp -d)"
cat >"$awk_bin/awk" <<'EOF'
#!/bin/sh
echo "awk-forced-failure" >&2
exit 1
EOF
chmod +x "$awk_bin/awk"
awk_dir="$(mktemp -d)"
mkdir -p "$awk_dir/.github/workflows"
printf 'uses: actions/checkout@%s # v6\n' "$sha40" >"$awk_dir/.github/workflows/ci.yml"
run_script "$awk_dir" "$awk_bin"
assert_script_fails "extract command failure" "failed to extract action references"
rm -rf "$awk_bin" "$awk_dir"

bad_sha_dir="$(mktemp -d)"
mkdir -p "$bad_sha_dir/.github/workflows"
printf 'uses: actions/checkout@v6 # v6\n' >"$bad_sha_dir/.github/workflows/ci.yml"
run_script "$bad_sha_dir"
assert_script_fails "non-sha ref" "not a full commit SHA"
rm -rf "$bad_sha_dir"

missing_comment_dir="$(mktemp -d)"
mkdir -p "$missing_comment_dir/.github/workflows"
printf 'uses: actions/checkout@%s\n' "$sha40" >"$missing_comment_dir/.github/workflows/ci.yml"
run_script "$missing_comment_dir"
assert_script_fails "missing version comment" "missing version comment"
rm -rf "$missing_comment_dir"

ok_dir="$(mktemp -d)"
mkdir -p "$ok_dir/.github/workflows"
printf '%s\n' "uses: ./.github/actions/local" "uses: docker://alpine:3.20" >"$ok_dir/.github/workflows/ci.yml"
run_script "$ok_dir"
if [ "$script_status" -ne 0 ]; then
  echo "FAIL local and docker refs exited ${script_status}"
  cat "$script_out"
  cat "$script_err"
  fail=1
elif ! grep -q "All external action references are resolvable" "$script_out"; then
  echo "FAIL local and docker refs missing success message"
  cat "$script_out"
  fail=1
fi
rm -f "$script_out" "$script_err"
rm -rf "$ok_dir"

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "check-action-refs unit checks: ok"
