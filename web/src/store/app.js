import { defineStore } from 'pinia'
import api from '@/api'

/** /api/stats 的复用窗口：切页太频繁时不必每次都打一次（顶栏刷新会强制拉） */
const STATS_TTL = 15000

/* 进行中的请求：放模块作用域，不把 Promise 塞进 pinia 变成响应式数据 */
let accountsInflight = null
let statsInflight = null
let statsAt = 0

export const useAppStore = defineStore('app', {
  state: () => ({
    accounts: [],
    accountsLoaded: false,
    accountsError: '',
    settings: null,
    stats: null,
    jobs: {},            // id -> 任务对象
    jobTick: 0,          // 每收到一次任务事件 +1，供视图 watch
    refreshTick: 0,      // 顶栏「刷新」+1，当前视图 watch 它重新加载自己的数据
    polling: false,
    _pollTimer: null
  }),

  getters: {
    sseConnected: () => false,
    sseStatus: () => 'offline',
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
          this.accountsLoaded = false
          this.accountsError = e?.message || '账号列表加载失败'
          throw e
        })
        .finally(() => {
          accountsInflight = null
        })
      return accountsInflight
    },

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

    triggerRefresh() {
      this.refreshTick++
    },

    /* ---------------- 任务管理（纯轻量短轮询，零长连接占用） ---------------- */

    mergeJobs(items) {
      const map = { ...this.jobs }
      for (const j of items || []) {
        if (j && j.id !== undefined) map[j.id] = { ...(map[j.id] || {}), ...j }
      }
      this.jobs = map
      this.jobTick++
    },

    async loadActiveJobs() {
      const [running, queued] = await Promise.all([
        api.listDownloads({ status: 'running', pageSize: 200 }),
        api.listDownloads({ status: 'queued', pageSize: 200 })
      ])
      this.mergeJobs(running?.items || [])
      this.mergeJobs(queued?.items || [])
      
      // 如果没有正在运行的任务，自动退火停止轮询
      if (!running?.items?.length && !queued?.items?.length && !this._forcePolling) {
        this.stopPolling()
      }
    },

    /** 仅在需要时按需短轮询（如任务页或有后台下载任务时） */
    startPolling(interval = 3500, force = false) {
      if (force) this._forcePolling = true
      if (this._pollTimer) return
      this.polling = true
      const tick = async () => {
        try {
          await this.loadActiveJobs()
        } catch (_) {
          /* 静默失败 */
        }
      }
      tick()
      this._pollTimer = setInterval(tick, interval)
    },

    stopPolling() {
      this._forcePolling = false
      if (this._pollTimer) {
        clearInterval(this._pollTimer)
        this._pollTimer = null
      }
      this.polling = false
    },

    // 兼容旧调用接口
    startEvents() {
      // 不再发起 EventSource 长连接，释放 HTTP 连接池
      this.loadActiveJobs().catch(() => {})
    },

    stopEvents() {
      this.stopPolling()
    },

    reconnectEvents() {
      this.loadActiveJobs().catch(() => {})
    }
  }
})
