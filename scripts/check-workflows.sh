#!/usr/bin/env bash
set -euo pipefail

echo "[workflow-check] YAML syntax"
ruby -e 'require "yaml"; Dir[".github/workflows/*.yml"].sort.each { |f| YAML.load_file(f); puts "ok: #{f}" }'

if command -v actionlint >/dev/null 2>&1; then
  echo "[workflow-check] actionlint"
  actionlint .github/workflows/*.yml
else
  echo "[workflow-check] skip actionlint (not installed)"
  echo "install: brew install actionlint (macOS) or https://github.com/rhysd/actionlint"
fi
