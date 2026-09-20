<script setup>
/**
 * 统计卡片：label + 值 + 可选副标题/图标/可点击跳转
 */
defineProps({
  label: { type: String, default: '' },
  value: { type: [String, Number], default: '—' },
  sub: { type: String, default: '' },
  icon: { type: String, default: '' },
  color: { type: String, default: '' },
  clickable: { type: Boolean, default: false }
})

const emit = defineEmits(['click'])
</script>

<template>
  <div
    class="ms-stat"
    :class="{ clickable }"
    @click="clickable && emit('click')"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable ? 0 : undefined"
    @keyup.enter="clickable && emit('click')"
  >
    <div class="label">
      <el-icon v-if="icon" :size="14" :style="color ? { color } : {}">
        <component :is="icon" />
      </el-icon>
      <span>{{ label }}</span>
    </div>
    <div class="value" :style="color ? { color } : {}">{{ value }}</div>
    <div v-if="sub" class="sub ms-dim">{{ sub }}</div>
  </div>
</template>

<style scoped>
.sub {
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
