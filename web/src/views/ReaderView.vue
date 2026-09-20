<template>
  <div class="rd" :class="{ 'rd-dark': dark }">
    <!-- 顶栏：点中间可隐藏，读图更沉浸 -->
    <div v-show="barVisible" class="rd-bar">
      <div class="rd-left">
        <el-button text class="rd-back" @click="back">
          <el-icon><ArrowLeft /></el-icon><span>返回</span>
        </el-button>
        <div class="rd-title">
          <div class="rd-name">{{ title }}</div>
          <div class="rd-sub">{{ meta.chapterTitle || ('第 ' + order + ' 章') }}</div>
        </div>
      </div>

      <div class="rd-right">
        <div class="rd-page">
          <template v-if="pages">{{ page }} / {{ pages }}</template>
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

        <el-radio-group v-model="fit" size="small" class="rd-fit" @change="applyFit">
          <el-radio-button label="width">适宽</el-radio-button>
          <el-radio-button label="height">适高</el-radio-button>
          <el-radio-button label="original">原始</el-radio-button>
        </el-radio-group>

        <el-button size="small" class="rd-theme" @click="toggleDark">
          <el-icon><component :is="dark ? 'Sunny' : 'Moon'" /></el-icon>
        </el-button>
      </div>
    </div>

    <!-- 图片区 -->
    <div ref="scroller" class="rd-scroll" :class="['rd-fit-' + fit, { 'rd-snap': fit === 'height' }]" @scroll.passive="onScroll">
      <div class="rd-pages" @click="onTap">
        <div v-for="p in pageList" :key="p" class="rd-item" :data-page="p">
          <img
            :src="pageUrl(p)"
            :alt="'第 ' + p + ' 页'"
            decoding="async"
            :loading="p <= 3 ? 'eager' : 'lazy'"
            @load="onLoaded(p)"
            @error="onError(p)"
          />
          <div v-if="failed[p]" class="rd-bad">
            第 {{ p }} 页加载失败
            <el-button text size="small" @click.stop="retry(p)">重试</el-button>
          </div>
        </div>
      </div>

      <div v-if="loading" class="rd-tip">正在加载…</div>
      <div v-else-if="loadError" class="rd-tip rd-tip-err">
        {{ loadError }}
        <el-button text size="small" @click="load">重试</el-button>
      </div>

      <div v-if="pages" class="rd-end">
        <el-button :disabled="order <= 1" @click="switchChapter(order - 1)">上一章</el-button>
        <span class="rd-end-tip">{{ meta.chapterTitle || '' }} 完</span>
        <el-button type="primary" :disabled="order >= chapters.length" @click="switchChapter(order + 1)">下一章</el-button>
      </div>
    </div>

    <!-- 手机端：左右点翻页 / 点中间出工具栏 -->
    <div v-if="isTouch" class="rd-zones">
      <div class="rd-zone" @click="flip(-1)"></div>
      <div class="rd-zone" @click="toggleBar"></div>
      <div class="rd-zone" @click="flip(1)"></div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '@/api'

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

const fit = ref(localStorage.getItem('ms-reader-fit') || 'width')
const dark = ref(localStorage.getItem('ms-reader-dark') === '1')
const barVisible = ref(true)
const page = ref(1)
const scroller = ref(null)
const isTouch = ref(false)

const chapLabel = (ch) => {
  const t = (ch.title || '').trim()
  return chapters.value.length > 1 ? `${ch.order}. ${t || '未命名'}` : (t || '第 1 章')
}

const pageUrl = (p) =>
  `/api/reader/${kind.value}/${encodeURIComponent(comicId.value)}/${order.value}/page/${p}`

