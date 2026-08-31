# 认证模块开发者指南

## 架构概览

```
用户请求
    │
    ▼
handler/link_auth*.go          ← 入口：解析 XML/JSON → 构造 *Flow（注入 OnPass/OnChallenge/OnFail 回调）
    │
    ▼
handler/authflow.go            ← AuthFlow 状态机（三端共用收口）：
    │                             调 authsrv.Authenticate/Resume → 统一锁定计数 → 按终态分发到回调
    ▼
auth/authsrv/authsrv.go        ← 编排层（与认证器同包，直接查库）：
    │                             加载组 → 解析 Profile → GetPipeline → Run/Resume
    ▼
auth/pipeline.go               ← GetPipeline(profile, resolver)：工厂实例化 + Provider 配置注入
    │                             Pipeline.Run(ctx) / Pipeline.Resume(ctx, from)：按序执行每个 Step
    ▼
auth/authsrv/*.go              ← 各认证器实现 Authenticator 接口（import dbdata 取数）
    │
    ▼
StepPass / StepPending / StepFail
    │         │              │
    ▼         ▼              ▼
 继续下一步  通知客户端       终止管道
            输入额外信息     返回错误
```

> **AuthFlow 收口（2026-08-11 落地）**：此前门户 / WebAuth / 原生 XML 三端各自写一套「跑管道 → `switch result.Result` 分发 → 统一锁计数 → `savePendingState`」，三份同构代码。现统一收口到 `handler/authflow.go` 的 `Flow`：
> - `Flow.Run(w, r)`：首次认证，内部调 `authsrv.Authenticate` 后分发。
> - `Flow.Resume(w, r, state)`：从挂起断点恢复，内部调 `authsrv.Resume` 后分发。
> - `Flow.Dispatch(w, r, result)`：**仅分发**一个已计算好的 `result`，**不重跑管道**——原生 XML 端点（`link_auth_pipeline.go`）在早已手动拿到 `result` 处使用，避免重复执行（曾因误用 Run/Resume 重跑导致 OTP 续验测试失败）。
>
> 三端只需在各自构造的 `Flow` 上提供 `OnPass`/`OnChallenge`/`OnFail` 三个回调，各自定义"通过动作"（建 VPN 会话 / 签门户 JWT / WebVPN 免登）与"挑战渲染"（XML / JSON）。**新增认证方法无需改 AuthFlow**：只在 `auth` 注册表登记新 step，管道自动编排，Flow 的分发与锁定逻辑零改动。
>
> **新增 SSO/OAuth2 类型（如钉钉）三端渲染出口零改动**：`BuildChallengeView` 透传 `sso_type`，WebAuth / 原生 XML / 门户三端（含 `PortalSSO` 入口校验）均自动生效，只需在 `handler/sso.go` 的 `ssoProviders` 注册一处（详见「范式 B」）。

### 包边界与依赖方向

依赖是**双向**的，但无环，关键在于「`auth`」下分两层角色：

| 包                                                                                              | 角色                                                                  | 依赖 dbdata？               |
| ----------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- | --------------------------- |
| `auth`（核心：`auth.go`/`pipeline.go`/`registry.go`/`config*.go`/`lockmanager.go`） | 契约 / 数据模型 / 管道引擎（叶子，仅依赖标准库 + 第三方库如 go-ldap） | **否**                |
| `auth/authsrv`                                                                                | 认证实现（认证器 + 管道编排，查库执行）                               | **是**                |
| `dbdata`                                                                                      | 数据层（引用`auth` 的类型与纯函数）                                 | 反向依赖核心`auth` 的类型 |

- 核心 `auth` 不 import `dbdata`/`handler`，是稳定的契约叶子。
- `auth/authsrv` 在核心 `auth` **之上**，直接 import `dbdata` 取数（认证器与管道编排都在本包），顺流而下不成环。
- `dbdata` 引用的是**核心 `auth` 的类型与纯函数**（如 `UserInfo`、`GetPipeline`、`ParseAuthProfile`、`IsRegistered`），不是在依赖认证逻辑，而是在"说 auth 定义的语言"（见 `dbdata/user.go` 的 `ToAuthInfo` 投影、`dbdata/group.go` 的管道校验；用户信息加载见 `authsrv.LoadUserInfo`）。

### 核心 auth 包文件职责

