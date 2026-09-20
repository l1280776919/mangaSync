/**
 * 按需注册 Element Plus 图标。
 *
 * 之前 main.js 用 `import * as Icons` 全量注册 296 个图标，打包后图标模块
 * 约 320KB（gzip 约 58KB），而模板实际只用到下面这些。
 *
 * ⚠️ 新增图标时**必须**在这里 import + 加进 ICONS：
 * 视图里的 `<el-icon><EditPen /></el-icon>` 以及 `:is="'Sunny'"` / `icon="Coin"`
 * 这类写法都是按名字解析全局组件的，没注册就渲染不出来（控制台会报
 * Failed to resolve component）。
 *
 * 清单来源：对 src/** 里所有字符串字面量（含 MENU / shortcuts 这类
 * `icon: 'Odometer'` 的配置数据）与所有大写组件标签做全量扫描，
 * 与 @element-plus/icons-vue 的导出名取交集，共 32 个。
 */
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  CircleCheck,
  CircleClose,
  Clock,
  Coin,
  Collection,
  DArrowLeft,
  DArrowRight,
  Document,
  Download,
  EditPen,
  Expand,
  Files,
  FolderOpened,
  Loading,
  Lock,
  Moon,
  Odometer,
  Picture,
  Refresh,
  Search,
  Select,
  Setting,
  Star,
  Sunny,
  SwitchButton,
  Tickets,
  User,
  UserFilled,
  WarningFilled
} from '@element-plus/icons-vue'

export const ICONS = {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  CircleCheck,
  CircleClose,
  Clock,
  Coin,
  Collection,
  DArrowLeft,
  DArrowRight,
  Document,
  Download,
  EditPen,
  Expand,
  Files,
  FolderOpened,
  Loading,
  Lock,
  Moon,
  Odometer,
  Picture,
  Refresh,
  Search,
  Select,
  Setting,
  Star,
  Sunny,
  SwitchButton,
  Tickets,
  User,
  UserFilled,
  WarningFilled
}

/** 挂到 app 上（名字 = 导出名，与全量注册时的名字一致） */
export function registerIcons(app) {
  for (const [name, comp] of Object.entries(ICONS)) {
    app.component(name, comp)
  }
  return app
}

export default registerIcons
