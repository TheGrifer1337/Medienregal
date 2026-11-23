#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
version=$(<VERSION)
[[ "$version" =~ ^(0|[1-9][0-9]*)\.[0-9]$ ]]
repo=${1:-TheGrifer1337/Medienregal}
[[ "$repo" =~ ^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$ ]]
mkdir -p dist
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "${GO_BIN:-go}" build -buildvcs=false -trimpath   -ldflags "-s -w -H=windowsgui -X main.version=$version -X main.releaseRepo=$repo" -o dist/Medienregal.exe .
"${MAKENSIS:-makensis}" -V3 "-DVERSION=$version" "-DAPPEXE=dist/Medienregal.exe"   "-DOUTFILE=dist/Medienregal-Setup-v$version.exe" installer.nsi
