# RemLink 控制台设计规范（new_ui）

视觉**风格**沿用黑白单色体系：主色是黑色，彩色只用来表达语义；字体为 vfonts Lato。
**排版和布局是 RemLink 自己的**，不得照搬 AxisNow。下列做法禁止使用：

- 74px 图标栏 + 182px 二级栏
- 页面标题上下 50px 留白
- 16px inset-ring 大框里放 5 列 KPI
- 灰底卡片行列表 + hover 黑边
- 7 列资源计数网格
- 居中 KPI 磁贴
- 44px 大按钮 + 分段 tab 放在标题右侧

## 产品化原则（重要：不要照搬旧版页面结构）

参考 Tailscale、NetBird、Cloudflare Zero Trust 这类现代企业接入控制台。旧版页面只用来确认**功能和接口**，信息架构和交互一律重新设计：

1. **一个页面只做一件事**。旧版「一页多 Tab」的大杂烩页面已经拆进侧边栏的二级菜单，每个子菜单是一个独立路由，**页面内不要再用 Tab 切换大分区**。
2. **对象型资源走「列表 → 详情页」**：认证源、证书这类对象，列表点进去是独立的详情路由（`/xxx/:id`），详情页分段展示「概览 / 配置 / 操作 / 危险区」。只有简单对象（几个字段）才用抽屉编辑。
3. **集成类对象用卡片画廊**，比如认证源：每种类型一张卡（名称、说明、状态「已连接 N 个 / 未配置」），点击后新建或进入详情。
4. **状态优先**：页面顶部先回答「现在是否正常」，例如证书剩余天数、ACME 自动续期状态、邮件通道是否可用。用状态条或摘要卡，再往下才是配置。
5. **向导代替长表单**：首次配置（ACME 签发、CA 初始化、认证源接入）用 `NSteps` 分步，每步只问必要的问题。
6. **危险操作单独放**：在页面底部放一个 `RlPanel` 危险区（标题用 `text-error`），比如删除、重置 CA、关闭加密，每个操作写清楚影响。
7. **审计/日志是事件流**：顶部是时间范围加筛选 chip；行可以展开看详情，或在右侧抽屉看详情；支持导出。
8. 文案要面向管理员的任务（「接入一个 LDAP 目录」），不要直接用字段名。

## 外壳（已完成，勿改）

- **侧栏**：由 `layout/AppLayout.vue`、`layout/nav.ts` 和 `components/SideNav.vue` 组成。单栏两级菜单，宽 240px，可折叠到 64px。一级是模块，可展开；二级是页面。折叠后鼠标悬停会弹出二级菜单。
- 不显示默认 logo，只显示品牌名文字。管理员自定义了 logo 时才显示图片。
- **顶栏**：高 56px，左边折叠按钮 + 面包屑，右边主题切换。
- **内容区**：`px-lg`，最大宽度 1600px。

## 页面骨架

```vue
<PageHeader title="用户" description="一句话说明这个页面管什么">
  <NButton>次要操作</NButton>
  <NButton type="primary">新建用户</NButton>   <!-- 每页最多一个 primary -->
  <template #tabs><UnderlineTabs v-model="tab" :options="..." /></template>  <!-- 可选 -->
</PageHeader>
<div class="grid gap-16px"> ... 若干 RlPanel ... </div>
```

- 按钮使用默认 medium 尺寸（36px），**不要用 size="large"**。
- 栅格间距统一用 16px（`gap-16px`）。

## 共享组件（`src/components/`，只用不改；确有需要就在自己目录下新建）

| 组件 | 用途 |
|---|---|
| `PageHeader` | 页头。props：`title`、`description`；默认插槽放操作按钮；`#meta` 插槽放标题旁的徽标或数量；`#tabs` 插槽放下划线 tab |
| `RlPanel` | 面板，1px 边框、12px 圆角。props：`title`、`subtitle`、`padded`（默认 true）。插槽：`#extra` 是标题栏右侧，`#toolbar` 是标题下的工具栏（搜索、筛选），`#footer` 放分页或汇总 |
| `StatCard` | 指标卡。props：`label`、`value`、`unit`、`hint`、`icon`、`to`、`tone`；默认插槽放迷你图，`#delta` 放环比 |
| `SettingRow` | 设置行，左边是标题和说明，右边是控件。放在 `<RlPanel :padded="false">` 里，多行之间自动加分隔线 |
| `StatusDot` | 状态点：6px 圆点 + 文字。tone 可选 success、warning、error、info、muted；`pulse` 让圆点呼吸 |
| `CountryFlag` | 国旗（flag-icons）。`code` 用 ISO 3166-1 alpha-2；传 `LAN` 显示「内网」占位；`with-name` 同时显示中文国名 |
| `EmptyState` | 空状态 |
| `UnderlineTabs` | 下划线 tab，v-model，options 结构为 `{label, value, count?}` |
| `AxLineChart` / `AxDonut` / `AxMap` / `AxSankey` | 图表。迷你图统一用 info 蓝单色折线 |

## 列表页模式（统一做法）

