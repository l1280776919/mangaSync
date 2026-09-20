<script setup>
import { computed } from 'vue'
import { ACCOUNT_STATUS, JOB_STATUS } from '@/utils/format'

/**
 * 通用状态标签。两种用法：
 * 1) 语义取值：<StatusTag domain="account" value="ok" /> / domain="job" value="running"
 * 2) 直接指定：<StatusTag label="已下载" type="success" />  （如收藏的下载状态）
 */
const props = defineProps({
  value: { type: String, default: '' },
  domain: { type: String, default: 'job' }, // account | job
  label: { type: String, default: '' },
  type: { type: String, default: '' },
  size: { type: String, default: 'small' },
  effect: { type: String, default: 'dark' }
})

const meta = computed(() => {
  if (props.label) return { label: props.label, type: props.type || 'info' }
  const table = props.domain === 'account' ? ACCOUNT_STATUS : JOB_STATUS
  return table[props.value] || { label: props.value || '未知', type: props.type || 'info' }
})
</script>

<template>
  <el-tag :type="meta.type" :size="size" :effect="effect">{{ meta.label }}</el-tag>
</template>
