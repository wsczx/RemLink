## 常见问题

### AnyConnect 客户端问题

> 请使用官方 AnyConnect / OpenConnect 客户端，其他版本未经充分测试，不保证可用性。

### OTP 动态码

> 请使用手机安装 freeotp 或 Google Authenticator，扫描管理后台显示的 OTP 二维码，生成的 6 位数字即是动态码。
>
> 管理员 OTP 有 90 秒防重放保护，同一验证码在 90 秒内不可重复使用。

### 企业微信 / 飞书扫码登录

> 1. 在管理后台「认证提供方」页面创建对应的 Provider（填写 AppID、Secret 等信息）
> 2. 在用户组的「认证配置」中添加 `wxwork` 或 `feishu` 步骤
>
> 门户也支持企业微信/飞书 SSO 直接登录（非 WebAuth 流程）。

### RADIUS Access-Challenge 二次验证

> RemLink 支持 RADIUS Access-Challenge 协议，服务端会下发挑战信息给客户端。
>
> 适用于需要短信验证码、硬件 Token 等二次验证场景。在认证 Pipeline 中添加 `radius` 步骤即可自动处理 Challenge 流程。

### 认证 Pipeline 编排

> 用户组的认证流程由多个「步骤」按序组合，例如 `[local, otp]` 表示先本地密码、再 TOTP 动态码。
>
> 每个步骤通过才继续下一步，任意步骤失败则认证终止。支持断点恢复（多步骤认证中途断开后可继续）。
>
> 支持的步骤类型：`local`（本地密码）、`ldap`、`radius`、`cert`（客户端证书）、`otp`（TOTP）、`sms`（短信验证码）、`wxwork`（企微）、`feishu`（飞书）

### 用户策略与组策略

> 只要有用户策略，组策略就不生效，相当于覆盖了组策略的配置。
>
> 策略支持批量应用到多个组或用户（管理后台策略列表 → 应用到组/用户）。

### 客户端证书

> - 支持 P12 和 PEM（CSR 模式）两种签发方式
> - 支持设备绑定（限制证书只能在指定数量的设备上使用）
> - 用户可在门户自助申请和下载证书

### 客户端连接名称

> 客户端连接名称在管理后台「软件配置」→「Profile 配置」中在线编辑（支持 AnyConnect Profile XML）：

```xml
<HostEntry>
    <HostName>VPN</HostName>
    <HostAddress>localhost</HostAddress>
</HostEntry>
```

### dpd timeout 设置问题

```yaml
# 客户端失效检测时间(秒) dpd > keepalive
cstp_keepalive = 4
cstp_dpd = 9
mobile_keepalive = 7
mobile_dpd = 15
```

> 以上参数为客户端超时检测时间。如一段时间内没有数据传输，防火墙会主动关闭连接。
>
> 如经常出现 timeout 错误，应根据当前防火墙的设置适当减小 dpd 数值。

### 审计日志 audit_interval 参数

> 默认值 `audit_interval = 600` 表示相同日志 600 秒内只记录一次，不同日志首次出现立即记录。在管理后台「软件配置」页面修改，即时生效无需重启。
>
> 去重 key 格式：源 IP + 目的 IP + 目的端口 + 协议类型 + 域名 MD5

### 反向代理问题

> RemLink 仅支持四层反向代理，不支持七层反向代理。如使用 Nginx 请用 stream 模块：

```conf
stream {
    upstream remlink_server {
        server 127.0.0.1:8443;
    }
    server {
        listen 443 tcp;
        proxy_timeout 30s;
        proxy_pass remlink_server;
    }
}
```

> Nginx 共用 443 端口示例（按 SNI 分流）：

```conf
stream {
    map $ssl_preread_server_name $name {
        vpn.xx.com        myvpn;
        default           defaultpage;
    }

    upstream myvpn {
        server 127.0.0.1:8443;
    }
    upstream defaultpage {
        server 127.0.0.1:8080;
    }

    server {
        listen 443 so_keepalive=on;
        ssl_preread on;
        #接收端也需要设置 proxy_protocol
        #proxy_protocol on;
        proxy_pass $name;
    }
}
```

