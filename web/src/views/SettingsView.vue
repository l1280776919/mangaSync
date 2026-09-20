<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api from '@/api'
import { useAppStore } from '@/store/app'
import StatCard from '@/components/StatCard.vue'
import { QUALITY_OPTIONS, formatBytes } from '@/utils/format'

const store = useAppStore()

const loading = ref(false)
const saving = ref(false)
const formRef = ref(null)
const health = ref(null)

/** 契约里 settings 的全部字段 */
const form = reactive({
  downloadRoot: '',
  picaDir: '',
  jmDir: '',
  picaProxy: '',
  jmProxy: '',
  jmVenv: '',
  jmBridge: '',
  concurrency: 2,
  imageWorkers: 8,
  quality: 'original',
  schedule: { enabled: false, time: '04:30' },
  serverPort: 8787
})

const rules = {
  downloadRoot: [{ required: true, message: '请填写下载根目录', trigger: 'blur' }],
  concurrency: [
    { required: true, message: '请填写并发数', trigger: 'blur' },
    {
      validator(rule, value, cb) {
        const n = Number(value)
        if (!Number.isInteger(n) || n < 1 || n > 8) cb(new Error('并发数取值范围 1 ～ 8'))
        else cb()
      },
      trigger: 'blur'
    }
  ],
  imageWorkers: [
    {
      validator(rule, value, cb) {
        const n = Number(value)
        if (!Number.isInteger(n) || n < 1 || n > 64) cb(new Error('图片并发数建议 1 ～ 64'))
        else cb()
      },
      trigger: 'blur'
    }
  ],
  scheduleTime: [
    {
      validator(rule, value, cb) {
        if (!form.schedule.enabled) return cb()
        if (!/^\d{2}:\d{2}$/.test(String(form.schedule.time || ''))) cb(new Error('时间格式应为 HH:mm'))
        else cb()
      },
      trigger: 'change'
    }
  ]
}

async function load() {
  loading.value = true
  try {
    const s = await api.getSettings()
    Object.assign(form, {
      downloadRoot: s?.downloadRoot ?? '',
      picaDir: s?.picaDir ?? '',
      jmDir: s?.jmDir ?? '',
      picaProxy: s?.picaProxy ?? '',
      jmProxy: s?.jmProxy ?? '',
      jmVenv: s?.jmVenv ?? '',
      jmBridge: s?.jmBridge ?? '',
      concurrency: s?.concurrency ?? 2,
      imageWorkers: s?.imageWorkers ?? 8,
      quality: s?.quality ?? 'original',
      schedule: {
        enabled: !!s?.schedule?.enabled,
        time: s?.schedule?.time || '04:30'
      },
      serverPort: s?.serverPort ?? 8787
    })
    store.settings = s
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    loading.value = false
  }
}

async function loadHealth() {
  try {
    health.value = await api.health()
  } catch (e) {
    health.value = null
  }
}

async function save() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    saving.value = true
    try {
      const payload = {
        downloadRoot: form.downloadRoot,
        picaDir: form.picaDir,
        jmDir: form.jmDir,
        picaProxy: form.picaProxy,
        jmProxy: form.jmProxy,
        jmVenv: form.jmVenv,
        jmBridge: form.jmBridge,
        concurrency: Number(form.concurrency),
        imageWorkers: Number(form.imageWorkers),
        quality: form.quality,
        schedule: { enabled: !!form.schedule.enabled, time: form.schedule.time },
        serverPort: Number(form.serverPort)
      }
      const merged = await api.saveSettings(payload)
      store.settings = merged
      Object.assign(form, { ...merged, schedule: { ...(merged?.schedule || form.schedule) } })
      ElMessage.success('设置已保存')
    } catch (e) {
      /* api.js 已提示 */
    } finally {
      saving.value = false
    }
  })
}

function reset() {
  load()
  ElMessage.info('已重新从后端读取设置')
}

onMounted(async () => {
  await Promise.allSettled([load(), loadHealth(), store.loadStats()])
})
</script>

