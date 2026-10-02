#!/bin/sh
# Copyright 2026 Brightfellow contributors
# SPDX-License-Identifier: Apache-2.0
#
# Fails if a source file lacks the SPDX header (01 §2).
set -eu
missing=$(git ls-files --cached --others --exclude-standard '*.go' '*.ts' '*.tsx' '*.sql' |
  grep -v -e '^packages/api-client/src/schema.d.ts$' |
  while read -r f; do
    head -n 3 "$f" | grep -q 'SPDX-License-Identifier: Apache-2.0' || echo "$f"
  done)
if [ -n "$missing" ]; then
  echo "Missing license header:"
  echo "$missing"
  exit 1
fi
