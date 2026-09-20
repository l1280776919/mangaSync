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
go vet ./... || { echo "go vet 未通过，终止构建"; exit 1; }
# 备份旧二进制便于回滚（*.bak-* 已在 .gitignore）
[ -f mangasync ] && cp mangasync "mangasync.bak-$(date +%m%d%H%M)"
go build -trimpath -ldflags "-s -w" -o mangasync .

echo "==> 重启服务并健康检查"
if systemctl restart mangasync 2>/dev/null; then
  sleep 2
  if curl -fsS -m 5 http://127.0.0.1:8787/api/health >/dev/null; then
    echo "==> ✅ 部署成功，健康检查通过"
  else
    echo "==> ⚠️ 健康检查失败，请查 journalctl -u mangasync（备份二进制在本目录 mangasync.bak-*）"
    exit 1
  fi
else
  echo "==> 未重启服务（无权限 / 服务未安装），仅完成构建"
fi

echo "==> 完成: $(pwd)/mangasync"
ls -lh mangasync | awk '{print $5, $9}'