```vue
<RlPanel :padded="false">
  <template #toolbar>
    <NInput v-model:value="kw" placeholder="搜索…" clearable class="w-240px!"><template #prefix><Search/></template></NInput>
    <NSelect ... class="w-160px!" />   <!-- 筛选 -->
    <div class="flex-1" />
    <span class="text-sm text-secondary">共 {{ total }} 条</span>
    <button class="rl-icon-btn" @click="load"><ArrowClockwise/></button>
  </template>
  <NDataTable :columns :data :loading :bordered="false" :row-key size="small" ... />
  <template #footer><span/> <NPagination .../></template>
</RlPanel>
```

- 表格不要外边框；主信息放在第一列，粗体，副信息换行写成 12px secondary。
- 行操作放在最后一列，用 text 按钮（编辑、删除），或用 `…` 下拉菜单。
- 状态列统一用 `StatusDot`。IP、MAC、端口用 `rl-mono`。
- 批量操作：勾选行后，在工具栏左侧显示「已选 N 项」和批量按钮。

## 新建 / 编辑

- 统一用**右侧抽屉** `NDrawer`（宽 560–720px），内容放在 `NDrawerContent`，底部是「取消 / 保存」，保存按钮为 primary。
- 表单较长时用 `NTabs type="line"` 分段，或用 18px 标题加 `NDivider` 分段。
- 删除用 `confirm()`（`@/utils/ui`），删除按钮 `type="error"`，并用 `secondary` 或 text 形态。

## 设置页模式

- 设置已按主题拆成侧边栏子页，一页只管一个主题，不再用 Tab 切换分区。
- 每个分区一个 `RlPanel title="分区名" :padded="false"`，里面是若干 `SettingRow`。
- 页面底部放一条吸底的保存条：`sticky bottom-0`，`bg-body`，上边框；内容为「有未保存修改」提示、「重置」「保存」。只有表单变脏时才高亮。

## Token 与工具类

- 颜色只用 CSS 变量或 Uno 映射，**禁止写死十六进制**。
  - 文字：`text-primary`、`text-secondary`、`text-tertiary`、`text-quaternary`、`text-success`、`text-warning`、`text-error`、`text-info`
  - 背景：`bg-body`、`bg-primary`、`bg-secondary`、`bg-fill`、`bg-action-hover`
  - 边框：`b-[var(--color-border-default)]`
- 字号：`text-sm` 12、`text-md` 14、`text-lg` 16、`text-xl` 18、`text-4xl` 24。
- 间距：xs 6、sm 10、md 20、lg 30，也可以直接写 px，比如 `gap-16px`。
- `ax-badge` 是 currentColor 14% 铺底的小徽标，可配合 `text-info` 等使用。`rl-kbd` 用于小代码块，`rl-icon-btn` 是 32px 纯图标按钮。
- 图标：`~icons/fluent/xxx-20-regular`（unplugin-icons，按需编译），可以在 https://icon-sets.iconify.design/fluent/ 查找名称。

## 数据层

- 请求一律用 `@/api/http` 的 `get`、`post`、`postForm`，`request` 返回已解包的 `data`，`code≠0` 时抛 `ApiError`。错误提示用 `ui.message.error(errMsg(e))`。
- 所有后端 JSON 接口统一在 `/api/v1/*` 命名空间下，前端调用时写完整路径（`get('/api/v1/user/list')`），不使用 axios baseURL：
  - 管理后台（8800）：`/api/v1/base/*`、`/api/v1/set/*`（含 WebSocket `/api/v1/set/syslog/ws`）、`/api/v1/user/*`、`/api/v1/group/*`、`/api/v1/policy/*`、`/api/v1/provider/*`、`/api/v1/statsinfo/*`、`/api/v1/locksinfo/*`、`/api/v1/webvpn/*`（应用/审计管理）、`/api/v1/portal/login-config`（免鉴权品牌配置）。
  - 用户侧（VPN 端口）：门户 `/api/v1/portal/*`、Web 认证 `/api/v1/web-auth/*`。门户与 Web 认证接口走裸 `axios`（见 `pages/portal/shared.ts` 的 `portalApi`），不经过后台 401 拦截。
  - 例外（不在 `/api/v1` 下，不要改）：WebVPN 应用子域名上的 `/webvpn/login-config`、`/webvpn/me`、`/webvpn/my-apps`、`/webvpn/logout`（放到 `/api/v1` 会劫持被代理内网应用自己的 `/api/v1`）；工具页 `/debug/*`；第三方/客户端回调 `/web-auth/sso-callback`、`/WXAuth|FeishuAuth|DingtalkAuth/callback`、`/+CSCOE+/*`。
  - 前端路由（`/admin/...`、`/portal/...`、`/web-auth/continue`）是页面路径，与接口前缀无关。
  - 开发时 `vite.config.ts` 代理 `/api` 与 `/debug` 到 `VITE_API_TARGET`。
- 后端字段**以旧前端 `web/src/pages/**` 为准，逐个对照**，不能少功能（接口路径按上面的 `/api/v1` 前缀）。后端 Go 代码在 `server/admin/`（后台）、`server/handler/`（门户/Web 认证），路由注册见各自 `server.go`。
- 时间格式化用 `formatDate`，字节数用 `formatBytes`（`@/utils/format`）；网络工具在 `@/utils/net`。
- 代码使用 Vue 3 `<script setup lang="ts">`，注释写中文，保持简洁。
