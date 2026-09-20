<script setup>
/**
 * 统计卡片：升级版带图标背景与柔光
 */
defineProps({
  label: { type: String, default: '' },
  value: { type: [String, Number], default: '—' },
  sub: { type: String, default: '' },
  icon: { type: String, default: '' },
  color: { type: String, default: '#3b82f6' },
  clickable: { type: Boolean, default: false }
})

const emit = defineEmits(['click'])
</script>

<template>
  <div
    class="ms-stat modern-stat"
    :class="{ clickable }"
    @click="clickable && emit('click')"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable ? 0 : undefined"
    @keyup.enter="clickable && emit('click')"
  >
    <div class="stat-top">
      <span class="label">{{ label }}</span>
      <div v-if="icon" class="icon-wrap" :style="{ backgroundColor: `${color}14`, color: color }">
        <el-icon :size="16">
          <component :is="icon" />
        </el-icon>
      </div>
    </div>
    <div class="value" :style="color ? { color } : {}">{{ value }}</div>
    <div v-if="sub" class="sub ms-dim" :title="sub">{{ sub }}</div>
  </div>
</template>

<style scoped>
.modern-stat {
  position: relative;
  overflow: hidden;
}

.stat-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.label {
  font-size: 13px;
  font-weight: 500;
  color: var(--ms-text-dim);
}

.icon-wrap {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.value {
  font-size: 24px;
  font-weight: 700;
  line-height: 1.15;
  letter-spacing: -0.5px;
  font-variant-numeric: tabular-nums;
  margin-top: 2px;
}

.sub {
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>