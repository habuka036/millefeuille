#!/usr/bin/env bash
set -euo pipefail
set +x

echo "Start Post Create..."

# Reconcile the installed tool set when mise.toml changes without an image rebuild.
mise trust /home/developer/.config/mise/config.toml
mise install
mise reshim

echo "Installed mise tool versions..."
node --version
python --version
go version
pandoc --version | head -n 1
java --version

if [ -f go.mod ]; then
  echo "Invoke go mod..."
  go mod download
  echo "Invoke go install gopls..."
  go install golang.org/x/tools/gopls@latest
  echo "Invoke go install dlv..."
  go install github.com/go-delve/delve/cmd/dlv@latest
fi

echo "Settings for kiro-cli..."
kiro-cli settings telemetry.enabled false
kiro-cli settings codeWhisperer.shareCodeWhispererContentWithAWS false

echo "Installed CLI versions..."
kiro-cli --version
codex --version
claude --version

if [ ! -d .kiro ]; then
  echo ""
  echo "cc-sdd (Kiro-compatible SDD for Claude Code) is not yet initialized in this project."
  echo "Run the following once, interactively, to scaffold it (it will prompt before writing files"
  echo "and may overwrite CLAUDE.md, so it is intentionally NOT run automatically here):"
  echo ""
  echo "  npx cc-sdd@latest --lang ja"
  echo ""
fi

echo "Post Create is done."
