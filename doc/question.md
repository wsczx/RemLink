## 常见问题

### AnyConnect 客户端问题

> 推荐使用官方 AnyConnect 客户端，可以使用webauth认证功能，体检更佳，其他版本未经充分测试，不保证可用性。

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
> - 支持设备绑定（限制证书只能在指定数量的设备上使用【仅对cisco anyconnect生效】）
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

> RemLink 支持 nftables 和 iptables 两种防火墙后端，UFW 和firewalld某些情况下可能和 **nftables 后端冲突**。

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
>
> - `REMLINK_ENCRYPTION_KEY` — 指定密钥文件的完整路径
> - `REMLINK_ENCRYPTION_KEY_DIR` — 指定密钥文件的存放目录
>
> **注意**：环境变量仅在密钥文件尚未生成时生效。如需迁移密钥路径，请先关闭加密，再移动密钥文件并设置环境变量。

### 私有证书问题

> RemLink 默认不支持私有证书，其他使用私有证书的问题请自行解决。

### IPv6 双栈（0.17.1 起支持）

> 在管理后台「软件配置 → 虚拟网络」填写 **IPv6 地址段（ipv6_cidr）** 即可开启双栈；留空则保持纯 IPv4。开启后重启服务、客户端重连生效。

**客户端拿到了 IPv6 地址，但上不了公网？**

- **网关都 ping 不通**：IPv6 隧道端点未建立。请确认已保存配置并重启服务、客户端已重连；并在系统日志中查看是否有报错（如权限不足）。
- **网关通、公网不通（NAT 模式）**：检查服务器出网网卡是否具备公网 IPv6 地址，以及「IPv6 全局 NAT」开关是否被关闭。
- **网关通、公网不通（纯路由模式）**：运营商 / 上游未把该 IPv6 地址段路由回 RemLink 服务器。若为VPS 的共享 /64 段而切成了纯路由，会因上游无回程而失败，建议改用 ULA 地址段（如 `fd00:c0de::/64`）并保持 NAT 模式。

**VPS 怎么配 IPv6 最省事？**

建议「IPv6 地址段」填一段 ULA 地址（如 `fd00:c0de::/64`，以 `fd` 开头即可），保持「IPv6 全局 NAT」为开启（默认）。客户端经服务器公网 IPv6 出网，无需上游配合。

**开启 IPv6 需要手动改系统 sysctl 吗？**

不需要。开启双栈后 RemLink 会自动开启系统 IPv6 转发，重启服务即可生效。

**只想让客户端访问内网 IPv6，不需要上公网？**

直接填写任意地址段并关闭「IPv6 全局 NAT」即可，客户端间、客户端与内网 IPv6 资源可正常互通。

**关闭 IPv6 全局 NAT 后，IPv4 的 NAT 也会一起关掉吗？**

不会。IPv4 与 IPv6 的 NAT 是各自独立的开关：「IPv4 全局 NAT」管 IPv4，「IPv6 全局 NAT」只管 IPv6。所以你可以「IPv4 继续 NAT 出网 + IPv6 纯路由」，只需关掉 IPv6 那一个，不影响 IPv4。

**「排除出口 IP」对 IPv6 也生效吗？**

生效。开启该选项后，服务端会同时把客户端的 IPv4 与 IPv6 出口地址从隧道中排除（不再加密传输），IPv4 / IPv6 都适用。

### 证书申请

> RemLink 内置 Let's Encrypt / TrustAsia ACME 证书自动申请功能，可在管理后台「证书设置」页面配置。
>
> 也支持手动上传自定义证书（PEM 格式）。

### 数据库：首次使用非 SQLite 与数据库切换

默认使用 SQLite（`conf/remlink.db`），无需任何配置。如需使用 MySQL / PostgreSQL / MSSQL：

**场景一：首次安装即使用外部数据库**

在首次启动前指定数据库连接，有三种等效方式（优先级：`conf/db.json` 最高，其次命令行参数 / 环境变量）：

1. 创建 `conf/db.json`（推荐，优先级最高）：

```json
{
  "db_type": "mysql",
  "db_source": "user:pass@tcp(127.0.0.1:3306)/remlink?charset=utf8mb4"
}
```

2. 命令行参数：`./remlink --db_type mysql --db_source "user:pass@tcp(127.0.0.1:3306)/remlink?charset=utf8mb4"`

3. 环境变量：`LINK_DB_TYPE=mysql LINK_DB_SOURCE="user:pass@tcp(127.0.0.1:3306)/remlink?charset=utf8mb4" ./remlink`

> 外部库需**提前创建好空的数据库**（表结构由 RemLink 首次连接时自动初始化）。数据库驱动已内置，无需额外安装。
>
> `db_type` 支持：`sqlite3` / `mysql` / `postgres` / `mssql`。

