#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
version=$(<VERSION)
[[ "$version" =~ ^(0|[1-9][0-9]*)\.[0-9]$ ]]
repo=${1:-TheGrifer1337/Medienregal}
[[ "$repo" =~ ^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$ ]]
mkdir -p dist
bootstrapper=dist/WebView2Bootstrapper.exe
if [[ ! -s "$bootstrapper" ]]; then
  curl --fail --silent --show-error --location --proto '=https' --max-time 90 \
    'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -o "$bootstrapper.part"
  mv "$bootstrapper.part" "$bootstrapper"
fi
"${GO_BIN:-go}" run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --manifest gui \
  --icon winres/bookshelf.ico --file-description 'Medatheke by Maximilian' --product-name 'Medatheke by Maximilian' \
  --original-filename Medienregal.exe --product-version "$version.0.0" --file-version "$version.0.0"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "${GO_BIN:-go}" build -buildvcs=false -trimpath   -ldflags "-s -w -H=windowsgui -X main.version=$version -X main.releaseRepo=$repo" -o dist/Medienregal.exe .
"${MAKENSIS:-makensis}" -V3 "-DVERSION=$version" "-DBOOTSTRAPPER=$bootstrapper" "-DAPPEXE=dist/Medienregal.exe"   "-DOUTFILE=dist/Medienregal-Setup-v$version.exe" installer.nsi
