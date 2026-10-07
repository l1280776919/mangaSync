<template>
  <div class="rd" :class="{ 'rd-dark': dark }" :style="{ '--reading-width': readingWidth + 'px' }">
    <!-- 顶栏：浮层（隐藏时不再挤占滚动区域，避免翻页位置跳动） -->
    <div class="rd-bar" :class="{ 'is-hidden': !barVisible }" :inert="!barVisible ? '' : undefined">
      <div class="rd-left">
        <el-button text class="rd-back" @click="back">
          <el-icon><ArrowLeft /></el-icon><span class="rd-back-txt">返回</span>
        </el-button>
        <div class="rd-title">
          <div class="rd-name">{{ title }}</div>
          <div class="rd-sub">{{ meta.chapterTitle || ('第 ' + order + ' 章') }}</div>
        </div>
      </div>

      <div class="rd-right">
        <div class="rd-page">
          <template v-if="meta.pages">{{ page }} / {{ meta.pages }}</template>
          <template v-else>— / —</template>
          <span v-if="meta.local" class="rd-local" title="本地已下载，秒开">本地</span>
          <span class="rd-origin-tag" title="当前以无损原画画质呈现">原画</span>
        </div>

        <el-select aria-label="选择章节" :model-value="order" class="rd-chap" size="small" :teleported="true" @change="switchChapter">
          <el-option v-for="ch in chapters" :key="ch.order" :label="chapLabel(ch)" :value="ch.order" />
        </el-select>

        <div class="rd-grp">
          <el-button aria-label="上一章" size="small" :disabled="!previousChapter || loading" @click="switchChapter(previousChapter?.order)">
            <el-icon><DArrowLeft /></el-icon>
          </el-button>
          <el-button aria-label="下一章" size="small" :disabled="!nextChapter || loading" @click="switchChapter(nextChapter?.order)">
            <el-icon><DArrowRight /></el-icon>
          </el-button>
        </div>

        <!-- 宽屏：完整控件；窄屏：收进「更多」菜单（手机上原来被隐藏、没法切适宽/适高） -->
        <el-radio-group v-if="!compact" v-model="fit" size="small" class="rd-fit" @change="applyFit">
          <el-radio-button label="width">适宽</el-radio-button>
          <el-radio-button label="height">适高</el-radio-button>
          <el-radio-button label="original">原始</el-radio-button>
        </el-radio-group>

        <el-button v-if="!compact" size="small" class="rd-theme" :title="dark ? '浅色' : '深色'" @click="toggleDark">
          <el-icon><component :is="dark ? 'Sunny' : 'Moon'" /></el-icon>
        </el-button>
        <el-button size="small" class="rd-fullscreen" :title="isFullscreen ? '退出全屏' : '全屏阅读'" @click="toggleFullscreen">
          <el-icon><FullScreen /></el-icon>
        </el-button>

        <el-button size="small" class="rd-more" aria-label="阅读设置" @click="settingsOpen = true">
          <el-icon><Setting /></el-icon><span v-if="!compact">设置</span>
        </el-button>
      </div>
    </div>

    <!-- 图片区 -->
    <div
      ref="scroller"
      tabindex="0"
      role="region"
      aria-label="漫画阅读区"
      class="rd-scroll"
      :class="['rd-fit-' + fit, { 'is-bar': barVisible, 'rd-snap': fit === 'height' }]"
      @scroll.passive="onScroll"
      @touchstart.passive="onTouchStart"
      @touchend="onTouchEnd"
      @touchcancel="touchStart = null"
      @click="onTap"
    >
      <div class="rd-pages">
        <div v-for="p in pageList" :key="kind + comicId + order + ':' + p" class="rd-item" :data-page="p" :style="itemStyle(p)">
          <ReaderPageImage
            v-if="shouldRender(p)"
            :src="imageUrl(p)"
            :page="p"
            :failed="!!failed[p]"
            :priority="p === page ? 'high' : 'auto'"
            @load="onLoaded(p, $event)"
            @error="onError(p, $event)"
          />
          <div v-else class="rd-ph"><span>{{ p }}</span></div>
          <div v-if="failed[p]" class="rd-bad">
            第 {{ p }} 页加载失败
            <el-button text size="small" @click.stop="retry(p)">重试</el-button>
          </div>
        </div>
      </div>

      <div v-if="loading" class="rd-tip" role="status">正在加载…</div>
      <div v-else-if="loadError" class="rd-tip rd-tip-err" role="alert">
        {{ loadError }}
        <el-button text size="small" @click.stop="load">重试</el-button>
        <el-button text size="small" @click.stop="router.push('/accounts')">检查源账号</el-button>
        <el-button text size="small" @click.stop="repairChapter">补下载本章</el-button>
      </div>

      <div v-if="meta.pages" class="rd-end">
        <el-button :disabled="!previousChapter || loading" @click.stop="switchChapter(previousChapter?.order)">上一章</el-button>
        <span class="rd-end-tip">{{ meta.chapterTitle || '' }} 完</span>
        <el-button type="primary" :disabled="!nextChapter || loading" @click.stop="switchChapter(nextChapter?.order)">
          下一章
        </el-button>
      </div>
    </div>
    <nav class="rd-bottom" :class="{ 'is-hidden': !barVisible }" :inert="!barVisible ? '' : undefined" aria-label="阅读导航">
      <button class="rd-control" aria-label="上一页" :disabled="loading || page <= 1" @click="flip(-1)">‹</button>
      <input class="rd-seek" type="range" aria-label="跳转页码" :min="1" :max="Math.max(1, meta.pages)" :value="page" :disabled="loading || !meta.pages" @change="jumpPage($event.target.value)" />
      <label class="rd-jump"><input aria-label="当前页码" type="number" inputmode="numeric" :min="1" :max="meta.pages" :value="page" :disabled="loading || !meta.pages" @change="jumpPage($event.target.value)" @keydown.enter="$event.target.blur()" /> / {{ meta.pages || '—' }}</label>
      <button class="rd-control" aria-label="下一页" :disabled="loading || page >= meta.pages" @click="flip(1)">›</button>
    </nav>
    <button v-if="!barVisible" class="rd-peek" aria-label="显示阅读工具栏" @click="toggleBar">{{ page }} / {{ meta.pages || '—' }} · 菜单</button>
    <el-drawer v-model="settingsOpen" title="阅读设置" :direction="compact ? 'btt' : 'rtl'" :size="compact ? 'auto' : '360px'" class="rd-settings">
      <div class="rd-setting-group"><p>章节</p><el-select aria-label="选择阅读章节" :model-value="order" @change="value => { switchChapter(value); settingsOpen = false }"><el-option v-for="ch in chapters" :key="ch.order" :label="chapLabel(ch)" :value="ch.order" /></el-select></div>
      <div class="rd-setting-group">
        <p>显示方式</p>
        <el-radio-group v-model="fit" @change="applyFit">
          <el-radio-button label="width">连续适宽</el-radio-button>
          <el-radio-button label="height">一屏一页</el-radio-button>
          <el-radio-button label="original">原始尺寸</el-radio-button>
        </el-radio-group>
      </div>
      <label v-if="!compact" class="rd-setting-group">阅读宽度 · {{ readingWidth }} px
        <input aria-label="阅读宽度" type="range" min="640" max="1400" step="40" v-model.number="readingWidth" @change="saveWidth" />
      </label>
      <div class="rd-setting-group"><el-switch :model-value="dark" active-text="深色阅读背景" @change="toggleDark" /></div>
      <div class="rd-setting-group"><el-switch v-model="tapToTurn" active-text="轻点画面两侧翻页" @change="saveTapSetting" /></div>
      <p class="rd-help">{{ compact ? '上下滑动阅读，中间轻点显示菜单。支持双指缩放。' : '← / → 翻页；空格 / Shift + 空格滚动；N / P 切章；F 切换适宽与适高；Esc 显示菜单。' }}</p>
      <p class="rd-help">{{ meta.missingPages.length ? `本地缺少 ${meta.missingPages.length} 页，缺页将在线加载` : meta.local ? '当前章节使用本地图片' : '当前章节在线加载' }} · 阅读进度自动保存</p>
      <el-button v-if="Object.keys(failed).length" @click="retryFailed">重试加载失败的图片</el-button>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '@/api'
