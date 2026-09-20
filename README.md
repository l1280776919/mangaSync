# mangaSync

漫画收藏同步系统：把**哔咔漫画(PicaComic)**和**禁漫(18comic/JMComic)**账号里的「我的收藏」增量同步下载到本地 NAS，
并提供账号管理、收藏管理、官方搜索、下载队列、漫画库浏览和定时同步。

- 后端：Go 1.27（单文件二进制，内置 SQLite 存账号/任务/库，内嵌前端静态资源）
- 前端：Vue 3 + Vite + Element Plus（浅色主题，中后台风格）
- 部署形态：**前后端不分离** —— 前端 `npm run build` 的产物被 `go:embed` 进后端，
  运行时只需要一个 `mangasync` 二进制 + 一个端口

## 功能

| 模块 | 能力 |
| --- | --- |
| 账号管理 | 新增/编辑/删除账号（哔咔、禁漫），测试登录、显示昵称/等级/收藏数、登录态失效自动重登 |
| 收藏管理 | 分页浏览收藏（封面/标题/作者/分类/章节数/已下载状态），关键词过滤，加入收藏、取消收藏，一键「整本下载」，一键「同步该账号收藏」 |
| 官方搜索 | 哔咔 `comics/advanced-search`、禁漫 `/search`，支持排序、分页，结果可直接下载或加收藏 |
| 下载队列 | 并发调度（可配 1–8）、章节级/图片级进度、速度、当前章节、失败重试、取消、日志查看（SSE 实时推送） |
| 漫画库 | 扫描两个下载目录生成索引（标题/章节/图片数/体积/路径），按源筛选、排序、删除（可选删文件）、重新扫描 |
| 设置 | 下载根目录与各源子目录、代理、并发、图片质量（原图/高清/普通）、每日定时同步时间 |
| 统计 | 账号/收藏/任务状态/库体积/磁盘剩余 |

## 架构

```
main.go                    启动、内嵌前端、每日定时同步
internal/config            设置（$MANGASYNC_HOME/config.json，默认 /var/lib/mangasync）
internal/store             SQLite（modernc.org/sqlite，纯 Go 无 CGO）
internal/source            漫画源抽象
   ├─ pica.go              哔咔：原生 Go 实现（签名、收藏、搜索、详情、章节图片、下载）
   └─ jm.go                禁漫：**原生 Go 实现**（md5 token 签名 + AES-ECB 接口解密 + 图片乱序还原）
   └─ jm_crypto.go/jm_image.go  禁漫加解密与图片解码（纯标准库 + nativewebp/x-image，无 Python）
internal/engine            任务队列、并发调度、进度事件（SSE）、收藏同步、目录扫描
internal/api               REST API + 内嵌前端（web/dist）
web/                       Vue 3 前端源码（构建产物进 web/dist）
docs/API.md                接口契约
```

两个源的关键差异：

| | 哔咔 PicaComic | 禁漫 18comic |
| --- | --- | --- |
| 接口 | `https://picaapi.picacomic.com/`，HMAC-SHA256 签名 | 移动端 API（域名自动更新） |
| 网络 | 国内多数网络下域名被污染，**通常需要代理** | **直连即可**（图片 CDN 也直连） |
| 登录 | 账号名（不是邮箱）+ 密码 | 账号 + 密码，登录态是 cookies |
| 图片 | `{fileServer}/static/{path}`，`image-quality: original` 拿原图 | `cdn-msp*.jmapiproxy*.cc/media/photos/...`，官方 App 同款 webp 原图 |
| 收藏增删 | `POST comics/{id}/favourite`（toggle，英式拼写） | `/favorite` toggle |
| 目录结构 | `<标题>/<001 - 章节名>/001.jpg` | `<id> <标题>/[<序号> <章节名>/]00001.webp` |

## 构建与运行

```bash
# 1. 前端（需要 node 18+）
cd web && npm install && npm run build && cd ..

# 2. 后端（Go 1.22+，会内嵌 web/dist）
go build -o mangasync .

# 3. 运行
./mangasync                 # 默认 :8787，数据目录 $MANGASYNC_HOME（默认 /var/lib/mangasync）
./mangasync -data /srv/mangasync -addr :9000   # 也可以用参数指定
./mangasync -addr :9000     # 自定义端口
```

一键：`scripts/build.sh`（构建前后端并输出 ./mangasync）。

systemd（可选）：

```bash
cp deploy/mangasync.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now mangasync
```

