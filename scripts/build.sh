#!/bin/bash
# 一键构建：前端 → 后端单文件二进制
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

echo "==> 构建前端 (web/ → web/dist)"
if [ -d web ]; then
  cd web
  [ -d node_modules ] || npm install --registry=https://registry.npmmirror.com
  npm run build
  cd ..
else
  echo "web/ 不存在，跳过前端"
fi

echo "==> 构建后端"
export PATH=$PATH:/usr/local/go/bin
go build -trimpath -ldflags "-s -w" -o mangasync .
echo "==> 完成: $(pwd)/mangasync"
ls -lh mangasync | awk '{print $5, $9}'
