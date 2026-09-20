import { defineStore } from 'pinia'
import api from '@/api'

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
    settings: null,
    stats: null,
    jobs: {},            // id -> 任务对象
    jobTick: 0,          // 每收到一次任务事件 +1，供视图 watch
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
      try {
        const data = await api.listAccounts()
        this.accounts = Array.isArray(data) ? data : (data?.items ?? [])
        this.accountsLoaded = true
      } catch (e) {
        this.accountsLoaded = true
        throw e
      }
      return this.accounts
    },

    async loadSettings(force = false) {
      if (this.settings && !force) return this.settings
      this.settings = await api.getSettings()
      return this.settings
    },

    async loadStats() {
      this.stats = await api.stats()
      return this.stats
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
      const res = await api.listDownloads({ status: 'running', pageSize: 200 })
      this.mergeJobs(res?.items || [])
      const queued = await api.listDownloads({ status: 'queued', pageSize: 200 })
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
