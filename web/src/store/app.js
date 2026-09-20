import { defineStore } from 'pinia'
import api from '@/api'

/** /api/stats 的复用窗口：切页太频繁时不必每次都打一次（顶栏刷新会强制拉） */
const STATS_TTL = 15000

/* 进行中的请求：放模块作用域，不把 Promise 塞进 pinia 变成响应式数据 */
let accountsInflight = null
let statsInflight = null
let statsAt = 0

/**
 * 全局状态：账号列表、设置、统计、以及 SSE 实时任务流。
 *
 * SSE：GET /api/events （event: job, data: <任务对象>）
 * - 掉线时指数退避自动重连
 * - 同时把「实时模式」降级为轮询 /api/downloads?status=running
 */
export const useAppStore = defineStore('app', {
  state: () => ({
    accounts: [],
    accountsLoaded: false,
    accountsError: '',   // 账号列表最近一次加载失败的原因（空串表示正常）
    settings: null,
    stats: null,
    jobs: {},            // id -> 任务对象
    jobTick: 0,          // 每收到一次任务事件 +1，供视图 watch
    refreshTick: 0,      // 顶栏「刷新」+1，当前视图 watch 它重新加载自己的数据
    sseStatus: 'idle',   // idle | connecting | online | offline
    sseRetries: 0,
    polling: false,
    lastEventAt: null,
    _es: null,
    _retryTimer: null,
    _pollTimer: null
  }),

  getters: {
    sseConnected: (s) => s.sseStatus === 'online',
    jobList: (s) => Object.values(s.jobs),
    runningJobs: (s) => Object.values(s.jobs).filter((j) => j.status === 'queued' || j.status === 'running'),
    accountMap: (s) => {
      const m = {}
      for (const a of s.accounts) m[a.id] = a
      return m
    },
    picaAccounts: (s) => s.accounts.filter((a) => a.kind === 'pica'),
    jmAccounts: (s) => s.accounts.filter((a) => a.kind === 'jm')
  },

  actions: {
    /* ---------------- 基础数据 ---------------- */

    async loadAccounts(force = false) {
      if (this.accountsLoaded && !force) return this.accounts
      // 并发调用（App 启动 + 视图 onActivated）只发一次请求
      if (accountsInflight) return accountsInflight
      accountsInflight = api
        .listAccounts()
        .then((data) => {
          this.accounts = Array.isArray(data) ? data : (data?.items ?? [])
          this.accountsLoaded = true
          this.accountsError = ''
          return this.accounts
        })
        .catch((e) => {
          // 加载失败**不**置 accountsLoaded：否则一次抖动后列表永久为空，
          // 用户只看到「该源还没有账号」，不知道其实是没加载成功
          this.accountsLoaded = false
          this.accountsError = e?.message || '账号列表加载失败'
          throw e
        })
        .finally(() => {
          accountsInflight = null
        })
      return accountsInflight
    },

    /**
     * 统计信息。maxAge 内的结果直接复用（避免每次路由跳转都重拉一份 stats），
     * 传 0 表示强制拉取。
     */
    async loadStats(maxAge = STATS_TTL) {
      if (this.stats && Date.now() - statsAt < maxAge) return this.stats
      if (statsInflight) return statsInflight
      statsInflight = api
        .stats()
        .then((s) => {
          this.stats = s
          statsAt = Date.now()
          return s
        })
        .finally(() => {
          statsInflight = null
        })
      return statsInflight
    },

    /** 顶栏「刷新」：广播给当前视图，由各视图重新加载自己的数据 */
    triggerRefresh() {
      this.refreshTick++
    },

    /* ---------------- 任务 & SSE ---------------- */

    /** 用接口数据整体替换任务缓冲区（用于轮询降级/首屏） */
    mergeJobs(items) {
      const map = { ...this.jobs }
      for (const j of items || []) {
        if (j && j.id !== undefined) map[j.id] = { ...(map[j.id] || {}), ...j }
      }
      this.jobs = map
      this.jobTick++
    },

    /** 加载进行中的任务（首屏/降级轮询用） */
    async loadActiveJobs() {
      // 两次请求并发（原来串行，降级轮询时白等一个 RTT）
      const [running, queued] = await Promise.all([
        api.listDownloads({ status: 'running', pageSize: 200 }),
        api.listDownloads({ status: 'queued', pageSize: 200 })
      ])
      this.mergeJobs(running?.items || [])
      this.mergeJobs(queued?.items || [])
    },

    startEvents() {
      if (this._es || typeof EventSource === 'undefined') {
        if (typeof EventSource === 'undefined' && this.sseStatus !== 'offline') {
          this.sseStatus = 'offline'
          this.startPolling()
        }
        return
      }
      this.sseStatus = 'connecting'
      let es
      try {
        es = new EventSource('/api/events')
      } catch (e) {
        this._onSseDown()
        return
      }
      this._es = es

      const onJob = (ev) => {
        try {
          const job = JSON.parse(ev.data)
          if (job && job.id !== undefined) {
            this.jobs = { ...this.jobs, [job.id]: { ...(this.jobs[job.id] || {}), ...job } }
            this.jobTick++
          }
        } catch (_) {
          /* 忽略非法消息 */
        }
        this.lastEventAt = Date.now()
        if (this.sseStatus !== 'online') {
          this.sseStatus = 'online'
          this.sseRetries = 0
          this.stopPolling()
        }
      }

      es.addEventListener('job', onJob)
      es.addEventListener('open', () => {
        this.sseStatus = 'online'
        this.sseRetries = 0
        this.stopPolling()
      })
      es.onerror = () => {
        // 浏览器原生重连不可控（也可能一直卡在 CONNECTING），这里手动接管
        this._onSseDown()
      }
    },

    _onSseDown() {
      if (this._es) {
        try {
          this._es.close()
        } catch (_) {
          /* ignore */
        }
        this._es = null
      }
      if (this.sseStatus !== 'offline') {
        this.sseStatus = 'offline'
        // 降级：SSE 不可用时用轮询兜底
        this.startPolling()
      }
      this._scheduleReconnect()
    },

    _scheduleReconnect() {
      if (this._retryTimer) return
      this.sseRetries = Math.min(this.sseRetries + 1, 6)
      const delay = Math.min(1000 * 2 ** (this.sseRetries - 1), 15000)
      this._retryTimer = setTimeout(() => {
        this._retryTimer = null
        if (this.sseStatus !== 'online') this.startEvents()
      }, delay)
    },

    /** 轮询兜底（SSE 掉线时每 5 秒拉一次进行中的任务） */
    startPolling(interval = 5000) {
      if (this._pollTimer) return
      const tick = async () => {
        try {
          await this.loadActiveJobs()
        } catch (_) {
          /* 后端没起来时静默失败 */
        }
      }
      tick()
      this._pollTimer = setInterval(tick, interval)
    },

    stopPolling() {
      if (this._pollTimer) {
        clearInterval(this._pollTimer)
        this._pollTimer = null
      }
    },

    reconnectEvents() {
      if (this._retryTimer) {
        clearTimeout(this._retryTimer)
        this._retryTimer = null
      }
      this.sseRetries = 0
      if (this._es) {
        try {
          this._es.close()
        } catch (_) {
          /* ignore */
        }
        this._es = null
      }
      this.startEvents()
    },

    stopEvents() {
      if (this._retryTimer) {
        clearTimeout(this._retryTimer)
        this._retryTimer = null
      }
      if (this._es) {
        try {
          this._es.close()
        } catch (_) {
          /* ignore */
        }
        this._es = null
      }
      this.stopPolling()
      this.sseStatus = 'idle'
    }
  }
})