| 文件                                                                                                | 职责                                                                                                                                                                                                                                                |
| --------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `auth.go`                                                                                         | 全部数据契约：`StepResult`/`ConnInfo`/`UserInfo`/`Context`（含各步骤私有状态 `OTPState`/`SMSState`/`RADIUSState`/`SSOState`）/`Authenticator`/`Challenger`/`ChallengeInfo`/`GroupInfo`/`PipelineState`/`PipelineResult` |
| `pipeline.go`                                                                                     | 管道引擎：`Pipeline` 类型、`GetPipeline`、`Run`/`Resume`/`runFrom`、`ProviderResolverFunc`                                                                                                                                              |
| `registry.go`                                                                                     | 认证器工厂注册表：`Register`/`GetFactory`/`IsRegistered`/`RegisteredNames`/`IsSSOType`                                                                                                                                                    |
| `config.go`                                                                                       | `GroupAuthProfile`/`AuthMethodConfig`、`ParseAuthProfile`、`GetProviderConfigFromMap`、Profile 合法性校验                                                                                                                                   |
| `config_ldap.go`/`config_radius.go`/`config_wxwork.go`/`config_feishu.go`/`config_dingtalk.go`/`config_sso.go` | 各第三方认证的配置结构体（实现`ProviderConfig` 接口，供 dbdata 引用）                                                                                                                                                                             |
| `lockmanager.go`                                                                                  | 防暴力破解三级锁定（全局单例）                                                                                                                                                                                                                      |

---

## 核心接口

### Authenticator（所有认证器必须实现）

```go
// auth/auth.go
type Authenticator interface {
    Name() string                                     // 认证器名称，如 "local", "ldap"
    Authenticate(ctx *Context) (StepResult, error)    // 执行认证
}
```

### StepResult（认证结果枚举）

| 值              | 含义                                                            | 管道行为                             |
| --------------- | --------------------------------------------------------------- | ------------------------------------ |
| `StepPass`    | 认证通过                                                        | 继续执行下一个 Step                  |
| `StepPending` | 需要客户端额外输入（如 OTP 码、SSO 重定向）；或用户尚未提供凭据 | 暂停管道，等待客户端提交后`Resume` |
| `StepFail`    | 认证失败                                                        | 立即终止管道                         |

### Context（贯穿管道的上下文，分组类型化）

```go
// auth/auth.go
type Context struct {
    Conn     ConnInfo   // 连接信息（Username/Password/GroupName/RemoteAddr/UserAgent/MacAddr/DeviceID/DeviceType/PlatformVer/TLS）
    UserInfo *UserInfo  // 用户信息（dbdata.User 投影，不含 PinCode；nil=未加载）
    Identity string     // 身份断言（证书 CN 等），管道末尾一致性检查

    OTP    *OTPState    // otp 步骤私有状态（Code/Sent）
    SMS    *SMSState    // sms 步骤私有状态（Phone/Code/Sent）
    RADIUS *RADIUSState // radius 步骤私有状态（State/ChallengeCode/ChallengeMsg）
    SSO    *SSOState    // SSO 步骤私有状态（Type/From/Authenticated/UserID/Code/...）

    PortalLogin     bool // Portal 登录流程
    PortalOTPDirect bool // Portal OTP 直连模式

    // 未导出：info（日志描述）/ passedSteps（已通过步骤）/ stepIdx（Resume 断点）
}
```

> **设计要点**：原 `ctx.Extra map[string]any` 硬编码字符串键（如 `otp_code`、`{ssoType}_authenticated`）已废弃，改为各步骤私有状态的具名字段，经 `GetOTP()`/`GetSMS()`/`GetRADIUS()`/`GetSSO()` 懒初始化（字段为 nil 时自动创建）。编译期类型安全，杜绝了 `otp_code` 与 `radius ChallengeCode` 共享总线碰撞。

> **用户信息单一来源**：`ctx.UserInfo` 是管道内共享的用户子集。投影**全仓只有一处**：`dbdata.User.ToAuthInfo()`（须写在 dbdata，因核心 auth 包不能反向 import dbdata）。查库+投影的内部原语是 `authsrv.loadUser(username) (*auth.UserInfo, error)`；`authsrv.LoadUserInfo(ctx)` 是其 ctx 包装（带去重守卫，查库失败静默忽略，全管道只查一次）。步骤需要用户信息时，`ctx.UserInfo` 为 nil 则调 `authsrv.LoadUserInfo(ctx)` 兜底加载。`local`/`cert` 因需完整 `*dbdata.User`（含密码哈希 `PinCode`，被刻意排除在 `UserInfo` 之外）自行 `dbdata.One` 查全实体，查完 `SetUserInfo` 供下游复用。

