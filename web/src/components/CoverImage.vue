<script setup>
import { computed, ref, watch } from 'vue'

/**
 * 封面图：src 指向契约里的 /api/comics/{kind}/{comicId}/cover
 * 加载失败（后端未起、图片缺失、被风控）时显示本地占位块，不发任何外网请求。
 */
const props = defineProps({
  kind: { type: String, default: '' },
  comicId: { type: [String, Number], default: '' },
  src: { type: String, default: '' },
  title: { type: String, default: '' },
  lazy: { type: Boolean, default: true }
})

const failed = ref(false)
const loading = ref(true)

const url = computed(() => {
  if (props.src) return props.src
  if (!props.kind || !props.comicId) return ''
  return `/api/comics/${props.kind}/${encodeURIComponent(props.comicId)}/cover`
})

watch(url, () => {
  failed.value = false
  loading.value = true
})

function onError() {
  failed.value = true
  loading.value = false
}

function onLoad() {
  loading.value = false
}
</script>

<template>
  <div class="cover">
    <img
      v-if="url && !failed"
      :src="url"
      :alt="title || '封面'"
      :loading="lazy ? 'lazy' : 'eager'"
      decoding="async"
      @error="onError"
      @load="onLoad"
    />
    <div v-else class="cover-fallback">
      <el-icon :size="26"><Picture /></el-icon>
      <span class="cover-fallback-text">{{ (title || '无封面').slice(0, 12) }}</span>
    </div>
    <div v-if="loading && url && !failed" class="cover-loading">
      <el-icon class="is-loading" :size="18"><Loading /></el-icon>
    </div>
  </div>
</template>

<style scoped>
.cover {
  position: relative;
  width: 100%;
  height: 100%;
  background: #101216;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.cover-fallback {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 100%;
  height: 100%;
  color: #5b6472;
  background: repeating-linear-gradient(
    45deg,
    #14171c,
    #14171c 8px,
    #171b21 8px,
    #171b21 16px
  );
  padding: 8px;
  text-align: center;
}

.cover-fallback-text {
  font-size: 11px;
  line-height: 1.2;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.cover-loading {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #6b7686;
  background: rgba(16, 18, 22, 0.45);
}
</style>
