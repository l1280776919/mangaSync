#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
npm ci --prefix web --no-audit --no-fund
npm test --prefix web
npm run build --prefix web
go vet ./...
go test ./...
commit=$(git rev-parse HEAD)
version=$(git describe --tags --always --dirty)
mkdir -p dist
go build -trimpath -ldflags "-s -w -X github.com/l1280776919/mangaSync/internal/api.Version=$version -X github.com/l1280776919/mangaSync/internal/api.Commit=$commit" -o dist/mangasync .
echo "Built dist/mangasync ($version). Deploy explicitly with scripts/deploy.sh."
