#!/bin/sh
# Print the CHANGELOG.md section of a release: scripts/release-notes.sh v0.1.0
# The section starts at "## 0.1.0" (or "## v0.1.0") and ends at the next "## ".
# Fails when there is none, so a release cannot go out without notes.
set -eu
tag="${1:?usage: release-notes.sh vX.Y.Z}"
ver="${tag#v}"
notes=$(awk -v a="## $ver" -v b="## v$ver" '
	/^## / { on = ($0 == a || index($0, a " ") == 1 || $0 == b || index($0, b " ") == 1); if (on) next }
	on { print }' "$(dirname "$0")/../CHANGELOG.md")
if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
	echo "CHANGELOG.md has no section '## $ver'" >&2
	exit 1
fi
printf '%s\n' "$notes"