### Challenger（需要客户端交互的认证器额外实现）

```go
type Challenger interface {
    Authenticator
    Challenge() *ChallengeInfo   // 返回挑战信息（OTP 输入、SSO 重定向等）
}
```

内置 `NopChallenger` 可嵌入到无需挑战的认证器中（返回 `nil`）。

### ChallengeType（挑战类型枚举）

| 常量                | 值           | 说明                             |
| ------------------- | ------------ | -------------------------------- |
| `ChallengeOTP`    | `"otp"`    | OTP 动态码输入                   |
| `ChallengeSSO`    | `"sso"`    | SSO 重定向（企微/飞书 OAuth2）   |
| `ChallengeRADIUS` | `"radius"` | RADIUS Access-Challenge 二次验证 |
| `ChallengeSMS`    | `"sms"`    | 短信验证码输入                   |

### PipelineResult（管道执行结果）

```go
type PipelineResult struct {
    Result      StepResult
    Err         error
    Username    string
    GroupName   string
    Info        string
    LimitTime   *time.Time // 用户过期时间，由认证器加载到 ctx.UserInfo 后透传
    Challenge   *ChallengeInfo
    State       PipelineState
    PrevStepIdx int // 执行前的管道步号，首次认证为 -1
}
```

`IsChallengeRetry()` 判断挑战是否原地踏步（`StepPending && State.StepIdx == PrevStepIdx`，即挑战码错误未推进到下一步），用于防爆计数。

---

## Profile 配置与合法性约束

组认证配置存于 `group.auth_profile`（JSON）：

```json
{ "step": [ {"type": "ldap", "provider": "上海LDAP"}, {"type": "otp"} ] }
```

`ParseAuthProfile`（`config.go`）解析时强制以下约束，违反直接报错：

1. **step 不能为空**。
2. **第一步不能是 `otp`**：OTP 依赖前置步骤提供用户名。
3. **`radius` 与 `otp` 不能同时配置**：RADIUS 服务端自带 TOTP 时会与本地 OTP 冲突。

`AuthMethodConfig` 仅两个字段：`type`（认证器名）和 `provider`（可选，引用命名 Provider）。`provider` 优先于内联配置；`local`/`cert`/`otp` 不支持 provider。

---

## 认证组合能力边界

管道是**顺序 AND 语义**：`profile.Step` 按序执行，每步独立读 `ctx.Conn.Password`，**所有步骤都 `StepPass` 才算通过**。这决定了组合的可行范围。

各认证器对密码的依赖：

| 认证器                | 缺密码时行为                                                             | 是否产生密码 |
| --------------------- | ------------------------------------------------------------------------ | ------------ |
| `local`             | `len(Password) < 6` → `StepFail`                                    | —           |
| `ldap`              | `Username==""` 或 `len(Password) < 1` → `StepPending`（等待输入） | —           |
| `radius`            | 缺凭据 →`StepPending`                                                 | —           |
| `cert`              | 只设`ctx.Identity` + `SetUserInfo`，**不产生密码**             | 否           |
| `wxwork`/`feishu`/`dingtalk` | 只设`ctx.Conn.Username = sso.UserID`，**不产生密码**             | 否           |

### 可行组合

- 单步：`[local]` `[ldap]` `[radius]` `[cert]`
- 凭据 + OTP：`[local,otp]` `[ldap,otp]` `[cert,otp]`（注意 `radius` 不能与 `otp` 同配）
- SSO + OTP：`[wxwork,otp]` `[feishu,otp]` `[dingtalk,otp]`

### 不可行组合

- **SSO/cert + 任何要密码的凭据步骤**：`[wxwork,local]` `[cert,local]` `[wxwork,ldap]` `[wxwork,radius]` 等。SSO/cert 不产生密码，后置的 `local` 会 `StepFail`、`ldap`/`radius` 会 `StepPending` 卡死。此类为重复认证，无真实需求。
- **OR 语义**（"企微 **或** 本地，二选一"）：管道纯 AND，无此语义。"任选认证方式"的需求由**按 Group 配不同认证**满足（一个组配 `[wxwork,otp]`，另一个配 `[local,otp]`）。

### `[SSO, otp]` 为何可用（关键机制）

otp 步用 `ctx.Conn.Username`（= SSO `UserID`）查本地用户，查不到会 `StepFail`。之所以能查到，是因为**组保存时自动同步 IdP 用户到本地**：

