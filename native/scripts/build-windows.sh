#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
python3 scripts/prepare-codex.py win32-x64
go run github.com/egoist/mygo/cmd/mygo build -platform windows/amd64 -o build
