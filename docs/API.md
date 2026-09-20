# mangaSync API 契约 (v1)

后端 Go（监听 `:8787`，可用 settings 改），前端 Vue 3 SPA 由后端 embed 同源托管，因此前端一律用相对路径 `/api/...`。

约定：
- 成功：HTTP 2xx + JSON body（就是数据本身，不包一层 ok）
- 失败：HTTP 4xx/5xx + `{"error":"人话错误信息"}`
- 时间：RFC3339 字符串（如 `2026-09-20T14:30:00+08:00`）
- 分页参数：`page` 从 1 开始，`pageSize` 默认 20（最大 200）
- 源标识 `kind`：`pica`（哔咔 PicaComic）| `jm`（禁漫 18comic）

## 设置

`GET /api/settings`

```json
{
  "downloadRoot": "/data/comics",
  "picaDir": "PicaComic",
  "jmDir": "18Comic",
  "picaProxy": "http://127.0.0.1:7890",
  "jmProxy": "",
  "concurrency": 2,
  "imageWorkers": 8,
  "quality": "original",
  "schedule": { "enabled": true, "time": "04:30" },
  "serverPort": 8787
}
```

`PUT /api/settings`：body 为上面对象的部分字段，返回合并后的完整设置。
- 所有文本响应默认 gzip（`Vary: Accept-Encoding`），`/api/events` 除外。
- **认证**：本文件列出的所有接口都需要登录会话（Cookie `ms_session`），否则 401。
  认证接口见 `docs/AUTH.md`：`POST /api/auth/login`、`POST /api/auth/logout`、
  `GET /api/auth/me`、`POST /api/auth/password`；仅 `/api/health` 免认证。
  首次初始化账号 `admin` / `admin999`，未改密前除认证接口外一律 403。
（校验：`downloadRoot` 必须存在或是可创建目录；`concurrency` 1..8；`quality` ∈ original|medium|low（只有 pica 用得上））

## 账号

`GET /api/accounts`

```json
[{
  "id": 1, "kind": "pica", "username": "your_account", "label": "哔咔主号",
  "nickname": "njdcjjbfuhv", "level": 1,
  "favoritesCount": 43, "favoritesMax": null,
  "status": "ok",             // ok | expired | error
  "note": "", "lastLoginAt": "...", "lastSyncAt": "...", "createdAt": "..."
}]
```

- `POST /api/accounts` body `{kind, username, password, label}` → 建号并立即登录，返回账号对象；密码错误返回 400 `{"error":"账号或密码错误"}`
- `PATCH /api/accounts/{id}` body `{label?, username?, password?, note?}`（改了 username/password 会自动重登）
- `DELETE /api/accounts/{id}`
- `POST /api/accounts/{id}/login` → 返回账号对象（刷新 status/nickname）
- `POST /api/accounts/{id}/sync` → 把该账号收藏全部入队，返回 `{"enqueued": 12, "skipped": 31}`
  （已下载完整且无新章节的跳过）

## 收藏

`GET /api/accounts/{id}/favorites?keyword=&page=1&pageSize=50`

```json
{
  "total": 43, "page": 1, "pageSize": 50,
  "items": [{
    "kind": "pica", "comicId": "5ea3a16eb724bb488487b8e1",
    "title": "本当にいた!!時間停止おじさん", "author": "スパイシーラブスヘブン",
    "categories": ["短篇"], "tags": ["C97"], "latestEp": "",
    "chapters": 2, "cover": "/api/comics/pica/5ea3a16eb724bb488487b8e1/cover",
    "downloaded": true, "downloadedChapters": 2, "images": 53, "bytes": 34500000,
    "localPath": "/data/comics/PicaComic/本当にいた!!時間停止おじさん"
  }]
}
```