```
GroupSet（admin/api_group.go）
    │  组配了 otp + ldap/wxwork/feishu 任一
    ▼
dbdata.SyncExternalUsersForOTP(g)（dbdata/group.go）
    │  异步 SaveUsers(g)
    ▼
把所有 IdP 用户写入本地 DB 并生成 OTP 密钥
```

本地用户名即用 SSO `UserID` 创建，故 `[SSO, otp]` 的 otp 步必能查到用户，不存在"SSO 身份 ≠ 本地用户名"的问题。

> 边角：IdP 中在组保存**之后**新增的用户，需等下次同步（飞书有 cron `SyncFeishuUsers`、钉钉有 `SyncDingtalkUsers`；企微/LDAP 需重新保存组触发）后才能登录，属运维范畴。

---

## 如何新增一个认证方法

新增认证器分两类，复杂度差异很大：

- **普通型（凭据 / 无交互）**：如 `ldap`/`radius`/简单密码类/换一家第三方凭据服务。只需下面 3 步，自注册即生效。
- **SSO/OAuth2 型**：如 `wxwork`/`feishu`/钉钉。除认证器本体 3 步外，还需在 handler 层加回调端点、dbdata 层加配置读取与用户同步、前端加入口（见范式 B）。文件多，但多与现有 `wxwork`/`feishu` 对称，属"复制改名字"的机械活。

### 范式 A：普通型（无交互 / 凭据类）

以新增 `example` 认证器（第三方凭据类，如另一家 LDAP）为例：

#### 第 1 步：定义配置类型 `auth/config_example.go`（如需外部配置）

```go
// auth/config_example.go
package auth

type ExampleConfig struct {
    Endpoint string `json:"endpoint"`
    Secret   string `json:"secret"`
}

// ValidateConfig 实现 ProviderConfig 接口
func (c *ExampleConfig) ValidateConfig() error { /* ... */ return nil }
```

> 配置类型放在核心 `auth` 包（文件名遵循 `config_<type>.go` 约定），使 `dbdata` 能引用（依赖倒置）。现有配置：`config_ldap.go`/`config_radius.go`/`config_wxwork.go`/`config_feishu.go`/`config_sso.go`。

#### 第 2 步：实现认证器 `auth/authsrv/example.go`

```go
// auth/authsrv/example.go
package authsrv

import "github.com/wsczx/remlink/auth"

func init() {
    auth.Register("example", func() auth.Authenticator {
        return &ExampleAuth{}
    })
}

type ExampleAuth struct {
    auth.NopChallenger    // 无交互挑战时嵌入；需交互则实现 Challenge()
    auth.ExampleConfig    // 嵌入配置结构体，Provider 配置自动注入
}

func (a *ExampleAuth) Name() string { return "example" }

func (a *ExampleAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
    // 校验逻辑 → StepPass / StepPending / StepFail
}
```

> 此处 `example` 为无交互的凭据类认证器，故嵌入 `NopChallenger`。**若新增的是 SSO，则不能嵌入 `NopChallenger`，必须实现 `Challenger`**（见范式 B）。

#### 第 3 步（可选）：Provider 配置注入

认证器结构体的 `json` tag 字段由 `GetPipeline` 通过 `resolver` 自动填充。管理后台创建 Provider 后，组配置引用即可：

```json
{ "step": [ {"type": "local"}, {"type": "example", "provider": "example_prod"}, {"type": "otp"} ] }
```

`GetPipeline` 检测到 `cfg.Provider != ""` 时调用 `resolver("example_prod", "example")`（即 `dbdata.ResolveProviderConfig`）获取配置 map，再经 `GetProviderConfigFromMap` 反序列化到认证器实例。

> `example` 此处为第三方凭据类（**非 SSO**），可出现在管道中间（如 `[local, example, otp]`）。SSO 类型不能和 `local` 同配（见「认证组合能力边界」），只能配 `[sso, otp]`。

#### 关键模式总结

| 模式 | 说明 |
|---|---|
| `init() + Register` | 自注册，新增文件即生效 |
| 嵌入配置结构体 + `json` tag | Provider 配置自动注入 |
| 嵌入 `NopChallenger` | 无交互的认证器（local/ldap/radius/cert/admin） |
| 实现 `Challenger` 接口 | 需要客户端交互的认证器（otp/sms/wxwork/feishu） |
| 分组类型化 `Context` 字段 | 同一 `Authenticate` 方法处理首次进入 + 回调恢复，状态存具名字段 |
| `ctx.SetInfo()` | 记录认证日志，最终写入审计 |
| `StepPending` → `Resume` | 暂停管道，等客户端提交后从断点继续 |