<template>
  <div class="ms-panel">
    <div class="ms-panel-title">
      <span>
        系统设置
        <span class="ms-sub">· GET / PUT /api/settings</span>
      </span>
      <div class="head-actions">
        <el-tag v-if="health" type="success" size="small" effect="dark">
          后端正常 · v{{ health.version }} · 运行 {{ Math.floor((health.uptimeSec || 0) / 60) }} 分
        </el-tag>
        <el-tag v-else type="info" size="small" effect="dark">后端状态未知</el-tag>
        <el-button size="small" :loading="loading" @click="load">重新读取</el-button>
      </div>
    </div>

    <el-form
      ref="formRef"
      v-loading="loading"
      :model="form"
      :rules="rules"
      label-width="132px"
      label-position="right"
      class="setting-form"
    >
      <el-divider content-position="left">下载路径</el-divider>

      <el-form-item label="下载根目录" prop="downloadRoot">
        <el-input v-model="form.downloadRoot" placeholder="/data/comics">
          <template #prepend><el-icon><FolderOpened /></el-icon></template>
        </el-input>
        <div class="tip">必须是已存在或可创建的目录。</div>
      </el-form-item>

      <el-form-item label="哔咔子目录">
        <el-input v-model="form.picaDir" placeholder="PicaComic" />
        <div class="tip">相对下载根目录，最终路径：根目录 / 子目录 / 作品名</div>
      </el-form-item>

      <el-form-item label="禁漫子目录">
        <el-input v-model="form.jmDir" placeholder="18Comic" />
      </el-form-item>

      <el-divider content-position="left">网络与并发</el-divider>

      <el-form-item label="哔咔代理">
        <el-input v-model="form.picaProxy" placeholder="http://127.0.0.1:7890（留空表示直连）" clearable />
      </el-form-item>

      <el-form-item label="禁漫代理">
        <el-input v-model="form.jmProxy" placeholder="http://127.0.0.1:7890（留空表示直连）" clearable />
      </el-form-item>

      <el-form-item label="并发任务数" prop="concurrency">
        <el-input-number v-model="form.concurrency" :min="1" :max="8" :step="1" />
        <span class="tip-inline">同时下载的作品数（1 ～ 8）</span>
      </el-form-item>

      <el-form-item label="图片并发数" prop="imageWorkers">
        <el-input-number v-model="form.imageWorkers" :min="1" :max="64" :step="1" />
        <span class="tip-inline">单本内并行抓图的线程数</span>
      </el-form-item>

      <el-form-item label="图片质量">
        <el-radio-group v-model="form.quality">
          <el-radio-button v-for="q in QUALITY_OPTIONS" :key="q.value" :value="q.value">
            {{ q.label }}
          </el-radio-button>
        </el-radio-group>
        <div class="tip">只有哔咔（pica）支持质量切换。</div>
      </el-form-item>

      <el-divider content-position="left">禁漫运行时（Python）</el-divider>

      <el-form-item label="jm venv 路径">
        <el-input v-model="form.jmVenv" placeholder="python3" clearable />
        <div class="tip">调用 jmcomic 的 python 解释器路径。</div>
      </el-form-item>

      <el-form-item label="jm bridge 脚本">
        <el-input v-model="form.jmBridge" placeholder="/opt/mangasync/engines/jm_bridge.py" clearable />
      </el-form-item>

      <el-divider content-position="left">定时同步</el-divider>

      <el-form-item label="启用定时同步">
        <el-switch v-model="form.schedule.enabled" active-text="开启" inactive-text="关闭" />
        <div class="tip">开启后会按设定时间自动同步所有账号的收藏。</div>
      </el-form-item>

      <el-form-item label="同步时间" prop="scheduleTime">
        <el-time-picker
          v-model="form.schedule.time"
          format="HH:mm"
          value-format="HH:mm"
          placeholder="04:30"
          :disabled="!form.schedule.enabled"
          style="width: 160px"
        />
      </el-form-item>

      <el-divider content-position="left">服务</el-divider>

      <el-form-item label="服务端口">
        <el-input-number v-model="form.serverPort" :min="1" :max="65535" :step="1" />
        <span class="tip-inline">默认 8787（需重启后端生效）</span>
      </el-form-item>

      <el-form-item>
        <el-button type="primary" :loading="saving" @click="save">保存设置</el-button>
        <el-button :disabled="loading" @click="reset">放弃修改</el-button>
      </el-form-item>
    </el-form>
  </div>

  <div class="ms-panel">
    <div class="ms-panel-title">磁盘与缓存</div>
    <div class="ms-stat-grid">
      <StatCard
        label="漫画库占用"
        :value="formatBytes(store.stats?.library?.bytes)"
        icon="FolderOpened"
        :sub="`${store.stats?.library?.comics ?? 0} 部作品`"
      />
      <StatCard
        label="剩余空间"
        :value="formatBytes(store.stats?.disk?.freeBytes)"
        icon="Coin"
        :sub="store.stats?.disk?.totalBytes ? `总容量 ${formatBytes(store.stats.disk.totalBytes)}` : '—'"
      />
      <StatCard
        label="封面缓存"
        value="后端自管"
        icon="Picture"
        sub="/var/lib/mangasync/cache/covers/"
      />
    </div>
  </div>
</template>

<style scoped>
.setting-form {
  max-width: 720px;
}

.tip {
  font-size: 12px;
  color: var(--ms-text-dim);
  line-height: 1.5;
  width: 100%;
}

.tip-inline {
  font-size: 12px;
  color: var(--ms-text-dim);
  margin-left: 10px;
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

:deep(.el-form-item__content) {
  flex-wrap: wrap;
}
</style>
