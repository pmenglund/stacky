#!/bin/bash

set -euo pipefail

PROMPT='continue with next task in .agent/PLAN.md'
DONE='done'
I=1

export GOCACHE="$(pwd)/.gocache"

while true; do
  say "iteration ${I}"
  echo ""
  echo -e "\033[1;31m==========================\033[0m"
  echo -e "\033[1;31m===== iteration ${I} =====\033[0m"
  echo -e "\033[1;31m==========================\033[0m"
  echo ""
  # increment I
  ((I++))
  codex exec --include-plan-tool --full-auto "${PROMPT}" resume
  git status
  git add cmd internal

  if [ -f "${DONE}" ]; then
    echo "done"
    rm done
    exit 0
  fi
done