### 范式 B：SSO/OAuth2 型（如钉钉）

SSO 认证器必须**实现** `Challenger` 接口（`Challenge().Type == ChallengeSSO`），否则 `IsSSOType()` 返回 false、管道不识别为 SSO。认证器本体仍是 3 步，但第 2 步改为**实现 `Challenger`**（而非嵌入 `NopChallenger`）。

#### 认证器本体（3 步）

- **第 1 步**：定义 `auth/config_dingtalk.go`（`DingtalkConfig`，含 `UseDefaultBrowser`/`AllowedDepartments`/`BlockedUserIDs` 等，实现 `ProviderConfig`；API 调用如 `GetDingtalkUser` 也写在此文件，因核心 `auth` 包才能被 `dbdata` 引用）。`BlockedUserIDs` 走 `ParseBlockedUserIDs` + `CheckUserID` 在 `Authenticate` 与回调阶段双重校验。
- **第 2 步**：实现 `auth/authsrv/dingtalk.go`，`init()+Register("dingtalk", ...)`，**实现 `Challenger`**：`Challenge()` 返回 `ChallengeSSO` 且 `Data` 带 `sso_type:"dingtalk"`；`Authenticate` 处理三条路径——①回调已完成（`ctx.SSO.Authenticated` 直接放行）、②管道内用 OAuth `code` 换用户并校验部门/拒绝名单、③首次进入返回 `StepPending` 触发挑战。
- **第 3 步**：Provider 注入同范式 A（配置经 `dbdata.ResolveProviderConfig` 注入）。

#### 还必须改的（让 OAuth2 回调真正落地）

| # | 文件 | 改动 | 重构后是否仍需 |
|---|---|---|---|
| 1 | `auth/authsrv/authsrv.go` `loadSSOConfig` | 加 `case "<type>"` → `dbdata.GetAuth<Type>` | 是 |
| 2 | `dbdata/provider.go` | `providerTypes`/`providerNames` + `ValidateProviderConfig`/`ResolveProviderConfig` 加 `<type>` | 是 |
| 3 | `dbdata/userauth_<type>.go`（`GetAuth<Type>`/`Sync<Type>Users`） | 仿 `GetAuthWework`/`GetAuthFeishu` 读配置 + 同步用户；`provider.go` 的 `ProvSecretKeys` 注册 `client_secret` 做加密脱敏 | 是 |
| 4 | `handler/link_auth_saml.go` | 在 `SAMLSPLogin` 已有分发（按 `ssotype` 参数）处加 `case "<type>"` + 新增 `<Type>AuthCallback`（code 换 user、部门校验、建 SSO 会话、SetCookie） | 是（端点必须存在，但已不是"加 if 分支"，而是统一分发加 case） |
| 5 | `server/handler/server.go` | 加路由 `/<Type>Auth/callback` → `<Type>AuthCallback` | 是 |
| 6 | `handler/sso.go` `ssoProviders` | 加一项（`callbackPath` + `buildAuthURL`） | **是——这是 handler 出口层唯一必改点** |
| 7 | `dbdata/group.go` `SyncExternalUsersForOTP` | 加 `<type>` 分支（仅需支持 `[<type>,otp]` 组合时） | 视组合需求 |
| 8 | 前端 | Provider 类型下拉、认证步骤下拉加 `<type>`；`WebAuth.vue` SSO 跳转处理 | 是 |

> **AuthFlow 重构带来的变化（关键）**：重构前第 6 项需改 `handler/link_webauth.go` 的 `webAuthBuildSSOURL` 加分支、三端出口（`link_webauth.go`/`portal.go`/`link_auth_pipeline.go`）各自按 `sso_type` 手拼渲染，且 `portal.go` 的 `PortalSSO` 入口白名单也要手写新类型。重构后：
> - `BuildChallengeView` 统一把管道产出的 `sso_type` **透传**给 `ToXML`/`ToWebAuthJSON`/`ToPortalJSON`，三端渲染出口**不再需要按类型加分支**；
> - `PortalSSO` 入口校验改为读 `ssoProviders` 注册表（`ssoTypeEnabled`），门户白名单**不再硬编码**。
>
> 因此新增 SSO 只需在 `sso.go` 的 `ssoProviders` 注册，WebAuth / 原生 XML / 门户三端（渲染 + 入口校验）**全部自动生效，无需改这三端**。改动文件数从「三端各改 + 约 9 处」降为「`ssoProviders` 一处 + 必要的策略/实体/配置/路由/前端下拉」。
>
> 这些改动多与现有 `wxwork`/`feishu` 对称，本质是"复制那套改名字"；复杂度来自 OAuth2 回调 + 多端（客户端/门户/系统浏览器）协作的固有成本，非架构缺陷。