**场景二：已运行的实例切换数据库**

在管理后台「软件配置」→「数据库」点击切换，向导会自动完成：测试连接 → 备份并迁移现有数据 → 写入 `conf/db.json` → 重启服务。支持 SQLite 与任意外部库之间互转。

各数据库的 `db_source` 格式详见 README「数据库」章节的对照表。

### WebVPN（Web 应用反向代理）

> WebVPN 把内网 HTTP/HTTPS 业务系统以统一域名下的子站点形式暴露给已登录用户。在「系统设置」填写 **WebVPN 域名**（如 `wv.example.com`，留空 = 功能关闭）并配置好 `*.wv.example.com` 泛域名证书后启用。

**WebVPN 必须与门户同主域吗？**

必须。WebVPN 会话（`webvpn_session`）与门户会话（`portal_session`）的 Cookie 共享域算法一致，均取「WebVPN 域名」的**父域**（如 `wv.example.com` → `.example.com`）。因此门户与 WebVPN 子站点必须处于同一主域下，否则「已登录门户自动兑换 WebVPN 会话」会失败、用户将被反复重定向到登录页。WebVPN 域名应配成类似 `wv.example.com` 的子域，与门户主域 `example.com`（或与 `vpn.example.com` 平级）同主域。

**访问子站点提示证书不安全 / 证书不匹配？**

确认「证书管理」里申请的 **WebVPN 泛域名证书** SAN 包含 `*.wv.example.com`。泛域名证书只覆盖 `*.wv.example.com` 这一级子域、不覆盖根域 `wv.example.com`，所以 WebVPN 域名应配成 `wv.example.com` 这样的子域、不要配成裸域；自签/其他 CA 上传的证书同样需含该 SAN，且客户端机器需先信任其根证书。

**页面能打开但样式/图片错乱、链接点不开？**

WebVPN 会改写页面中的绝对地址。若后端返回的是绝对内网地址且未走代理，检查后端是否把资源指向了绝对域名；尽量使用相对路径或同源资源。硬编码绝对内网 URL 的后端应用，子域名方案本身无法完全解决（webvpn 品类的固有难题）。

**配置了授权用户 / 用户组后部分用户无法访问？**

用户维度与组维度是**交集**关系——两者都填时用户必须**同时**满足（既在用户白名单中、又属于其中某个组）。确认用户名、组名完全匹配、区分大小写。两者都留空表示所有已登录用户可访问（**绝非匿名**，未登录会被重定向到登录页）。

**踢出会话后用户仍能访问？**

会话吊销基于 jti 黑名单 + 失效水位，**下次请求才会校验**，请让用户刷新页面验证；若仍异常，确认服务时间为最新且未跨多实例时钟漂移。注意服务重启会清空吊销状态，重启前签发的 JWT 最长 3h 内重新有效。

**审计日志为空？**

确认访问确实经过了 WebVPN 网关（即通过 `*.wv.example.com` 域名访问），直接访问后端 IP 不会记录。

**来源 IP 限制不生效？**

WebVPN 会清洗伪造的 `X-Real-IP` / `X-Forwarded-For` 头，以**真实 TCP 连接对端 IP** 作为「来源 IP」判断依据。若 RemLink 前面有负载均衡 / 七层反代终止了 TLS，连接对端会变成 LB 的内网 IP，来源 IP 白名单与审计客户端 IP 都会失真。解决：让 LB 透传真实客户端 IP（Proxy Protocol 或直连公网），不要在 WebVPN 之前套会改写源 IP 的七层反代。

**功能完全不生效 / 所有 WebVPN 路由 404？**

确认「系统设置 → WebVPN 域名」已填写（非空）。该字段为空时功能关闭，所有 WebVPN 路由不生效。

**用了自定义端口（如 8443）WebVPN 还能用吗？**

能用。浏览器访问时带上端口即可，例如 `https://nas.wv.example.com:8443`。未登录时跳转到门户登录页、以及门户「我的应用」跳转链接，都会**自动沿用你访问时的端口**，登录成功后正确回跳到带端口的子站点。请勿在链接里漏写端口。DNS 解析（泛域名 A 记录 / hosts）只负责把子域名指向服务器 IP，与端口无关。

**门户已登录，访问 WebVPN 子站点却被反复要求登录 / 重定向到登录页？**

多半是 **WebVPN 与门户不在同一主域**，导致 `webvpn_session` 与 `portal_session` 的 Cookie 共享域无法交叉识别，「自动兑换」失败。确认「系统设置 → WebVPN 域名」与门户主域属于同一父域（如 WebVPN `wv.example.com`、门户 `vpn.example.com`，父域均为 `example.com`）。两者父域不一致时必须调整。