import ReaderPageImage from '@/components/ReaderPageImage.vue'
import { useIsMobile } from '@/composables/useIsMobile'

import { auth } from '@/store/auth'
import { createPreloader } from '@/utils/preloader'
const preloader = createPreloader()
let loadController = null
let disposed = false
let ready = false
let readingIdentity = null
const retryTimers = new Set()
const route = useRoute()
const router = useRouter()

const kind = computed(() => String(route.params.kind || ''))
const comicId = computed(() => String(route.params.comicId || ''))
const order = computed(() => Number(route.params.order || 1))

const title = ref('')
const chapters = ref([])
const meta = reactive({ missingPages: [], pages: 0, chapterTitle: '', local: false })
const pageList = computed(() => Array.from({ length: meta.pages }, (_, i) => i + 1))
const loading = ref(true)
const loadError = ref('')
const failed = reactive({})
const loaded = reactive({})
const imageAttempts = reactive({})
const visibleRange = reactive({ start: 1, end: 1 })

/** 后端给的每页像素尺寸（本地/缓存命中时才有），用于精确占位 */
const sizes = ref([])
/** 图片实际加载出来后的真实尺寸：优先级最高，保证占位比例最终精确 */
const natSize = reactive({})
/** 正在后台预取的页，避免重复请求 */
const preloading = reactive({})
/**
 * 章节世代：每次 load 递增。
 * 切章后旧章节图片的回调（onload/onerror）一律作废 —— 慢链路下预载的旧图
 * 否则会把新章节的状态写坏（占位骨架提前消失、比例用上一章的尺寸）。
 */