#### SSO 认证器骨架（最小示例）

```go
// auth/authsrv/dingtalk.go
package authsrv

import "github.com/wsczx/remlink/auth"

func init() {
    auth.Register("dingtalk", func() auth.Authenticator {
        return &DingtalkAuth{}
    })
}

// 实现 Challenger（不能嵌入 NopChallenger）
type DingtalkAuth struct {
    auth.DingtalkConfig // 嵌入配置，Provider 配置自动注入
}

func (a *DingtalkAuth) Name() string { return "dingtalk" }

func (a *DingtalkAuth) Authenticate(ctx *auth.Context) (auth.StepResult, error) {
    if sso := ctx.SSO; sso != nil {
        if sso.Authenticated && sso.UserID != "" { // 路径①：回调已完成
            ctx.Conn.Username = sso.UserID
            return auth.StepPass, nil
        }
        if sso.Code != "" { // 路径②：管道内用 code 换用户
            userID, err := a.GetDingtalkUser(sso.Code)
            if err != nil {
                return auth.StepFail, err
            }
            // （可选）部门校验
            ctx.Conn.Username = userID
            return auth.StepPass, nil
        }
    }
    return auth.StepPending, nil // 路径③：首次进入，等待 SSO 回调
}

func (a *DingtalkAuth) Challenge() *auth.ChallengeInfo {
    return &auth.ChallengeInfo{
        Type:     auth.ChallengeSSO,
        Template: "saml",
        Data:     map[string]any{"sso_type": "dingtalk", "use_default_browser": a.UseDefaultBrowser},
    }
}
```

#### SSO 类型识别

只要实现 `Challenger` 且 `Challenge().Type == ChallengeSSO`，`IsSSOType()`（`registry.go`）会通过工厂实例反射自动识别——**无需在 `GetSSOType()` 中添加硬编码判断**。若新类型有浏览器模式配置（`UseDefaultBrowser`），在 `authsrv.go` 的 `loadSSOConfig` switch 中添加分支即可（见上表第 1 项）。

---

## Pipeline 执行流程

```
                      init 请求
                         │
                         ▼
                   handler 解析 XML
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
          SSO 预检   证书自动认证   常规认证
       (authsrv.    (authsrv.     (authsrv.
        GetSSOType) CanCertAutoAuth) Authenticate)
                         │          │
                         ▼          ▼
              ┌──── auth/authsrv ────┘
              │   加载组 → 解析 Profile
              │   含 otp 且首步非 local 时预载 UserInfo
              │
              ▼
        pipeline.Run/Resume  ← GetPipeline + runFrom
              │
   ┌──────────┼──────────┐
   ▼          ▼          ▼
StepFail   StepPending  StepPass（所有步骤通过 + 身份一致性校验）
   │          │          │
   拒绝       │          创建会话
              ▼
       通知客户端输入额外信息（OTP码/SSO重定向）
              │
              ▼
       客户端提交额外信息
              │
              ▼
   authsrv.Resume(ctx, state)  ← 加载组（组名取自 ctx.Conn.GroupName）+ 从断点恢复
              │
              ▼
         继续执行后续步骤...
```

### 关键规则

1. **步骤按序执行**：`profile.Step` 数组的顺序即执行顺序，不可跳过。
2. **全通过才算通过**：所有 Step 都返回 `StepPass`，管道才返回成功。
3. **身份一致性校验**：所有步骤通过后，若 `ctx.Identity != ""`（如证书 CN），校验其与 `ctx.Conn.Username` 一致，不一致则 `StepFail`。用于证书认证防身份冒用。
4. **断点恢复**：`StepPending` 后客户端再次请求时，`authsrv.Resume(ctx, state)` 从上次挂起的步骤（`state.StepIdx`）继续，组名取自 `ctx.Conn.GroupName`。已通过的步骤（`state.PassedSteps`）不重复执行。
5. **SSO Token 注入**：OAuth2 回调成功后，回调处理（`handler/link_auth_saml.go`）在 `ctx.SSO` 保存已验证标记（含部门校验结果），管道中的 SSO 步骤检测到此标记直接 `StepPass`，后续步骤（如 OTP）继续执行。

---

## 现有认证器一览

