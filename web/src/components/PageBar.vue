<script setup>
import { computed } from 'vue'
import { useIsMobile } from '@/composables/useIsMobile'

/**
 * 统一分页条（契约分页参数 page / pageSize，page 从 1 开始）
 */
defineProps({
  total: { type: Number, default: 0 },
  page: { type: Number, default: 1 },
  pageSize: { type: Number, default: 20 },
  pageSizes: { type: Array, default: () => [20, 50, 100, 200] },
  disabled: { type: Boolean, default: false }
})

/* 手机端：去掉总数/每页条数/跳页，避免分页条换行溢出 */
const isMobile = useIsMobile()
const layout = computed(() =>
  isMobile.value ? 'prev, pager, next' : 'total, sizes, prev, pager, next, jumper'
)
const pagerCount = computed(() => (isMobile.value ? 5 : 7))

const emit = defineEmits(['update:page', 'update:pageSize', 'change'])

function onPage(p) {
  emit('update:page', p)
  emit('change', { page: p })
}

function onSize(s) {
  emit('update:pageSize', s)
  emit('update:page', 1)
  emit('change', { page: 1, pageSize: s })
}
</script>

<template>
  <div v-if="total > 0" class="ms-pager">
    <el-pagination
      background
      :layout="layout"
      :pager-count="pagerCount"
      :small="isMobile"
      :total="total"
      :current-page="page"
      :page-size="pageSize"
      :page-sizes="pageSizes"
      :disabled="disabled"
      @current-change="onPage"
      @size-change="onSize"
    />
  </div>
</template>
