<script setup>
import { KIND_LABEL } from '@/utils/format'

const props = defineProps({
  kind: { type: String, default: '' },
  size: { type: String, default: 'small' }
})

const map = {
  pica: { label: '哔咔', type: 'primary' },
  jm: { label: '禁漫', type: 'danger' }
}

const meta = map[props.kind] || { label: KIND_LABEL[props.kind] || props.kind || '未知', type: 'info' }
</script>

<template>
  <el-tag :type="meta.type" :size="size" effect="light" class="kind-tag">{{ meta.label }}</el-tag>
</template>

<style scoped>
.kind-tag {
  font-weight: 600;
}
</style>
