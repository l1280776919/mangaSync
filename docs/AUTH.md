# mangaSync 认证契约（v1，重构中）

旧的 `authUser/authPass` + HTTP Basic / `?token=` cookie 方案**已废弃**，改为：登录页 + 服务端会话 + 首次登录强制改密。

## 账号与初始化
- 用户存 SQLite `users` 表；密码用 PBKDF2-HMAC-SHA256（200k 迭代 + 随机盐）存储，绝不存明文。
- **首次初始化（users 表为空）自动创建**：`admin` / `admin999`，且 `mustChangePassword = true`。
- 只要 `mustChangePassword` 为 true，除 `/api/auth/*` 与 `/api/health` 外，**所有接口返回 403**
  `{"error":"请先修改初始密码","mustChangePassword":true}`，前端必须跳到改密页。

## 会话
- 登录成功 → `Set-Cookie: ms_session=<64位hex>; Path=/; HttpOnly; SameSite=Lax; Max-Age=30天`
- 会话存 SQLite `sessions` 表（token、user_id、created_at、expires_at、ua），**重启不掉线**。
- 登出 / 改密成功 → 删除会话（改密时保留当前会话，踢掉该用户其它会话）。
- 未登录访问受保护接口 → `401 {"error":"未登录"}`。

## 接口

### POST /api/auth/login
请求 `{"username":"admin","password":"admin999"}`
- 200 → `{"username":"admin","mustChangePassword":true,"isAdmin":true,"lastLoginAt":"..."}` + Set-Cookie
- 401 → `{"error":"账号或密码错误"}`（账号不存在与密码错误**都**返回这句，别泄露哪个错）
- 429 → `{"error":"登录尝试过于频繁，请稍后再试"}` + `Retry-After: <秒>`
  （同一 IP+账号连续失败 5 次后锁定 60s，成功登录清零）

### POST /api/auth/logout
- 200 → `{"ok":true}`，并清除 cookie（未登录也返回 200）

### GET /api/auth/me
- 200 → `{"username":"admin","mustChangePassword":false,"isAdmin":true,"lastLoginAt":"...","sessionExpiresAt":"..."}`
- 401 → `{"error":"未登录"}`

### POST /api/auth/password
请求 `{"oldPassword":"...","newPassword":"..."}`
- 200 → `{"ok":true,"mustChangePassword":false}`（新密码生效；当前会话保留）
- 400 → `{"error":"新密码至少 6 位"}` / `{"error":"新密码不能与原密码相同"}`
- 401 → `{"error":"原密码不正确"}`
- 未登录 → 401

### GET /api/health
始终免认证（探活用），返回 `{"status":"ok","uptimeSec":n,"version":"..."}`

## 其它受保护接口
全部沿用原有契约（见 `docs/API.md`），只是**都要带会话 cookie**；401 时前端跳登录页。

## 前端要求（本次重构）
1. 新增路由 `/login`（登录页，浅色，居中卡片：账号、密码、登录按钮、错误提示；
   底部小字提示「初始账号 admin / admin999，登录后请立即修改密码」）。
2. 新增路由 `/change-password`（改密页：原密码、新密码、确认新密码，≥6 位校验，
   成功后提示并跳 `#/dashboard`；`mustChangePassword=true` 时**不允许**跳过，只能改密或退出登录）。
3. 全局路由守卫：进入任意页面前 `GET /api/auth/me`；401 → 跳 `/login`；
   `mustChangePassword` → 强制跳 `/change-password`。
4. 侧边栏底部加「退出登录」；`api.js` 里所有请求遇 401 → 清状态跳 `/login`（`/api/auth/me` 自身除外，避免死循环）。
5. 设置页删掉旧的「访问控制」（authUser/authPass 字段已不存在），改成「账号安全」区块：
   显示当前账号、最近登录时间、内嵌改密表单（同 `/change-password` 的逻辑）、退出登录按钮。