let gen = 0

function setting(key, fallback) { try { return localStorage.getItem(key) ?? fallback } catch (_) { return fallback } }
function persist(key, value) { try { localStorage.setItem(key, String(value)) } catch (_) { /* Reading works without storage. */ } }
const storedFit = setting('ms-reader-fit', 'width')
const fit = ref(['width', 'height', 'original'].includes(storedFit) ? storedFit : 'width')
const settingsOpen = ref(false)
const readingWidth = ref(Math.max(640, Math.min(1400, Number(setting('ms-reader-width', '960')) || 960)))
const tapToTurn = ref(setting('ms-reader-tap', '1') === '1')
const chapterIndex = computed(() => chapters.value.findIndex(ch => ch.order === order.value))
const previousChapter = computed(() => chapters.value[chapterIndex.value - 1])
const nextChapter = computed(() => chapterIndex.value < 0 ? null : chapters.value[chapterIndex.value + 1])
function saveWidth() { persist('ms-reader-width', readingWidth.value); applyFit() }
function saveTapSetting() { persist('ms-reader-tap', tapToTurn.value ? '1' : '0') }
function retryFailed() { Object.keys(failed).forEach(p => retry(Number(p))); settingsOpen.value = false }
const dark = ref(setting('ms-reader-dark', '0') === '1')
const isFullscreen = ref(!!document.fullscreenElement)
const retriedTimes = reactive({})

async function toggleFullscreen() {
  try {
    if (document.fullscreenElement) await document.exitFullscreen?.()
    else if (document.documentElement.requestFullscreen) await document.documentElement.requestFullscreen()
    else ElMessage.info('当前浏览器不支持全屏，可以隐藏工具栏继续阅读')
  } catch (_) { ElMessage.info('全屏未能开启，请使用浏览器的全屏功能') }
}
function onFullscreenChange() { isFullscreen.value = !!document.fullscreenElement }
const barVisible = ref(true)
const page = ref(1)
const scroller = ref(null)

/* 手机端判定：与全站断点一致（≤768px） */
const isMobile = useIsMobile()
const compact = isMobile

/**
 * 视口虚拟化渲染窗口（Windowing）：
 * 彻底解决高分辨率 GIF 动图或超长画册几十张大图同时常驻 DOM 导致浏览器标签页显存/内存撑爆崩溃（Aw, Snap!）。
 * 仅在视口附近挂载 <img>，视口外的通过精确 CSS 占位骨架撑起完整高度，滚动条平滑不跳动。
 */
const WINDOW_BEHIND = 2
const WINDOW_AHEAD = 4

function shouldRender(p) {
  const cur = page.value || 1
  return (p >= cur - WINDOW_BEHIND && p <= cur + WINDOW_AHEAD) || (p >= visibleRange.start && p <= visibleRange.end)
}

const DEFAULT_RATIO = '2 / 3' // 拿不到真实尺寸时的兜底比例（常见漫画页）

const chapLabel = (ch) => {
  const t = (ch.title || '').trim()
  return chapters.value.length > 1 ? `${ch.order}. ${t || '未命名'}` : (t || '第 1 章')
}

const pageUrl = (p) =>
  `/api/reader/${kind.value}/${encodeURIComponent(comicId.value)}/${order.value}/page/${p}`

const imageUrl = p => pageUrl(p) + (imageAttempts[p] ? `?retry=${imageAttempts[p]}` : '')

const storeKey = computed(() => `ms-reader:${auth.user?.username || "local"}:${kind.value}:${comicId.value}`)

/** 每页占位样式：优先真实尺寸 → 后端尺寸 → 兜底比例 */
function itemStyle(p) {
  const s = natSize[p] || sizes.value[p - 1]
  const w = s && s[0] > 0 ? s[0] : 0
  const h = s && s[1] > 0 ? s[1] : 0
  const ratio = w && h ? `${w} / ${h}` : DEFAULT_RATIO

  if (fit.value === 'height') return {} // 一屏一页，高度由 CSS 控制
  if (fit.value === 'original') {
    // 原始尺寸：按真实像素宽展示（拿不到就退回适宽比例块）
    return w ? { aspectRatio: ratio, width: w + 'px', maxWidth: 'none' } : { aspectRatio: ratio }
  }
  return { aspectRatio: ratio } // 适宽
}