const storeKey = computed(() => `ms-reader:${kind.value}:${comicId.value}`)

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
  loading.value = true
  loadError.value = ''
  Object.keys(failed).forEach((k) => delete failed[k])
  try {
    // 本子信息（章节列表）：失败也不阻塞阅读
    if (!chapters.value.length) {
      try {
        const c = await api.comic(kind.value, comicId.value)
        title.value = c.title || c.comicId
        chapters.value = (c.chapters || []).map((x) => ({ order: x.order, title: x.title }))
        if (!chapters.value.length) chapters.value = [{ order: 1, title: '' }]
      } catch (e) {
        title.value = comicId.value
        chapters.value = [{ order: 1, title: '' }]
      }
    }
    const m = await api.readerMeta(kind.value, comicId.value, order.value)
    meta.pages = m.pages || 0
    meta.chapterTitle = m.chapterTitle || ''
    meta.local = !!m.local
    if (!meta.pages) loadError.value = m.error || '这一章拿不到图片'
  } catch (e) {
    loadError.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

/** 恢复上次读到哪一页 */
function restorePosition() {
  const saved = readProgress()
  if (!saved || saved.order !== order.value || !saved.page || saved.page <= 1) return
  nextTick(() => {
    setTimeout(() => scrollToPage(saved.page, false), 260)
  })
}

function scrollToPage(p, smooth = true) {
  const el = scroller.value?.querySelector(`.rd-item[data-page="${p}"]`)
  if (!el) return
  scroller.value.scrollTo({ top: el.offsetTop - 2, behavior: smooth ? 'smooth' : 'auto' })
  page.value = p
}

/** 滚动时更新「当前页」并记录进度 */
let saveTimer = null
function onScroll() {
  const sc = scroller.value
  if (!sc) return
  const items = sc.querySelectorAll('.rd-item')
  const mid = sc.scrollTop + sc.clientHeight * 0.35
  let cur = 1
  for (const it of items) {
    if (it.offsetTop <= mid) cur = Number(it.dataset.page)
    else break
  }
  if (cur !== page.value) {
    page.value = cur
    clearTimeout(saveTimer)
    saveTimer = setTimeout(saveProgress, 600)
    preload(cur)
  }
}

/** 提前把后面几页塞进浏览器缓存，翻页更顺 */
function preload(cur) {
  for (let i = cur + 1; i <= Math.min(cur + 2, meta.pages); i++) {
    if (loaded[i] || failed[i]) continue
    const im = new Image()
    im.src = pageUrl(i)
  }
}

function onLoaded(p) {
  loaded[p] = true
  delete failed[p]
}
function onError(p) {
  failed[p] = true
  delete loaded[p]
}
function retry(p) {
  delete failed[p]
  const el = scroller.value?.querySelector(`.rd-item[data-page="${p}"] img`)
  if (el) el.src = pageUrl(p) + '?t=' + Date.now()
}

function flip(dir) {
  if (fit.value === 'height' || fit.value === 'width') {
    // 逐页滚动：适高模式一屏一页，适宽模式滚一屏
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

/** 手机：点屏幕左右两侧翻页 */
function onTap(e) {
  if (!isTouch.value) return
  const r = e.currentTarget.getBoundingClientRect()
  const x = (e.clientX - r.left) / r.width
  if (x < 0.3) flip(-1)
  else if (x > 0.7) flip(1)
  else toggleBar()
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
  isTouch.value = matchMedia('(hover: none)').matches || 'ontouchstart' in window
  window.addEventListener('keydown', onKey)
  load().then(restorePosition)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
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

/* ---------- 顶栏 ---------- */
.rd-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  background: rgba(255, 255, 255, 0.92);
  border-bottom: 1px solid #e5e7eb;
  backdrop-filter: blur(8px);
  flex-wrap: wrap;
}
.rd-dark .rd-bar {
  background: rgba(26, 29, 35, 0.92);
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
  max-width: 42vw;
}
.rd-sub {
  font-size: 12px;
  opacity: 0.62;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 42vw;
}
.rd-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.rd-page {
  font-variant-numeric: tabular-nums;
  font-size: 13px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  opacity: 0.85;
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
}
.rd-scroll.rd-snap {
  scroll-snap-type: y mandatory;
}
.rd-pages {
  max-width: 100%;
  margin: 0 auto;
}
.rd-item {
  position: relative;
  display: flex;
  justify-content: center;
  background: #fff;
  margin: 0 auto;
}
.rd-dark .rd-item {
  background: #1b1e24;
}
.rd-fit-width .rd-item img {
  width: 100%;
  height: auto;
  display: block;
}
.rd-fit-height .rd-item {
  height: 100vh;
  scroll-snap-align: start;
  align-items: center;
}
.rd-fit-height .rd-item img {
  max-height: 100vh;
  max-width: 100%;
  width: auto;
  object-fit: contain;
}
.rd-fit-original .rd-pages {
  width: max-content;
}
.rd-fit-original .rd-item img {
  display: block;
  max-width: none;
}
.rd-bad {
  position: absolute;
  inset: auto 0 12px 0;
  text-align: center;
  font-size: 13px;
  color: #b3352f;
  background: rgba(255, 255, 255, 0.9);
  padding: 8px;
  border-radius: 8px;
  width: max-content;
  margin: 0 auto;
}
.rd-tip {
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
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 28px 12px 40px;
  font-size: 13px;
  opacity: 0.9;
}
.rd-end-tip {
  opacity: 0.6;
}

/* ---------- 手机端点击区（左右翻页 / 中间出工具栏） ---------- */
.rd-zones {
  position: absolute;
  inset: 0;
  top: 56px;
  display: flex;
  z-index: 3;
}
.rd-zone {
  flex: 1;
}
.rd-zone:nth-child(2) {
  flex: 1.4;
}

/* ---------- 手机端布局 ---------- */
@media (max-width: 768px) {
  .rd-bar {
    padding: 6px 8px;
    gap: 6px;
  }
  .rd-name,
  .rd-sub {
    max-width: 40vw;
  }
  .rd-right {
    width: 100%;
    justify-content: space-between;
    gap: 6px;
  }
  .rd-chap {
    width: 130px;
  }
  .rd-fit {
    display: none;
  }
}
</style>
