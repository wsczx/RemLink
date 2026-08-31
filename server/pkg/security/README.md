# 加解密与脱敏开发指南

## 架构概览

敏感字段分三种存储模式，由不同的类型和机制处理：

| 类型                    | 适用场景                       | DB 接口     | JSON 接口                         | 当前使用方                                                           |
| ----------------------- | ------------------------------ | ----------- | --------------------------------- | -------------------------------------------------------------------- |
| `EncryptedString`     | 字符串字段（密码、密钥、PEM）  | Scan/Value  | MarshalJSON/UnmarshalJSON         | SettingSmtp.Password, ClientCertData.PrivateKey, LegoUserData.Key 等 |
| `EncryptedJSON[T]`    | JSON blob 字段                 | FromDB/ToDB | MarshalJSON/UnmarshalJSON（透传） | Provider.Config                                                      |
| `SettingServerConfig` | 外部结构体中的普通 string 字段 | —          | 手动 MarshalJSON/UnmarshalJSON    | base.ServerConfig.JwtSecret/AdminOtp                                 |

批量加解密（启停加密时）有两类路径：

- `settingFactories` 列表：走 `SettingGet`+`SettingSave`（触发 MarshalJSON 加密）
- `migrateRawColumn`：走原始 SQL 绕过 Scan/Value 回路（ClientCertData、Provider）

## 新增敏感字段操作清单

### 场景 A：Setting 结构体中新增敏感字符串字段

以在 `SettingSmtp` 中新增 `ApiKey` 为例：

**1. 声明字段类型为 `EncryptedString`**

```go
// dbdata/setting.go
type SettingSmtp struct {
    // ...
    ApiKey security.EncryptedString `json:"api_key"`
}
```

**2. 无需改 settingFactories** — `SettingSmtp` 已在列表中，自动覆盖。

**3. API handler 返回前脱敏**

```go
// admin/api_other.go
data.ApiKey = data.ApiKey.Masked()
```

**4. API handler 保存时还原占位符**

```go
if data.ApiKey.IsPlaceholder() {
    data.ApiKey = old.ApiKey
}
```

**5. 用 `SettingSave` 而非 `SettingSet` 保存**（首次写入也能正确插入）

完成。DB 读写、JSON 序列化、批量加解密全部自动处理。

### 场景 B：xorm 表中新增敏感字符串字段

以在 `ClientCertData` 中新增 `ClientSecret` 为例：

**1. 声明字段类型为 `EncryptedString`**

```go
// dbdata/cert_client.go
type ClientCertData struct {
    // ...
    ClientSecret security.EncryptedString `json:"client_secret" xorm:"text"`
}
```

**2. 批量加解密注册** — 在 `secret_batch.go` 的 `EnableEncryption`/`DisableEncryption` 中添加 `migrateRawColumn` 调用：

```go
// EnableEncryption 中
n, _, err := migrateRawColumn("client_cert_data", "client_secret", "ClientCertData", true)

// DisableEncryption 中
n, w, err := migrateRawColumn("client_cert_data", "client_secret", "ClientCertData", false)
```

**3. API handler 返回前脱敏 + 保存时还原占位符**（同场景 A 的步骤 3-4）

### 场景 C：xorm 表中新增敏感 JSON blob 字段

以新增一个 `metadata` 列为例：

**1. 声明字段类型为 `EncryptedJSON`**

```go
// dbdata/tables.go
type SomeTable struct {
    // ...
    Metadata security.EncryptedJSON[json.RawMessage] `json:"metadata" xorm:"text"`
}
```

**2. 批量加解密注册** — 同场景 B，用 `migrateRawColumn` 注册该列。

**3. API handler 返回时自动透传明文**（EncryptedJSON 的 MarshalJSON 直接返回内部 Data，无需脱敏）。

如果 blob 内含敏感数据需要脱敏，在 handler 层手动处理。

### 场景 D：SettingServerConfig 中新增敏感字段

以新增 `AdminToken` 为例（不推荐，尽量用场景 A）：

**1. 在 `base.ServerConfig` 中添加普通 string 字段**

```go
// base/config.go
type ServerConfig struct {
    // ...
    AdminToken string `json:"admin_token"`
}
```

**2. 在 `SettingServerConfig` 的 `MarshalJSON`/`UnmarshalJSON` 中添加加解密**