function saveProgress(keepalive = false) {
  if (!ready || disposed || !meta.pages || !readingIdentity) return
  const progress = { order: readingIdentity.order, page: page.value, title: readingIdentity.title, updatedAt: Date.now() }
  api.saveReading(readingIdentity.kind, readingIdentity.comicId, progress, keepalive).catch(() => {})
  try {
    localStorage.setItem(readingIdentity.key, JSON.stringify(progress))
  } catch (e) {
    /* 隐私模式下写不了，忽略 */
  }
}

function readProgress() {
  try {
    return JSON.parse(localStorage.getItem(storeKey.value) || 'null')
  } catch (e) {
    return null
  }
}

async function load() {
  const my = ++gen
  clearTimeout(saveTimer)
  if (rafId) { cancelAnimationFrame(rafId); rafId = null }
  ready = false
  loadController?.abort()
  loadController = new AbortController()
  const signal = loadController.signal
  preloader.reset()
  for (const timer of retryTimers) clearTimeout(timer)
  retryTimers.clear()
  Object.keys(retriedTimes).forEach(k => delete retriedTimes[k])
  meta.pages = 0
  meta.chapterTitle = ''
  meta.local = false
  meta.missingPages = []
  page.value = 1
  loading.value = true
  loadError.value = ''
  Object.keys(failed).forEach((k) => delete failed[k])
  Object.keys(loaded).forEach((k) => delete loaded[k])
  Object.keys(imageAttempts).forEach(k => delete imageAttempts[k])
  visibleRange.start = visibleRange.end = 1
  Object.keys(preloading).forEach((k) => delete preloading[k])
  // 上一章测出来的真实尺寸也要清：否则新章节的占位比例会沿用上一章的数字
  Object.keys(natSize).forEach((k) => delete natSize[k])
  sizes.value = []
  try {
    // 本子信息（章节列表）：失败也不阻塞阅读
    if (!chapters.value.length) {
      try {
        const c = await api.comic(kind.value, comicId.value, signal, true)
        if (my !== gen) return // 已经切章，丢弃这次结果
        title.value = c.title || c.comicId
        chapters.value = (Array.isArray(c.chapters) ? c.chapters : []).map(x => ({ order: Number(x.order), title: x.title })).filter(x => Number.isInteger(x.order) && x.order > 0).sort((a, b) => a.order - b.order)
        if (!chapters.value.length) chapters.value = [{ order: order.value, title: '' }]
      } catch (e) {
        if (my !== gen) return
        title.value = comicId.value
        chapters.value = [{ order: order.value, title: '' }]
      }
    }
    const m = await api.readerMeta(kind.value, comicId.value, order.value, signal)
    if (my !== gen) return // 快速连切章节时，旧响应不能覆盖新章节
    meta.pages = m.pages || 0
    meta.chapterTitle = m.chapterTitle || ''
    meta.local = !!m.local
    meta.missingPages = m.missingPages || []
    // 后端返回的每页尺寸（[[w,h], ...]，缺页为 [0,0]）
    sizes.value = Array.isArray(m.sizes) ? m.sizes : []
    if (!meta.pages) {
      loadError.value = m.error || '这一章拿不到图片'
    } else {
      readingIdentity = { kind: kind.value, comicId: comicId.value, order: order.value, title: title.value, key: storeKey.value }
      await nextTick()
      if (my !== gen || disposed) return
      ready = true
      restorePosition()
      preload(page.value)
    }
  } catch (e) {
    if (my !== gen) return
    loadError.value = e?.message || '加载失败'
  } finally {
    // 只有最后一次 load 有权关掉 loading（否则新请求还在跑，骨架屏就没了）
    if (my === gen) { loading.value = false; await nextTick(); if (my === gen) updateCurrentPage() }
  }
}

/** 恢复上次读到哪一页（占位撑开高度后 offsetTop 已可靠，不必久等） */
function restorePosition() {
  const saved = readProgress()
  const requested = Number(route.query.page)
  const target = requested > 0 ? requested : (saved?.order === order.value ? saved.page : 1)
  scrollToPage(Math.min(meta.pages, Math.max(1, Number(target) || 1)), false)
}

function jumpPage(value) {
  const p = Math.min(meta.pages, Math.max(1, Math.trunc(Number(value)) || 1))
  if (!ready || !meta.pages) return
  scrollToPage(p, false)
  saveProgress()
  preload(p)
}
function scrollToPage(p, smooth = false) {
  const el = scroller.value?.querySelector(`.rd-item[data-page="${p}"]`)
  if (!el) return
  scroller.value.scrollTo({ top: Math.max(0, el.offsetTop - 58), behavior: smooth ? 'smooth' : 'auto' })
  page.value = p
  visibleRange.start = visibleRange.end = p
}