- `POST /api/accounts/{id}/favorites` body `{"comicId":"1473052"}` → 加入收藏
- `DELETE /api/accounts/{id}/favorites/{comicId}` → 取消收藏
- 说明：哔咔收藏分页 20/页（后端负责翻页聚合）；禁漫一次性返回全部

## 漫画详情 / 封面

- `GET /api/comics/{kind}/{comicId}` →

```json
{ "kind":"jm","comicId":"1473052","title":"...","author":"...","categories":["同人"],
  "tags":[],"description":"","chapters":[
    {"order":1,"id":"1473052","title":"第1话","images":36,"downloaded":true}
  ],
  "downloadedChapters":1, "localPath":"...", "cover":"/api/comics/jm/1473052/cover" }
```

- `GET /api/comics/{kind}/{comicId}/cover` → 直接返回图片二进制（后端代理下载并缓存到 `<数据目录>/cache/covers/`，避免前端跨域/被风控）

## 搜索（官方接口）

`GET /api/search?kind=pica&keyword=&page=1&pageSize=20&accountId=1&sort=`

```json
{ "total": 80, "page": 1, "pageSize": 20, "items": [ { ...同收藏 item 结构... } ] }
```

- pica：`POST comics/advanced-search`（支持 `sort`：`dd`新到旧 / `da` / `ld`点赞 / `vd`观看）
- jm：`/search`（`sort` = `mr`最新 / `mv`最多观看 / `mp`最多图片 / `tf`今日最多点赞）
- `accountId` 可选：传了就用该账号的登录态（禁漫搜索需要登录态更稳）

## 下载任务

`POST /api/downloads`

```json
{ "kind":"jm", "accountId":1, "comicId":"1473052", "title":"可选覆盖",
  "chapters":[1,2], "all":false }
```
→ `{"id": 7}`（`all:true` 或省略 chapters 表示整本）

`GET /api/downloads?status=&page=&pageSize=`

```json
{ "items": [{
  "id":7, "kind":"jm", "comicId":"1473052", "title":"...", "accountId":1,
  "status":"running",            // queued|running|done|failed|canceled
  "chaptersTotal":1, "chaptersDone":0, "imagesTotal":36, "imagesDone":12,
  "bytes":5242880, "speedBps":1048576, "currentChapter":"第1话",
  "localPath":"/data/comics/18Comic/1473052 ...", "error":"",
  "createdAt":"...","startedAt":"...","finishedAt":null
}] }
```

- `POST /api/downloads/{id}/cancel`、`POST /api/downloads/{id}/retry`、`DELETE /api/downloads/{id}`
- `GET /api/downloads/{id}/logs` → `[{"ts":"...","level":"info","msg":"..."}]`
- 入库策略与现有脚本一致：章节先下到隐藏临时目录，张数校验通过才改名；已完整章节跳过

## 漫画库

`GET /api/library?kind=&keyword=&page=&pageSize=&sort=time|size` →

```json
{ "total": 55, "stats": {"comics":55,"chapters":131,"images":7350,"bytes":3200000000},
  "items": [{ "id":3,"kind":"pica","comicId":"5ea3...","title":"...",
              "path":"/data/comics/PicaComic/...","chapters":2,"images":53,"bytes":34500000,
              "updatedAt":"...","source":"scan|db" }] }
```

- `DELETE /api/library/{id}?files=true|false`（files=true 同时删磁盘目录）
- `POST /api/library/scan` → 重新扫描下载目录，返回 `{"found": 55, "added": 2}`

## 统计 / 事件

- `GET /api/stats` → `{"accounts":2,"favorites":55,"downloads":{"queued":0,"running":1,"done":12,"failed":1},"library":{...},"disk":{"freeBytes":0,"totalBytes":0}}`
- `GET /api/events` → SSE，`event: job`，`data: <任务对象>`，每秒推送处于 queued/running 的任务与最近变化；前端据此刷新进度条

## 健康检查

`GET /api/health` → `{"status":"ok","version":"0.1.0","uptimeSec":123}`