```go
// dbdata/setting.go
func (s SettingServerConfig) MarshalJSON() ([]byte, error) {
    if security.IsEnabled() {
        s.Config.JwtSecret = security.EncryptIfNeeded(s.Config.JwtSecret)
        s.Config.AdminOtp = security.EncryptIfNeeded(s.Config.AdminOtp)
        s.Config.AdminToken = security.EncryptIfNeeded(s.Config.AdminToken)  // 新增
    }
    // ...
}

func (s *SettingServerConfig) UnmarshalJSON(data []byte) error {
    // ...
    if security.IsEnabled() {
        s.Config.JwtSecret = security.DecryptIfNeeded(s.Config.JwtSecret)
        s.Config.AdminOtp = security.DecryptIfNeeded(s.Config.AdminOtp)
        s.Config.AdminToken = security.DecryptIfNeeded(s.Config.AdminToken)  // 新增
    }
    return nil
}
```

**3. 无需改 settingFactories 或 migrateRawColumn** — `SettingServerConfig` 走 `saveServerConfig`，MarshalJSON 自动覆盖。

**4. API handler 中手动脱敏和还原** — 由于 `AdminToken` 是普通 string，不享受 `Masked()`/`IsPlaceholder()`，需手动处理。

## 关键约束

### 为什么有四套机制

| 机制                                     | 存在原因                                                                                  |
| ---------------------------------------- | ----------------------------------------------------------------------------------------- |
| `EncryptedString` Scan/Value           | xorm 列级类型，ClientCertData 用                                                          |
| `EncryptedJSON` FromDB/ToDB            | xorm 列级泛型，Provider 用                                                                |
| `SettingServerConfig` 手动 MarshalJSON | `base.ServerConfig` 是外部结构体，不能改字段类型为 EncryptedString（循环依赖 + 侵入性） |
| `migrateRawColumn` 原始 SQL            | 启停加密时必须绕过 Scan/Value 的自动加解密回路                                            |

### 启停加密的回路问题

`EnableEncryption` 走 `SettingGet`（触发 UnmarshalJSON 解密）→ `SettingSave`（触发 MarshalJSON 加密），看似没问题。但 `ClientCertData`/`Provider` 走 xorm 的 Scan/Value，`Scan` 读出时自动解密成明文，`Value` 写入时又自动加密——等于什么都没做。所以这两类必须用 `migrateRawColumn` 绕过 Scan/Value。

### MarshalJSON 默认加密

`EncryptedString.MarshalJSON` 在加密启用时返回密文。这意味着：

- **DB 存储**（`SettingSave` → `json.Marshal`）：密文入库 ✓
- **API 响应**：如果不调 `.Masked()`，前端拿到密文（不是明文，安全可接受，但对前端是脏数据）
- **必须**在 API handler 返回前调 `.Masked()` 脱敏

### 占位符还原流程

前端发回 `******` 表示"未修改，保留原值"：

1. handler 收到请求，字段值为 `******`
2. `IsPlaceholder()` 返回 true
3. 从 DB 读取旧值（`SettingGet` 自动解密）
4. 用旧值替换占位符
5. `SettingSave` 保存（MarshalJSON 自动加密）

## 现有敏感字段清单

| 字段                                       | 类型                           | 存储位置          | 脱敏位置     |
| ------------------------------------------ | ------------------------------ | ----------------- | ------------ |
| SettingSmtp.Password                       | EncryptedString                | Setting JSON blob | api_other.go |
| SettingSms.AliAccessKeySecret              | EncryptedString                | Setting JSON blob | api_other.go |
| SettingSms.TencentSecretKey                | EncryptedString                | Setting JSON blob | api_other.go |
| SettingTLSCert.CertKeyContent              | EncryptedString                | Setting JSON blob | —           |
| SettingClientCA.KeyContent                 | EncryptedString                | Setting JSON blob | —           |
| SettingLetsEncrypt.DNSProvider.*.SecretKey | EncryptedString                | Setting JSON blob | api_cert.go  |
| LegoUserData.Key                           | EncryptedString                | Setting JSON blob | —           |
| ClientCertData.PrivateKey                  | EncryptedString                | xorm 列           | api_cert.go  |
| Provider.Config                            | EncryptedJSON[json.RawMessage] | xorm 列           | —           |
| ServerConfig.JwtSecret                     | string（手动加密）             | Setting JSON blob | —           |
| ServerConfig.AdminOtp                      | string（手动加密）             | Setting JSON blob | —           |