/** 滚动时更新「当前页」并记录进度（rAF 节流，避免每帧跑 querySelectorAll） */
let saveTimer = null
let rafId = null
function onScroll() {
  if (rafId) return
  rafId = requestAnimationFrame(() => {
    rafId = null
    updateCurrentPage()
  })
}

function updateCurrentPage() {
  const sc = scroller.value
  if (!sc || !ready || loading.value) return
  const probe = sc.scrollTop + sc.clientHeight * 0.4
  // Query the page container once; binary search only reads O(log n) offsets.
  const container = sc.querySelector('.rd-item')?.parentElement
  const pages = container?.children || []
  function pageAt(y) {
    let lo = 0, hi = pages.length - 1, result = 1
    while (lo <= hi) {
      const mid = (lo + hi) >> 1
      if (pages[mid].offsetTop <= y) { result = Number(pages[mid].dataset.page) || result; lo = mid + 1 }
      else hi = mid - 1
    }
    return result
  }
  // Cover the actual viewport as well as nearby page numbers (short/landscape pages).
  visibleRange.start = pageAt(Math.max(0, sc.scrollTop - sc.clientHeight * 0.5))
  visibleRange.end = pageAt(sc.scrollTop + sc.clientHeight * 1.5)
  const cur = pageAt(probe)
  if (cur !== page.value) {
    page.value = cur
    clearTimeout(saveTimer)
    saveTimer = setTimeout(saveProgress, 500)
    preload(cur)
  }
}

/**
 * 原画画质下的智能分级并发预加载引擎：
 * - 深度拓展：向前预取 1 页，向后预取 8 页
 * - 并发控制：最大 2 个后台下载并发，避免拥塞视口正在渲染的当前原画
 * - 有序调度：按离当前页的距离由近及远有序排队
 */
function preload(cur) {
  if (!meta.pages || !loaded[cur]) return // Visible page always wins over speculative work.
  const ahead = navigator.connection?.saveData ? 1 : 4
  const needed = []
  for (let distance = 1; distance <= ahead; distance++) {
    for (const p of [cur + distance, cur - distance]) {
      if (p >= 1 && p <= meta.pages && !loaded[p] && !failed[p]) needed.push(p)
    }
  }
  preloader.schedule(needed, pageUrl, (p, w, h) => {
    if (w && h) setNaturalSize(p, w, h)
  })
}

let anchoring = false
function setNaturalSize(p, w, h) {
  const sc = scroller.value
  const anchor = sc?.querySelector(`.rd-item[data-page="${page.value}"]`)
  const before = anchor?.offsetTop
  const anchorPage = page.value
  const beforeScroll = sc?.scrollTop
  const my = gen
  natSize[p] = [w, h]
  if (anchoring || !anchor || p >= page.value) return
  anchoring = true
  nextTick(() => {
    anchoring = false
    if (!disposed && my === gen && sc && anchor.isConnected && page.value === anchorPage && Math.abs(sc.scrollTop - beforeScroll) < 1) sc.scrollTop += anchor.offsetTop - before
  })
}

function onLoaded(p, e) {
  if (disposed || !ready || !e?.target?.isConnected || !e.target.src.includes(pageUrl(p))) return
  loaded[p] = true
  delete failed[p]
  const im = e?.target
  if (im && im.naturalWidth) setNaturalSize(p, im.naturalWidth, im.naturalHeight)
  if (p === page.value) preload(p)
}
function onError(p, e) {
  if (disposed || !ready || !e?.target?.isConnected || !e.target.src.includes(pageUrl(p))) return
  delete loaded[p]
  if (!retriedTimes[p]) {
    retriedTimes[p] = 1
    const my = gen
    const timer = setTimeout(() => {
      retryTimers.delete(timer)
      if (my === gen && !disposed) retry(p)
    }, 600)
    retryTimers.add(timer)
    return
  }
  failed[p] = true
  delete loaded[p]
  preloading[p] = false
}
function retry(p) {
  delete failed[p]
  preloading[p] = false
  delete loaded[p]
  imageAttempts[p] = (imageAttempts[p] || 0) + 1
}

function flip(dir) {
  if (!ready || !meta.pages) return
  if (fit.value === 'height' || fit.value === 'width') {
    // 逐页滚动：适高模式一屏一页，适宽模式滚一页
    const target = Math.min(Math.max(page.value + dir, 1), meta.pages || 1)
    scrollToPage(target)
  } else {
    scroller.value?.scrollBy({ top: dir * scroller.value.clientHeight * 0.9, behavior: 'smooth' })
  }
  saveProgress()
}

function toggleBar() {
  barVisible.value = !barVisible.value
}

function toggleDark() {
  dark.value = !dark.value
  persist('ms-reader-dark', dark.value ? '1' : '0')
}

function applyFit() {
  persist('ms-reader-fit', fit.value)
  const current = page.value
  nextTick(() => scrollToPage(current, false))
}

