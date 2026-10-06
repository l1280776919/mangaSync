<script setup>
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps({
  src: { type: String, required: true },
  page: { type: Number, required: true },
  failed: Boolean,
  priority: { type: String, default: 'auto' }
})
const emit = defineEmits(['load', 'error'])
const image = ref(null)
const decoded = ref(false)
let generation = 0
let alive = true
let checking = null

// Readiness belongs to this DOM image, never to a page that was loaded in the past.
async function showImage(event) {
  const node = event?.target || image.value
  if (!alive || !node || node !== image.value || !node.complete || !node.naturalWidth) return
  const token = generation
  const url = node.src
  if (checking === `${token}:${url}` || decoded.value) return
  checking = `${token}:${url}`
  try {
    if (typeof node.decode === 'function') await node.decode()
    if (!alive || token !== generation || node !== image.value || node.src !== url) return
    decoded.value = true
    emit('load', { target: node })
  } catch (_) {
    if (alive && token === generation && node === image.value && node.src === url) emit('error', { target: node })
  } finally {
    if (token === generation) checking = null
  }
}
function imageError(event) {
  if (!alive || event.target !== image.value) return
  decoded.value = false
  emit('error', event)
}
watch(() => props.src, async () => {
  const token = ++generation
  decoded.value = false
  checking = null
  await nextTick()
  // Cached images may already be complete before a listener observes load.
  if (alive && token === generation) showImage()
}, { immediate: true, flush: 'pre' })
onBeforeUnmount(() => { alive = false; generation++ })
</script>

<template>
  <div class="rd-image">
    <img ref="image" :src="src" :alt="'第 ' + page + ' 页'" loading="eager" decoding="async" :fetchpriority="priority" :class="{ 'is-loaded': decoded }" @load="showImage" @error="imageError" />
    <div v-if="!decoded && !failed" class="rd-ph" role="status" :aria-label="'正在加载第 ' + page + ' 页'"><span>{{ page }}</span></div>
  </div>
</template>

<style scoped>
.rd-image { position: absolute; inset: 0; }
</style>