| 名称       | 文件                  | 类型                                                    | Challenge | Challenge 类型      |
| ---------- | --------------------- | ------------------------------------------------------- | :-------: | ------------------- |
| `local`  | `authsrv/local.go`  | 本地密码（bcrypt，支持密码+OTP 尾部合并输入，兜底剥离） |    ✗    | —                  |
| `ldap`   | `authsrv/ldap.go`   | LDAP/AD Bind 认证                                       |   ✗¹   | —                  |
| `radius` | `authsrv/radius.go` | RADIUS 认证                                             |   ✓²   | `ChallengeRADIUS` |
| `cert`   | `authsrv/cert.go`   | TLS X.509 客户端证书                                    |    ✗    | —                  |
| `otp`    | `authsrv/otp.go`    | TOTP 动态码（独立窗口）                                 |    ✓    | `ChallengeOTP`    |
| `sms`    | `authsrv/sms.go`    | 短信验证码                                              |    ✓    | `ChallengeSMS`    |
| `wxwork` | `authsrv/wxwork.go` | 企业微信 OAuth2                                         |    ✓    | `ChallengeSSO`    |
| `feishu` | `authsrv/feishu.go` | 飞书 OAuth2（支持部门限制 + 拒绝用户名单）             |    ✓    | `ChallengeSSO`    |
| `dingtalk` | `authsrv/dingtalk.go` | 钉钉 OAuth2（支持部门限制 + 拒绝用户名单）           |    ✓    | `ChallengeSSO`    |
| `admin`  | `authsrv/admin.go`  | 管理员用户（后台登录，密码 + 可选 OTP）                 |    ✗    | —                  |

> ¹ LDAP 嵌入 `NopChallenger`，无交互挑战。缺少 `Username/Password` 时返回 `StepPending`（等待输入凭据），故只适用于凭据由用户直接提供的管道，**不适用于 SSO/cert 先行**（见「认证组合能力边界」）。
>
> ² RADIUS 实现 `Challenger` 接口：收到服务端 Access-Challenge 时返回 `StepPending` + `ChallengeRADIUS` 二次验证提示（`Resume` 时用验证码作为 User-Password 带回 State）；缺凭据时同样返回 `StepPending`（等待输入）。

编排层（`authsrv/authsrv.go`）对外暴露的函数：`Authenticate`/`Resume`/`LoadUserInfo`/`GetSSOType`/`CanCertAutoAuth`/`GetSSOBrowserMode`。

---

## LockManager 防暴力破解

`lockmanager.go` 实现三级锁定策略，为全局单例，后台定时清理过期记录：

| 级别     | 触发条件                   | 锁定范围          | 默认阈值 | 默认锁定时长 |
| -------- | -------------------------- | ----------------- | -------- | ------------ |
| 用户+IP  | 同一用户从同一 IP 连续失败 | 该 IP 上的该用户  | 5 次     | 5 分钟       |
| 全局用户 | 同一用户从任意 IP 累计失败 | 该用户（所有 IP） | 20 次    | 5 分钟       |
| 全局 IP  | 同一 IP 上任意用户累计失败 | 该 IP（所有用户） | 40 次    | 5 分钟       |

若在一段时间内无新失败记录，计数器自动重置。阈值和锁定时长可通过管理后台配置。

### 多步管道的防爆计数

LockManager 按 username+ip 维度单一计数，不区分认证方法。多步管道（如 `[ldap, otp]`）中，密码阶段和 OTP 阶段共享同一个计数器。为避免密码阶段耗掉的额度挤占 OTP 阶段，在**首次挑战**（前序凭据通过、进入挑战阶段）时调用 `lockManager.Success` 重置计数，给后续挑战阶段一个独立的计数窗口。

挑战码错误时，通过 `PipelineResult.IsChallengeRetry()` 判断是否原地踏步（`StepPending && State.StepIdx == PrevStepIdx`），只有原地踏步才计入 `Fail`。推进到下一挑战阶段不算失败。

---

## Provider 配置注入

第三方认证（企微/飞书/钉钉/LDAP/RADIUS）的服务端配置（AppID、Secret、服务器地址等）通过 Provider 机制统一管理，与认证步骤解耦：

```
管理后台创建 Provider（如"飞书生产环境"）
    │
    ▼
dbdata 存储 provider 配置（JSON，敏感字段加密）
    │
    ▼
GroupAuthProfile.Step 引用 provider 名称
    │
    ▼
GetPipeline 调用 resolver(providerName, stepType)  →  dbdata.ResolveProviderConfig
    │
    ▼
GetProviderConfigFromMap 反序列化到认证器实例
```