function switchChapter(next) {
  const n = Number(next)
  if (!chapters.value.some(ch => ch.order === n) || n === order.value) return
  saveProgress()
  router.replace(`/reader/${kind.value}/${encodeURIComponent(comicId.value)}/${n}`)
}

async function repairChapter() {
  try {
    const accounts = await api.listAccounts()
    const account = (Array.isArray(accounts) ? accounts : accounts.items || []).find(a => a.kind === kind.value)
    if (!account) return router.push('/accounts')
    await api.createDownload({ kind: kind.value, comicId: comicId.value, accountId: account.id, chapters: [order.value], title: title.value })
    loadError.value = '已加入下载队列，下载完成后点击重试'
  } catch (e) { ElMessage.error(e?.message || '加入下载队列失败，请重试') }
}
function back() {
  saveProgress()
  router.replace('/library')
}

/* ---------- 点按手势：左/右翻页、中间切换工具栏 ---------- */
/* 原来用一层全屏透明点击区（.rd-zones），会吃掉图片区的点击与滚动；
   现在改成：触屏用 touchstart/touchend 判断「轻点 vs 拖动」，不拦截滚动。 */
let touchStart = null
let lastTouchAt = 0

function interactive(target) { return !!target?.closest?.('button, a, input, select, textarea, [role=button], [contenteditable=true]') }
function onTouchStart(e) {
  if (e.touches?.length !== 1 || interactive(e.target)) { touchStart = null; return }
  const t = e.touches && e.touches[0]
  if (!t) return
  touchStart = { x: t.clientX, y: t.clientY, at: Date.now() }
}

function onTouchEnd(e) {
  lastTouchAt = Date.now()
  if (!touchStart || e.touches?.length || interactive(e.target)) { touchStart = null; return }
  const t = (e.changedTouches && e.changedTouches[0]) || null
  const start = touchStart
  touchStart = null
  if (!t) return
  const dx = Math.abs(t.clientX - start.x)
  const dy = Math.abs(t.clientY - start.y)
  const dt = Date.now() - start.at
  // 位移超过 12px 或超过 500ms 视为滚动/长按，不当作翻页
  if (dx > 12 || dy > 12 || dt > 500) return
  const sc = scroller.value
  if (!sc) return
  const r = sc.getBoundingClientRect()
  const x = (t.clientX - r.left) / r.width
  if (tapToTurn.value && x < 0.3) flip(-1)
  else if (tapToTurn.value && x > 0.7) flip(1)
  else toggleBar()
}

/** 宽屏鼠标：点图片中间区域切换工具栏（触屏刚处理过的合成 click 直接忽略） */
function onTap(e) {
  if (interactive(e.target)) return
  if (Date.now() - lastTouchAt < 600) return // 触屏设备上紧跟的合成 click，避免双动作
  const sc = scroller.value
  if (!sc) return
  const r = sc.getBoundingClientRect()
  const x = (e.clientX - r.left) / r.width
  if (x > 0.35 && x < 0.65) toggleBar()
}

function onKey(e) {
  if (settingsOpen.value || e.ctrlKey || e.metaKey || e.altKey || interactive(e.target) || e.target?.closest?.('[role=listbox], [role=dialog]')) return
  switch (e.key) {
    case ' ':
      scroller.value?.scrollBy({ top: (e.shiftKey ? -1 : 1) * scroller.value.clientHeight * 0.85, behavior: 'auto' })
      e.preventDefault()
      break
    case 'Home': jumpPage(1); e.preventDefault(); break
    case 'End': jumpPage(meta.pages); e.preventDefault(); break
    case 'ArrowRight':
    case 'PageDown':
      flip(1)
      e.preventDefault()
      break
    case 'ArrowLeft':
    case 'PageUp':
      flip(-1)
      e.preventDefault()
      break
    case 'n':
      switchChapter(nextChapter.value?.order)
      break
    case 'p':
      switchChapter(previousChapter.value?.order)
      break
    case 'f':
      fit.value = fit.value === 'width' ? 'height' : 'width'
      applyFit()
      break
    case 'Escape':
      barVisible.value = true
      break
  }
}

watch(() => route.fullPath, (_, old) => {
  saveProgress()
  if (!old || readingIdentity?.kind !== kind.value || readingIdentity?.comicId !== comicId.value) chapters.value = []
  load()
})