### 性能参考

```
内网环境测试数据
虚拟服务器：CentOS 7 4C8G
网络模式：tun + TCP 传输
客户端文件下载速度：240 Mb/s
客户端网卡下载速度：270 Mb/s
服务端网卡上传速度：280 Mb/s
```

> TLS 加密协议、隧道 header 头都会占用一定带宽。

### 登录防爆说明

```
1. 用户 A 在 IP 1.2.3.4 上尝试登录:
   失败 5 次，触发该 IP 上的用户 A 锁定 5 分钟。
   在这 5 分钟内，用户 A 从 IP 1.2.3.4 无法进行新的登录尝试。

2. 用户 A 更换 IP 到 1.2.3.5 继续尝试登录:
   累计失败 20 次，触发全局用户 A 锁定 5 分钟。
   在这 5 分钟内，用户 A 从任何 IP 地址都无法进行新的登录尝试。

3. IP 1.2.3.4 上多个用户尝试登录:
   累计 40 次失败登录尝试（无论来自多少不同用户），触发该 IP 的全局锁定 5 分钟。
   在这 5 分钟内，从 IP 1.2.3.4 的所有登录尝试都将被拒绝。

如果在 N 分钟内没有新的失败尝试，失败计数会在 N 分钟后（*_reset_time）重置。
```

### UFW / firewalld 兼容问题

> RemLink 支持 nftables 和 iptables 两种防火墙后端，UFW 冲突**仅在 nftables 后端下发生**。

**确认当前后端**：

```shell
grep "Firewall driver" /var/log/remlink.log
# "using nftables" → 可能与 UFW 冲突
# "using iptables"  → 不会与 UFW 冲突
```

**根因**：nftables 的 `accept` 只终止当前 base chain，不阻止同 hook 上其他 base chain 的处理。UFW 的 FORWARD 链（priority=0）会在 RemLink 的链之后执行 DROP 策略。

**症状**：VPN 连接成功但无法访问外网，`ufw disable` 后恢复正常。

**解决方案**：

1. 放行全局 VPN 网段（默认 `192.168.90.0/24`，以实际配置为准）：

```shell
# UFW
ufw route allow from 192.168.90.0/24
ufw reload

# firewalld
firewall-cmd --permanent --add-rich-rule='rule source address="192.168.90.0/24" accept'
firewall-cmd --reload
```

2. 组级别独立网段也需要单独放行：

```shell
ufw route allow from 10.0.1.0/24
ufw reload
```

> 添加 UFW 规则后无需重启 RemLink，执行 `ufw reload` 即可立即生效。

### 流量配额

> 支持为用户设置流量配额（上下行），支持 daily / weekly / monthly 自动重置。
>
> 超出配额后自动下线，在管理后台用户列表中配置。

### 敏感字段加密

> 数据库中的敏感字段（管理员密码、JWT 密钥、证书密钥、Provider 配置、SMTP/SMS 密码等）支持 AES-256-GCM 加密存储。
>
> 加密为可选功能，需在管理后台「安全设置」页面手动启用。启用后密钥文件 `.encryption_key` 默认保存在工作目录。
>
> 可通过环境变量自定义密钥位置：
> - `REMLINK_ENCRYPTION_KEY` — 指定密钥文件的完整路径
> - `REMLINK_ENCRYPTION_KEY_DIR` — 指定密钥文件的存放目录
>
> **注意**：环境变量仅在密钥文件尚未生成时生效。如需迁移密钥路径，请先关闭加密，再移动密钥文件并设置环境变量。

### 私有证书问题

> RemLink 默认不支持私有证书，其他使用私有证书的问题请自行解决。

### 证书申请

> RemLink 内置 Let's Encrypt / TrustAsia ACME 证书自动申请功能，可在管理后台「证书设置」页面配置。
>
> 也支持手动上传自定义证书（PEM 格式）。