打开 `http://<NAS>:8787`，先在「账号」页添加哔咔/禁漫账号（登录成功才会保存），
然后「漫画库 → 重新扫描」把已有漫画索引进来，之后在「收藏」页点「同步账号收藏」即可增量下载。

## 登录与访问控制

管理后台需要登录，账号密码存在本地 SQLite（PBKDF2-HMAC-SHA256 加盐，200k 迭代），会话存库、重启不掉线。

- **首次初始化**：数据库里没有任何账号时，自动创建 `admin` / `admin999`，并强制首次登录修改密码
  （未改密前除改密/登出外所有接口返回 403）。
- 登录页 `#/login`；改密页 `#/change-password`，也可在「设置 → 账号安全」里改。
- 会话 cookie：`ms_session`，HttpOnly + SameSite=Lax，有效期 30 天；改密后自动踢掉该账号的其它会话。
- 登录限流：同一账号连续失败 5 次锁 60 秒（穿透场景下客户端 IP 都是 127.0.0.1，故按账号计数）。
- `/api/health` 始终免认证，便于外部探活/监控。

公网访问：本机当前走 **Cloudflare Tunnel**（`cloudflared` 服务，tunnel 名 `fnos`，入口 https://manga.lamtap.com ），
只有出站连接、NAS 不开放入站端口（frp 方案已退役）。换成反代/端口映射同理，打开就是登录页：

```bash
# 命令行验证
curl -c ck.txt -X POST http://<地址>/api/auth/login \
  -H 'Content-Type: application/json' -d '{"username":"admin","password":"你的密码"}'
curl -b ck.txt http://<地址>/api/stats
```

> 提示：穿透/反代只走 HTTP 时，登录密码是明文传输的；公网长期使用建议在前面套一层 HTTPS
> （本机用 Cloudflare Tunnel，客户端到 CF 边缘这一段已经是 HTTPS）。
> 服务对文本响应默认开启 gzip（前端 element-plus 打包 1MB → 约 340KB），穿透/公网访问更快。

## 与独立下载脚本的兼容性

如果之前用独立脚本下载过漫画，把 `downloadRoot` 指到同一个目录即可：本系统沿用相同的目录命名约定
（哔咔 `标题/001 - 章节名/001.jpg`、禁漫 `<id> 标题/[序号 章节名/]00001.webp`），
点一次「漫画库 → 重新扫描」就能把历史下载识别进来，不会重复下载。
下载逻辑同样保持该约定：章节先写隐藏临时目录 `.xxx.part`，图片张数校验通过才改名；已完整章节自动跳过。

## 数据与备份

- 数据目录：`/vol1/@appdata/mangasync/`（SQLite 库 + `config.json`，含站点账号）；漫画库在 `/vol1/1000/漫画/`。
- **每日备份**（`/etc/cron.daily/mangasync-backup`）：SQLite 在线备份 + 配置 + 漫画库镜像 → 独立物理盘
  `/vol2/backup/mangasync/`（DB 保留最近 30 份）。
  ⚠️ 数据库是 WAL 模式，**直接 `cp mangasync.db` 会丢掉最近写入**；备份脚本用 Python 内置 `sqlite3`
  的在线 backup API（等价 `sqlite3 .backup`）。恢复时停服务、把备份文件拷回数据目录即可。
- **每日巡检**（Hermes cron「mangaSync 巡检」）：检查服务健康 / 同步任务新鲜度 / 磁盘空间 / 备份新鲜度，
  仅在异常时推送通知，正常时静默。
- 服务由 systemd 管理（`Restart=always` + `StartLimitIntervalSec=0`，崩溃自动拉起），并设了内存/CPU 上限
  （`MemoryMax=2G`、`CPUQuota=200%`），避免影响 NAS 上的其它应用。

## 说明

- 后端默认监听所有网卡的 8787 端口；**除 `/api/health` 外的所有接口都需要登录会话**（见上文「登录与访问控制」），
  公网使用时请保持登录开启，并优先走 HTTPS 入口（本机为 Cloudflare Tunnel）。
- 禁漫为纯 Go 实现，不需要 Python / jmcomic。
- **图片画质**：站点下发的原图若带乱序（scramble），会先解码→按行带还原→**无损 WebP 编码**落盘
  （不引入二次压缩损失；不做乱序的图直接原字节保存）。代价是体积约为站点有损版的 2~3 倍。
- 图片来源与账号凭据仅保存在本机（config.json 权限 600、SQLite 本地文件）。