async function initialize() {
  if (route.query.resume === '1') {
    let saved = readProgress()
    try {
      const remote = await api.reading(kind.value, comicId.value)
      if (remote && (!saved || remote.updatedAt > (saved.updatedAt || 0))) saved = remote
    } catch (_) { /* Offline local progress remains available. */ }
    if (disposed) return
    const path = `/reader/${kind.value}/${encodeURIComponent(comicId.value)}/${saved?.order || order.value}`
    await router.replace({ path, query: { page: saved?.page || 1 } })
  } else await load()
}
let resizeTimer = null
function onResize() { const current = page.value; clearTimeout(resizeTimer); resizeTimer = setTimeout(() => { if (ready && !disposed) scrollToPage(current, false) }, 120) }
function onPageHide() { saveProgress(true) }
function onVisibility() { if (document.hidden) saveProgress(true) }
onMounted(() => {
  window.addEventListener('keydown', onKey)
  window.addEventListener('pagehide', onPageHide)
  document.addEventListener('visibilitychange', onVisibility)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  window.addEventListener('resize', onResize)
  initialize()
})
onBeforeUnmount(() => {
  saveProgress(true)
  disposed = true
  gen++
  loadController?.abort()
  preloader.reset()
  clearTimeout(saveTimer)
  for (const timer of retryTimers) clearTimeout(timer)
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('pagehide', onPageHide)
  document.removeEventListener('visibilitychange', onVisibility)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  window.removeEventListener('resize', onResize)
  clearTimeout(resizeTimer)
  if (rafId) cancelAnimationFrame(rafId)
})

</script>

<style scoped>
.rd {
  position: fixed;
  inset: 0;
  z-index: 2000;
  display: flex;
  flex-direction: column;
  background: #eef0f3;
  color: #1f2329;
}
.rd-dark {
  background: #16181d;
  color: #e6e8ee;
}

