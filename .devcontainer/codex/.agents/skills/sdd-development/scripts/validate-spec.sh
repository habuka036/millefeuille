#!/usr/bin/env bash

set -euo pipefail

usage() {
  echo "Usage: $(basename "$0") <spec-directory>" >&2
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

spec_dir=$1

[[ -d "$spec_dir" ]] || fail "spec directory does not exist: $spec_dir"

requirements_file=""
if [[ -f "$spec_dir/requirements.md" ]]; then
  requirements_file="$spec_dir/requirements.md"
elif [[ -f "$spec_dir/bugfix.md" ]]; then
  requirements_file="$spec_dir/bugfix.md"
else
  fail "requirements.md or bugfix.md is required"
fi

for required_file in design.md tasks.md; do
  [[ -f "$spec_dir/$required_file" ]] || fail "missing $spec_dir/$required_file"
done

design_file="$spec_dir/design.md"
tasks_file="$spec_dir/tasks.md"
requirement_pattern='(BUG-REQ|REQ|NFR|SEC|COMP)-[0-9]{3}'

mapfile -t requirement_ids < <(
  grep -Eo "$requirement_pattern" "$requirements_file" | sort -u
)

if [[ ${#requirement_ids[@]} -eq 0 ]]; then
  fail "no requirement IDs found in $requirements_file"
fi

for requirement_id in "${requirement_ids[@]}"; do
  grep -Fq "$requirement_id" "$design_file" || \
    fail "$requirement_id is not referenced by design.md"
  grep -Fq "$requirement_id" "$tasks_file" || \
    fail "$requirement_id is not referenced by tasks.md"
done

if grep -Eq '^- \[( |~|!)\] TASK-[0-9]{3}' "$tasks_file"; then
  fail "tasks.md contains incomplete or blocked tasks"
fi

if grep -Eq '\{\{[^}]+\}\}' \
  "$requirements_file" "$design_file" "$tasks_file"; then
  fail "spec files contain unresolved template placeholders"
fi

for spec_file in "$requirements_file" "$design_file"; do
  if awk '
    /^## Open questions/ { in_section = 1; next }
    in_section && /^## / { in_section = 0 }
    in_section && /^- / && $0 != "- None" { found = 1 }
    END { exit(found ? 0 : 1) }
  ' "$spec_file"; then
    fail "unresolved open questions found in $spec_file"
  fi
done

grep -Fq '| Requirement | Design | Task | Test |' "$tasks_file" || \
  fail "tasks.md does not contain the traceability table"

echo "Spec validation passed: $spec_dir"