配置变更（如更换 AppSecret）无需修改认证组步骤，即时生效。

---

## 设计原则

- **核心 `auth` 为稳定契约叶子**：`auth.go`/`pipeline.go`/`registry.go`/`config*.go`/`lockmanager.go` 不依赖 `dbdata`/`handler`，可独立测试与复用（测试用外部测试包 `package auth_test`）。
- **工厂注册**：每个认证器通过 `init() + auth.Register()` 自注册，新增文件即可生效，无需改动其他代码。
- **配置解耦**：认证器不关心配置从哪来（数据库/文件），由 `GetPipeline` 的 `resolver`（`dbdata.ResolveProviderConfig`）注入。
- **实现层查库**：需要查库的逻辑（认证器、组加载、SSO 检测、OTP 预载、用户信息投影、管道编排）统一放在 `auth/authsrv`，它直接 import `dbdata`，避开循环依赖。核心 `auth` 不依赖 `dbdata`，故编排函数作为 `authsrv.X` 暴露，调用方直接引用。
- **并发安全**：认证器实例在 `GetPipeline` 时创建一次，管道单次执行中复用。认证器本身应无内部可变状态（跨请求的缓存如 SMS 验证码、admin OTP 防重放用包级带锁 map 单独管理）。
- **handler 边界**：handler 只负责 XML/JSON 解析、会话管理（`AuthSession`/`SessionStore`）、构造 `Flow` 并注入三端回调（`OnPass`/`OnChallenge`/`OnFail`）、渲染响应。**「跑管道 → 按结果分发 → 统一锁定计数 → 存挑战断点」已由 `handler/authflow.go` 的 AuthFlow 统一收口**，三端不再各自写分发；认证编排逻辑仍全部在 `auth/authsrv` 内。

### 会话层（handler 侧）

`handler` 持有 `SessionStore`（`map[string]*AuthSession`）与 `AuthSession`：

```go
type AuthSession struct {
    SessionID  string             // 会话唯一标识（SessStore key）
    Ctx        *auth.Context      // 管道状态唯一载体（含 stepIdx/passedSteps 断点）
    UserActLog *dbdata.UserActLog // 审计日志（dbdata 类型，核心 auth 不能持有）
    CreatedAt  time.Time          // TTL
}
```

- `Context` 在核心 `auth` 包，因 import 边界**不能**持有 `UserActLog`（dbdata 类型）与 `SessionID`/`CreatedAt`（会话关注点），故由 handler 层的 `AuthSession` 信封承载。
- `SessStore` 为纯内存 TTL 基础设施，与认证逻辑无关；Context 无需序列化，`Resume` 时 TLS 从新请求重新注入。

### Handler 文件结构

```
handler/
├── authflow.go             # AuthFlow 状态机（三端共用收口）：跑管道 + 统一锁定计数 + 分发到回调
├── link_auth.go            # 入口 + init/SSO 分流 + 证书自动认证
├── link_auth_pipeline.go   # 原生 XML 端点：构造 Flow（OnPass=建VPN会话/OnChallenge=渲染XML）→ Dispatch 预计算结果
├── link_auth_tpl.go        # XML 模板 + RequestData
├── link_auth_otp.go        # AuthSession + 会话 CRUD
├── link_auth_saml.go       # SAML/企微/飞书/钉钉 SSO 端点
├── link_auth_webauth.go    # WebAuth 端点：构造 Flow（JSON 渲染）→ Run/Resume
├── portal.go               # 门户端点：构造 Flow（写 portalAuthResponse）→ Run/Resume
├── link_auth_session.go    # CreateSession 创建会话入口
└── link_auth_*_test.go     # 单元 / 端到端测试
```

> **三端如何使用 AuthFlow**：
> - **原生 XML（`link_auth_pipeline.go`）**：因建 VPN 会话前需先用 `result` 做上下文准备（如注入 `SessionID`），它先拿到 `result` 再调 `flow.Dispatch(w, r, result)`——只分发不重跑管道。
> - **WebAuth（`link_auth_webauth.go`）**：直接 `flow.Run` / `flow.Resume`，由 AuthFlow 调管道。
> - **门户（`portal.go`）**：同 WebAuth 用 `flow.Run` / `flow.Resume`；其回调不直写 `w`，而是把结果写入本地 `portalAuthResponse` 结构体，由 `portalOK` 后续写出（保持门户响应契约）。