/* ---------- 顶栏（浮层） ---------- */
.rd-bar {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: calc(8px + env(safe-area-inset-top)) max(12px, env(safe-area-inset-right)) 8px max(12px, env(safe-area-inset-left));
  background: rgba(255, 255, 255, 0.94);
  border-bottom: 1px solid #e5e7eb;
  backdrop-filter: blur(8px);
  transition: transform 0.22s ease, opacity 0.22s ease;
}
.rd-bar.is-hidden {
  transform: translateY(-100%);
  opacity: 0;
  pointer-events: none;
}
.rd-dark .rd-bar {
  background: rgba(26, 29, 35, 0.94);
  border-bottom-color: #2c313b;
}
.rd-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}
.rd-back {
  flex: none;
}
.rd-title {
  min-width: 0;
}
.rd-name {
  font-size: 14px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 40vw;
}
.rd-sub {
  font-size: 12px;
  opacity: 0.62;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 40vw;
}
.rd-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}
.rd-page {
  font-variant-numeric: tabular-nums;
  font-size: 13px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  opacity: 0.85;
  white-space: nowrap;
}
.rd-origin-tag {
  font-size: 11px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--el-color-primary-light-9, #eff4fe);
  color: var(--el-color-primary, #2f6fed);
  border: 1px solid var(--el-color-primary-light-7, #c8d9fb);
}
.rd-local {
  font-size: 11px;
  padding: 0 5px;
  border-radius: 4px;
  background: #e8f4ea;
  color: #2c7a43;
}
.rd-chap {
  width: 168px;
}
.rd-grp {
  display: inline-flex;
  gap: 4px;
}

/* ---------- 图片区 ---------- */
.rd-scroll {
  flex: 1;
  overflow-y: auto;
  overflow-x: auto;
  position: relative;
  -webkit-overflow-scrolling: touch;
  overscroll-behavior: contain;
  padding: calc(58px + env(safe-area-inset-top)) 0 calc(72px + env(safe-area-inset-bottom));
  scrollbar-gutter: stable;
  overflow-anchor: none;
}

.rd-scroll.rd-snap {
  /* proximity 比 mandatory 柔和：不会「吸」得太急，滑动更跟手 */
  scroll-snap-type: y proximity;
}
.rd-pages {
  max-width: min(100%, var(--reading-width));
  margin: 0 auto;
}
.rd-item {
  position: relative;
  overflow: hidden;
  background: #fff;
  margin: 0 auto;
}
.rd-dark .rd-item {
  background: #1b1e24;
}
/* 图片绝对定位铺满占位块：占位比例与图片一致时无变形 */
.rd-item :deep(img) {
  image-rendering: auto;
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
  opacity: 0;
  transition: none;
}
.rd-item :deep(img).is-loaded {
  opacity: 1;
}
.rd-fit-width .rd-item {
  width: 100%;
}
.rd-fit-height .rd-item {
  height: calc(100dvh - 132px - env(safe-area-inset-top) - env(safe-area-inset-bottom));
  min-height: 200px;
  scroll-snap-align: start;
}
.rd-fit-height .rd-item :deep(img) {
  image-rendering: auto;
  object-fit: contain;
}
.rd-fit-original .rd-pages {
  width: max-content;
  max-width: none;
  margin: 0 auto;
}
.rd-fit-original .rd-item :deep(img) {
  image-rendering: auto;
  object-fit: contain;
}

/* 占位骨架 */
:deep(.rd-ph) {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  color: #aab2bd;
  background: linear-gradient(100deg, #f3f5f8 30%, #e8ecf1 50%, #f3f5f8 70%);
  background-size: 200% 100%;
  animation: rd-shimmer 1.5s linear infinite;
}
.rd-dark :deep(.rd-ph) {
  color: #5f6774;
  background: linear-gradient(100deg, #20242b 30%, #272c35 50%, #20242b 70%);
  background-size: 200% 100%;
}
@keyframes rd-shimmer {
  from {
    background-position: 200% 0;
  }
  to {
    background-position: -200% 0;
  }
}

.rd-bad {
  position: absolute;
  left: 50%;
  bottom: 12px;
  transform: translateX(-50%);
  z-index: 5;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
  font-size: 13px;
  color: #b3352f;
  background: rgba(255, 255, 255, 0.94);
  padding: 6px 10px;
  border-radius: 8px;
}
.rd-tip {
  position: relative;
  z-index: 5;
  text-align: center;
  padding: 40px 0;
  font-size: 13px;
  opacity: 0.7;
}
.rd-tip-err {
  color: #b3352f;
  opacity: 1;
}
.rd-end {
  position: relative;
  z-index: 5;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 28px 12px 40px;
  font-size: 13px;
  opacity: 0.9;
  flex-wrap: wrap;
}
.rd-end-tip {
  opacity: 0.6;
}

/* ---------- 手机端（≤768px，与全站断点一致） ---------- */
@media (max-width: 768px) {
  .rd-bar {
    padding: calc(6px + env(safe-area-inset-top)) 8px 6px;
    gap: 6px;
  }
  .rd-back-txt {
    display: none;
  }
  .rd-name,
  .rd-sub {
    max-width: 34vw;
  }
  .rd-chap {
    width: 116px;
  }
  .rd-pages { max-width: 100%; }
  .rd-chap, .rd-grp, .rd-page, .rd-theme, .rd-fit { display: none; }
  .rd-title { flex: 1; }
  .rd-name, .rd-sub { max-width: 55vw; }
  .rd-right { gap: 4px; }
  .rd-bar .el-button { min-height: 40px; }
}

.rd-bottom { position: absolute; bottom: 0; left: 0; right: 0; z-index: 30; display: flex; align-items: center; justify-content: center; gap: 12px; padding: 8px max(16px, env(safe-area-inset-right)) calc(8px + env(safe-area-inset-bottom)); background: rgba(255,255,255,.95); border-top: 1px solid #dce1e8; backdrop-filter: blur(12px); transition: transform .2s, opacity .2s; }
.rd-bottom.is-hidden { transform: translateY(100%); opacity: 0; pointer-events: none; }
.rd-dark .rd-bottom { background: rgba(26,29,35,.95); border-color: #353b46; }
.rd-control { width: 44px; height: 44px; border: 1px solid #9099a650; border-radius: 12px; background: transparent; color: inherit; font-size: 28px; cursor: pointer; }
.rd-control:disabled { opacity: .3; cursor: default; }
.rd-seek { flex: 1; max-width: 560px; min-width: 40px; height: 40px; accent-color: #527dce; }
.rd-jump { white-space: nowrap; font-size: 13px; font-variant-numeric: tabular-nums; }
.rd-jump input { width: 54px; height: 40px; text-align: center; border: 1px solid #9099a650; border-radius: 8px; color: inherit; background: transparent; font: inherit; }
.rd-peek { position: absolute; right: max(16px, env(safe-area-inset-right)); bottom: calc(12px + env(safe-area-inset-bottom)); z-index: 30; padding: 10px 16px; border: 1px solid #ffffff30; border-radius: 24px; color: #fff; background: #202630cc; cursor: pointer; }
.rd-setting-group { display: block; margin-bottom: 24px; }
.rd-setting-group input[type=range] { display: block; width: 100%; margin-top: 16px; }
.rd-setting-group .el-radio-group { display: flex; flex-wrap: wrap; gap: 4px; }
.rd-help { font-size: 13px; line-height: 1.8; color: #77818d; }
.rd-scroll:focus-visible, .rd-control:focus-visible, .rd-peek:focus-visible { outline: 2px solid #527dce; outline-offset: -2px; }
@media (max-width: 768px) { :global(.rd-settings) { max-height: 85dvh; padding-bottom: env(safe-area-inset-bottom); } }
@media (prefers-reduced-motion: reduce) { .rd *, .rd *::before { animation: none !important; transition: none !important; scroll-behavior: auto !important; } }
@media (min-width: 769px) and (max-width: 1150px) { .rd-fit, .rd-grp { display: none; } .rd-name, .rd-sub { max-width: 24vw; } }
</style>
