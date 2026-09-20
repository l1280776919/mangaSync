<template>
  <div class="rd" :class="{ 'rd-dark': dark }">
    <!-- 顶栏：浮层（隐藏时不再挤占滚动区域，避免翻页位置跳动） -->
    <div class="rd-bar" :class="{ 'is-hidden': !barVisible }">
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
        </div>

        <el-select v-model="order" class="rd-chap" size="small" :teleported="true" @change="switchChapter">
          <el-option v-for="ch in chapters" :key="ch.order" :label="chapLabel(ch)" :value="ch.order" />
        </el-select>

        <div class="rd-grp">
          <el-button size="small" :disabled="order <= 1" @click="switchChapter(order - 1)">
            <el-icon><DArrowLeft /></el-icon>
          </el-button>
          <el-button size="small" :disabled="order >= chapters.length" @click="switchChapter(order + 1)">
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

        <el-dropdown v-if="compact" trigger="click" placement="bottom-end" @command="onCommand">
          <el-button size="small" class="rd-more" title="阅读设置">
            <el-icon><Setting /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="width">适宽</el-dropdown-item>
              <el-dropdown-item command="height">适高</el-dropdown-item>
              <el-dropdown-item command="original">原始尺寸</el-dropdown-item>
              <el-dropdown-item command="theme" divided>{{ dark ? '切到浅色' : '切到深色' }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>

    <!-- 图片区 -->
    <div
      ref="scroller"
      class="rd-scroll"
      :class="['rd-fit-' + fit, { 'is-bar': barVisible, 'rd-snap': fit === 'height' }]"
      @scroll.passive="onScroll"
      @touchstart.passive="onTouchStart"
      @touchend="onTouchEnd"
      @click="onTap"
    >
      <div class="rd-pages">
        <div v-for="p in pageList" :key="p" class="rd-item" :data-page="p" :style="itemStyle(p)">
          <img
            :src="pageUrl(p)"
            :alt="'第 ' + p + ' 页'"
            decoding="async"
            :loading="p <= 3 ? 'eager' : 'lazy'"
            :class="{ 'is-loaded': loaded[p] }"
            @load="onLoaded(p, $event)"
            @error="onError(p)"
          />
          <!-- 未加载时的占位（撑住高度，滚动位置不跳） -->
          <div v-if="!loaded[p] && !failed[p]" class="rd-ph"><span>{{ p }}</span></div>
          <div v-if="failed[p]" class="rd-bad">
            第 {{ p }} 页加载失败
            <el-button text size="small" @click.stop="retry(p)">重试</el-button>
          </div>
        </div>
      </div>

      <div v-if="loading" class="rd-tip">正在加载…</div>
      <div v-else-if="loadError" class="rd-tip rd-tip-err">
        {{ loadError }}
        <el-button text size="small" @click.stop="load">重试</el-button>
      </div>

      <div v-if="meta.pages" class="rd-end">
        <el-button :disabled="order <= 1" @click.stop="switchChapter(order - 1)">上一章</el-button>
        <span class="rd-end-tip">{{ meta.chapterTitle || '' }} 完</span>
        <el-button type="primary" :disabled="order >= chapters.length" @click.stop="switchChapter(order + 1)">
          下一章
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '@/api'
import { useIsMobile } from '@/composables/useIsMobile'

const route = useRoute()
const router = useRouter()

const kind = computed(() => String(route.params.kind || ''))
const comicId = computed(() => String(route.params.comicId || ''))
const order = ref(Number(route.params.order || 1))

const title = ref('')
const chapters = ref([])
const meta = reactive({ pages: 0, chapterTitle: '', local: false })
const pageList = computed(() => Array.from({ length: meta.pages }, (_, i) => i + 1))
const loading = ref(true)
const loadError = ref('')
const failed = reactive({})
const loaded = reactive({})

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

const fit = ref(localStorage.getItem('ms-reader-fit') || 'width')
const dark = ref(localStorage.getItem('ms-reader-dark') === '1')
const barVisible = ref(true)
const page = ref(1)
const scroller = ref(null)

/* 手机端判定：与全站断点一致（≤768px） */
const isMobile = useIsMobile()
const compact = isMobile

const DEFAULT_RATIO = '2 / 3' // 拿不到真实尺寸时的兜底比例（常见漫画页）

const chapLabel = (ch) => {
  const t = (ch.title || '').trim()
  return chapters.value.length > 1 ? `${ch.order}. ${t || '未命名'}` : (t || '第 1 章')
}

const pageUrl = (p) =>
  `/api/reader/${kind.value}/${encodeURIComponent(comicId.value)}/${order.value}/page/${p}`

const storeKey = computed(() => `ms-reader:${kind.value}:${comicId.value}`)

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

function saveProgress() {
  try {
    localStorage.setItem(storeKey.value, JSON.stringify({ order: order.value, page: page.value }))
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
  loading.value = true
  loadError.value = ''
  Object.keys(failed).forEach((k) => delete failed[k])
  Object.keys(loaded).forEach((k) => delete loaded[k])
  Object.keys(preloading).forEach((k) => delete preloading[k])
  // 上一章测出来的真实尺寸也要清：否则新章节的占位比例会沿用上一章的数字
  Object.keys(natSize).forEach((k) => delete natSize[k])
  sizes.value = []
  try {
    // 本子信息（章节列表）：失败也不阻塞阅读
    if (!chapters.value.length) {
      try {
        const c = await api.comic(kind.value, comicId.value)
        if (my !== gen) return // 已经切章，丢弃这次结果
        title.value = c.title || c.comicId
        chapters.value = (c.chapters || []).map((x) => ({ order: x.order, title: x.title }))
        if (!chapters.value.length) chapters.value = [{ order: 1, title: '' }]
      } catch (e) {
        if (my !== gen) return
        title.value = comicId.value
        chapters.value = [{ order: 1, title: '' }]
      }
    }
    const m = await api.readerMeta(kind.value, comicId.value, order.value)
    if (my !== gen) return // 快速连切章节时，旧响应不能覆盖新章节
    meta.pages = m.pages || 0
    meta.chapterTitle = m.chapterTitle || ''
    meta.local = !!m.local
    // 后端返回的每页尺寸（[[w,h], ...]，缺页为 [0,0]）
    sizes.value = Array.isArray(m.sizes) ? m.sizes : []
    if (!meta.pages) loadError.value = m.error || '这一章拿不到图片'
  } catch (e) {
    if (my !== gen) return
    loadError.value = e?.message || '加载失败'
  } finally {
    // 只有最后一次 load 有权关掉 loading（否则新请求还在跑，骨架屏就没了）
    if (my === gen) loading.value = false
  }
}

/** 恢复上次读到哪一页（占位撑开高度后 offsetTop 已可靠，不必久等） */
function restorePosition() {
  const saved = readProgress()
  if (!saved || saved.order !== order.value || !saved.page || saved.page <= 1) return
  nextTick(() => {
    setTimeout(() => scrollToPage(saved.page, false), 60)
  })
}

function scrollToPage(p, smooth = true) {
  const el = scroller.value?.querySelector(`.rd-item[data-page="${p}"]`)
  if (!el) return
  scroller.value.scrollTo({ top: el.offsetTop - 2, behavior: smooth ? 'smooth' : 'auto' })
  page.value = p
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
  if (!sc) return
  const items = sc.querySelectorAll('.rd-item')
  const probe = sc.scrollTop + sc.clientHeight * 0.4
  let cur = 1
  for (const it of items) {
    if (it.offsetTop <= probe) cur = Number(it.dataset.page)
    else break
  }
  if (cur !== page.value) {
    page.value = cur
    clearTimeout(saveTimer)
    saveTimer = setTimeout(saveProgress, 500)
    preload(cur)
  }
}

/** 提前把后面几页塞进浏览器缓存（顺带记录真实尺寸，占位更准） */
function preload(cur) {
  const my = gen // 记住这批预载属于哪一章
  for (let i = cur + 1; i <= Math.min(cur + 3, meta.pages); i++) {
    if (loaded[i] || failed[i] || preloading[i]) continue
    preloading[i] = true
    const im = new Image()
    im.onload = () => {
      if (my !== gen) return // 已经切章：旧图回调作废
      loaded[i] = true
      preloading[i] = false // 成功也要复位，否则这张图再也不会被预载
      if (im.naturalWidth) natSize[i] = [im.naturalWidth, im.naturalHeight]
    }
    im.onerror = () => {
      if (my !== gen) return
      preloading[i] = false
    }
    im.src = pageUrl(i)
  }
}

function onLoaded(p, e) {
  loaded[p] = true
  delete failed[p]
  const im = e?.target
  if (im && im.naturalWidth) natSize[p] = [im.naturalWidth, im.naturalHeight]
}
function onError(p) {
  failed[p] = true
  delete loaded[p]
  preloading[p] = false
}
function retry(p) {
  delete failed[p]
  preloading[p] = false
  const el = scroller.value?.querySelector(`.rd-item[data-page="${p}"] img`)
  if (el) el.src = pageUrl(p) + '?t=' + Date.now()
}

function flip(dir) {
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
  localStorage.setItem('ms-reader-dark', dark.value ? '1' : '0')
}

function onCommand(cmd) {
  if (cmd === 'theme') {
    toggleDark()
    return
  }
  fit.value = cmd
  applyFit()
}

function applyFit() {
  localStorage.setItem('ms-reader-fit', fit.value)
  nextTick(() => scrollToPage(page.value, false))
}

function switchChapter(next) {
  const n = Number(next)
  if (!n || n < 1 || (chapters.value.length && n > chapters.value.length) || n === order.value) return
  router.replace(`/reader/${kind.value}/${encodeURIComponent(comicId.value)}/${n}`)
}

function back() {
  saveProgress()
  if (window.history.length > 1) router.back()
  else router.replace('/favorites')
}

/* ---------- 点按手势：左/右翻页、中间切换工具栏 ---------- */
/* 原来用一层全屏透明点击区（.rd-zones），会吃掉图片区的点击与滚动；
   现在改成：触屏用 touchstart/touchend 判断「轻点 vs 拖动」，不拦截滚动。 */
let touchStart = null
let lastTouchAt = 0

function onTouchStart(e) {
  const t = e.touches && e.touches[0]
  if (!t) return
  touchStart = { x: t.clientX, y: t.clientY, at: Date.now() }
}

function onTouchEnd(e) {
  lastTouchAt = Date.now()
  if (!touchStart) return
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
  if (x < 0.3) flip(-1)
  else if (x > 0.7) flip(1)
  else toggleBar()
}

/** 宽屏鼠标：点图片中间区域切换工具栏（触屏刚处理过的合成 click 直接忽略） */
function onTap(e) {
  if (isMobile.value) return // 触屏已在 onTouchEnd 处理
  if (Date.now() - lastTouchAt < 600) return // 触屏设备上紧跟的合成 click，避免双动作
  const sc = scroller.value
  if (!sc) return
  const r = sc.getBoundingClientRect()
  const x = (e.clientX - r.left) / r.width
  if (x > 0.35 && x < 0.65) toggleBar()
}

function onKey(e) {
  if (e.target && ['INPUT', 'TEXTAREA'].includes(e.target.tagName)) return
  switch (e.key) {
    case 'ArrowRight':
    case 'PageDown':
    case ' ':
      flip(1)
      e.preventDefault()
      break
    case 'ArrowLeft':
    case 'PageUp':
      flip(-1)
      e.preventDefault()
      break
    case 'n':
      switchChapter(order.value + 1)
      break
    case 'p':
      switchChapter(order.value - 1)
      break
    case 'f':
      fit.value = fit.value === 'width' ? 'height' : 'width'
      applyFit()
      break
    case 'Escape':
      back()
      break
  }
}

watch(
  () => route.params.order,
  (v) => {
    const n = Number(v || 1)
    if (n && n !== order.value) {
      order.value = n
      load().then(() => {
        scroller.value?.scrollTo({ top: 0 })
        page.value = 1
      })
    }
  }
)

onMounted(() => {
  window.addEventListener('keydown', onKey)
  load().then(restorePosition)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  if (rafId) cancelAnimationFrame(rafId)
  saveProgress()
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
  padding: 8px 12px;
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
  overflow-x: hidden;
  -webkit-overflow-scrolling: touch;
  overscroll-behavior: contain;
  padding-top: 54px;
  transition: padding-top 0.2s ease;
}
.rd-scroll:not(.is-bar) {
  padding-top: 0;
}
.rd-scroll.rd-snap {
  /* proximity 比 mandatory 柔和：不会「吸」得太急，滑动更跟手 */
  scroll-snap-type: y proximity;
}
.rd-pages {
  max-width: 100%;
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
.rd-item img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
  opacity: 0;
  transition: opacity 0.18s ease;
}
.rd-item img.is-loaded {
  opacity: 1;
}
.rd-fit-width .rd-item {
  width: 100%;
}
.rd-fit-height .rd-item {
  height: 100vh;
  scroll-snap-align: start;
}
.rd-fit-height .rd-item img {
  object-fit: contain;
}
.rd-fit-original .rd-pages {
  width: max-content;
  margin: 0 auto;
}
.rd-fit-original .rd-item img {
  object-fit: contain;
}

/* 占位骨架 */
.rd-ph {
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
.rd-dark .rd-ph {
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
    padding: 6px 8px;
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
  .rd-scroll {
    padding-top: 50px;
  }
}
</style>
