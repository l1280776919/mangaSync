/** 展示用的格式化与常量工具 */

export const KIND_OPTIONS = [
  { value: 'pica', label: '哔咔 PicaComic' },
  { value: 'jm', label: '禁漫 18comic' }
]

export const KIND_LABEL = { pica: '哔咔', jm: '禁漫' }

/** 账号状态 */
export const ACCOUNT_STATUS = {
  ok: { label: '正常', type: 'success' },
  expired: { label: '登录过期', type: 'warning' },
  error: { label: '异常', type: 'danger' }
}

/** 下载任务状态 */
export const JOB_STATUS = {
  queued: { label: '排队中', type: 'info' },
  running: { label: '下载中', type: 'primary' },
  done: { label: '已完成', type: 'success' },
  failed: { label: '失败', type: 'danger' },
  canceled: { label: '已取消', type: 'info' }
}

/** 各源可用的排序方式 */
export const SORT_OPTIONS = {
  pica: [
    { value: 'dd', label: '新到旧' },
    { value: 'da', label: '旧到新' },
    { value: 'ld', label: '最多点赞' },
    { value: 'vd', label: '最多观看' }
  ],
  jm: [
    { value: 'mr', label: '最新' },
    { value: 'mv', label: '最多观看' },
    { value: 'mp', label: '最多图片' },
    { value: 'tf', label: '今日最多点赞' }
  ]
}

export const QUALITY_OPTIONS = [
  { value: 'original', label: '原图 original' },
  { value: 'medium', label: '中等 medium' },
  { value: 'low', label: '低 low' }
]

/** 字节数 → 人类可读 */
export function formatBytes(bytes) {
  const n = Number(bytes)
  if (!n || n < 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

/** 速度 bps → 人类可读 */
export function formatSpeed(bps) {
  const n = Number(bps)
  if (!n || n <= 0) return '—'
  return `${formatBytes(n)}/s`
}

/** 秒 → 1天2小时3分4秒 */
export function formatDuration(sec) {
  const n = Math.floor(Number(sec))
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n < 60) return `${n} 秒`
  const d = Math.floor(n / 86400)
  const h = Math.floor((n % 86400) / 3600)
  const m = Math.floor((n % 3600) / 60)
  const s = n % 60
  const parts = []
  if (d) parts.push(`${d} 天`)
  if (h) parts.push(`${h} 小时`)
  if (m) parts.push(`${m} 分`)
  if (!d && !h && s) parts.push(`${s} 秒`)
  return parts.join('') || '0 秒'
}

/** 时间戳(秒) → 时长 */
export function durationBetween(startAt, endAt) {
  if (!startAt) return '—'
  const s = new Date(startAt).getTime()
  const e = endAt ? new Date(endAt).getTime() : Date.now()
  if (!Number.isFinite(s) || !Number.isFinite(e)) return '—'
  return formatDuration((e - s) / 1000)
}

/**
 * 任务耗时（列表列 & 进度行共用，避免两份手写实现）。
 * 口径：<1 分钟 → “N 秒”；<1 小时 → “N 分 M 秒”；否则 → “N 小时 M 分”。
 * endAt 缺省表示仍在进行中（算到当前时刻）。
 */
export function formatElapsed(startAt, endAt) {
  if (!startAt) return '—'
  const s = new Date(startAt).getTime()
  const e = endAt ? new Date(endAt).getTime() : Date.now()
  if (!Number.isFinite(s) || !Number.isFinite(e)) return '—'
  const sec = Math.max(0, Math.floor((e - s) / 1000))
  if (sec < 60) return `${sec} 秒`
  const m = Math.floor(sec / 60)
  if (m < 60) return `${m} 分 ${sec % 60} 秒`
  return `${Math.floor(m / 60)} 小时 ${m % 60} 分`
}

/** RFC3339 → 本地显示 */
export function formatTime(value, withSeconds = false) {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return String(value)
  const p = (x) => String(x).padStart(2, '0')
  const base = `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
  return withSeconds ? `${base}:${p(d.getSeconds())}` : base
}

/** 相对时间：3 分钟前 */
export function fromNow(value) {
  if (!value) return '—'
  const t = new Date(value).getTime()
  if (!Number.isFinite(t)) return String(value)
  const diff = Math.floor((Date.now() - t) / 1000)
  if (diff < 0) return formatTime(value)
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)} 天前`
  return formatTime(value)
}

/** 已下载状态：全部 / 部分 / 未下载 */
export function downloadStateOf(item) {
  const total = Number(item?.chapters) || 0
  const done = Number(item?.downloadedChapters) || 0
  if (item?.downloaded === true || (total > 0 && done >= total)) {
    return { key: 'all', label: '已下载', type: 'success' }
  }
  if (done > 0) return { key: 'part', label: `部分 ${done}/${total}`, type: 'warning' }
  return { key: 'none', label: '未下载', type: 'info' }
}

/** 任务进度百分比 */
export function jobProgress(job) {
  if (!job) return 0
  if (job.status === 'done') return 100
  const imgTotal = Number(job.imagesTotal) || 0
  const imgDone = Number(job.imagesDone) || 0
  if (imgTotal > 0) return Math.min(100, Math.round((imgDone / imgTotal) * 100))
  const chTotal = Number(job.chaptersTotal) || 0
  const chDone = Number(job.chaptersDone) || 0
  if (chTotal > 0) return Math.min(100, Math.round((chDone / chTotal) * 100))
  return 0
}

export function truncate(text, max = 80) {
  const s = String(text ?? '')
  return s.length > max ? `${s.slice(0, max)}…` : s
}
