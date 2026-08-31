# RemLink

![GitHub release](https://img.shields.io/github/v/release/wsczx/RemLink)
![GitHub downloads](https://img.shields.io/github/downloads/wsczx/RemLink/total)
[![Gitee](https://img.shields.io/badge/Gitee-RemLink-C71D23?logo=gitee)](https://gitee.com/wsczx/remlink)
[![Docker pulls](https://img.shields.io/docker/pulls/wsczx/remlink.svg)](https://hub.docker.com/r/wsczx/remlink)

RemLink 是一个企业级远程办公软件，支持多人同时在线，兼容 AnyConnect(推荐) / OpenConnect 客户端。

> **声明**：RemLink 基于 [AnyLink](https://github.com/bjdgyc/anylink) 深度重构，在原项目基础上进行了认证架构重写、前端重构、安全加固与大量功能增强。感谢原作者 [bjdgyc](https://github.com/bjdgyc) 的开源贡献。

## 加入交流群

点击链接加入群聊【RemLink 交流群】：[https://qm.qq.com/q/3f7gEjcaVO](https://qm.qq.com/q/3f7gEjcaVO)

> 有任何使用问题、功能建议或合作意向，欢迎加入交流群讨论。

## 简介

RemLink 基于 [ietf-openconnect](https://tools.ietf.org/html/draft-mavrogiannopoulos-openconnect-02) 协议开发，借鉴 [ocserv](http://ocserv.gitlab.io/www/index.html) 思路，同时兼容 AnyConnect 客户端（推荐）。使用 TLS/DTLS 进行数据加密，支持 RSA 或 ECC 证书（自签证书 / Let's Encrypt / TrustAsia）。

## 端口说明

| 端口       | 用途                    |
| ---------- | ----------------------- |
| 443 (TCP)  | VPN 连接（TLS-TCP）     |
| 443 (UDP)  | VPN 连接（DTLS-UDP）    |
| 8800 (TCP) | 管理后台 Web 界面 + API |

> 管理后台访问地址：`https://<IP>:8800`
> VPN 连接地址：`<域名或IP>:443`

## 下载

从 [Releases](https://github.com/wsczx/RemLink/releases) 下载对应平台的二进制：`remlink-linux-amd64`（x86_64）或 `remlink-linux-arm64`（ARM64）。下载后重命名为 `remlink` 即可使用。

## 功能特性

<details>
<summary>点击展开完整功能列表</summary>

### 网络与基础设施

- IP 分配（IP、MAC 映射持久化）
- TLS-TCP 通道 / DTLS-UDP 通道
- 兼容 AnyConnect / OpenConnect 客户端
- tun 设备 NAT 模式 / tap、macvtap、ipvtap 设备桥接模式
- 支持 proxy protocol v1 & v2
- nftables/iptables 自动配置
- 流量压缩（LZS）、出口 IP 自动放行
- 空闲链接超时自动断开、流量速率限制
- 组级别独立 IP 池与 NAT 规则
- **IPv4 / IPv6 双栈支持**（叠加于任意网络模式，管理后台一键开启）
- 内置 Let's Encrypt / TrustAsia ACME 证书自动申请

### 认证体系

- 本地密码认证
- TOTP 动态码认证
- 客户端证书认证（支持设备绑定、CSR 模式）
- LDAP / AD 认证
- RADIUS 认证（含 Access-Challenge 二次验证）
- 企业微信 OAuth2 扫码登录
- 飞书 OAuth2 扫码登录
- OIDC 兼容的第三方身份认证
- SMS 短信验证码（腾讯云 + 阿里云，含防暴力破解）
- WebAuth 浏览器端认证
- 认证 Pipeline 可编排架构（多步骤自由组合 + 断点恢复）
- Provider 统一管理第三方认证配置
- 登录防爆（用户 + IP 三级锁定策略）
- 自动同步 LDAP / 企微 / 飞书用户

### 用户门户

- 客户端下载页面
- 证书自助申请与下载（P12格式）
- 在线设备管理与踢下线
- 密码自助重置（token 防重放 + 限流）
- OTP 动态码绑定
- 企业微信 / 飞书 SSO 直接登录
- 自定义品牌展示（Logo / 标题 / 副标题 / 页脚 / Favicon）
- 自定义仪表盘（公告 / 快捷链接 / 主题色 / 自定义 CSS / 客户端连接指引）

### Web 代理（Web 访问网关）

- 通过子域名安全发布内网 Web 应用（OA、Wiki、后台等），用户免装客户端、浏览器直连
- 独立 Web 代理域名 + 泛域名证书，与 VPN 隧道完全隔离
- 用户 / 组 / 路径 / 来源 IP 多级访问控制
- 访问审计日志（用户、路径、状态码、流量、风险等级，可导出 CSV）
- 管理员一键踢出用户会话

### 管理与运维

- Web 管理后台（自适应响应式）
- 用户 / 组 / 策略管理（策略支持批量应用到组/用户）
- 用户批量发邮件 / 批量删除
- 用户活动审计日志 / IP 访问审计 / 管理员操作审计日志
- 系统日志实时推送（WebSocket）
- 数据库在线切换（SQLite / MySQL / PostgreSQL / MSSQL，支持自动数据迁移）
- 数据备份与还原
- 在线升级
- 配置数据库化（支持命令行参数 / 环境变量覆盖，部分配置热更新）
- AnyConnect Profile XML 在线编辑
- 敏感字段 AES-256-GCM 加密存储（可选启用）+ API 脱敏
- 流量配额管理（支持 daily/weekly/monthly 自动重置）
- 安全 HTTP 响应头自动注入
- 支持 Docker 非特权模式
- pprof / statsviz 性能诊断工具（需手动开启）

</details>

## 快速开始

### Docker 部署（推荐）

```bash
docker run -itd --name remlink --privileged \
    -p 443:443 -p 8800:8800 -p 443:443/udp \
    -v /home/myconf:/app/conf \
    --restart=always \
    wsczx/remlink

# 查看随机生成的管理员密码
docker logs remlink 2>&1 | head -20
```

非特权模式（更安全）：

```bash
docker run -itd --name remlink \
    -p 443:443 -p 8800:8800 -p 443:443/udp \
    -v /dev/net/tun:/dev/net/tun --cap-add=NET_ADMIN \
    -v /home/myconf:/app/conf \
    --restart=always \
    wsczx/remlink
```

### Docker Compose

参考 `deploy/docker-compose.yaml`：

```yaml
services:
  remlink:
    image: wsczx/remlink:latest
    container_name: remlink
    privileged: true
    ports:
      - "443:443"
      - "8800:8800"
      - "443:443/udp"
    volumes:
      - ./conf:/app/conf
    restart: always
```

### 二进制部署

```bash
# 下载对应架构的二进制并重命名为 remlink
curl -fL https://github.com/wsczx/RemLink/releases/latest/download/remlink-linux-amd64 -o remlink
chmod +x remlink
sudo ./remlink
```

### 一键安装（推荐）

自动下载最新版二进制、安装到 `/usr/local/remlink`、注册 systemd 服务并放行防火墙端口：

```bash
curl -fsSL https://raw.githubusercontent.com/wsczx/RemLink/main/deploy/install.sh | sudo bash
# 或指定版本： sudo VERSION=0.18.5 bash install.sh
# 仅安装不启动： sudo bash install.sh --no-start
```

脚本位于 `deploy/install.sh`，亦可下载后本地执行。

### Systemd 服务（手动）

```bash
sudo mkdir -p /usr/local/remlink
sudo curl -fL https://github.com/wsczx/RemLink/releases/latest/download/remlink-linux-amd64 -o /usr/local/remlink/remlink
sudo chmod +x /usr/local/remlink/remlink
sudo cp deploy/remlink.service /usr/lib/systemd/system/  # CentOS
# Ubuntu: /lib/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now remlink
```

## 首次使用

1. 启动后查看日志获取随机生成的管理员密码
2. 浏览器访问 `https://<IP>:8800` 登录管理后台
3. 进入「系统设置 > 安全设置」修改管理员密码
4. 在「软件配置」中设置 `link_mode`（tun/tap/macvtap/ipvtap）和网络参数
5. 在「证书设置」中配置 TLS 证书（测试可用自签证书，生产建议申请正式证书）
6. AnyConnect 客户端连接 `<域名>:443`

> 测试环境使用自签证书时，需在客户端取消勾选「阻止不受信任的服务器」。

## 管理命令

```bash
./remlink -h                        # 查看帮助
./remlink tool -d                   # 查看所有配置项
./remlink tool -s                   # 生成 JWT 密钥
./remlink --reset-admin-password    # 重置管理员密码（需先停止服务）
./remlink --disable-admin-otp       # 禁用管理员 OTP（需先停止服务）
```

## 数据库

默认 SQLite，无需配置。支持在线切换到 MySQL / PostgreSQL / MSSQL：

| db_type  | db_source                                                      |
| -------- | -------------------------------------------------------------- |
| sqlite3  | `./conf/remlink.db`                                          |
| mysql    | `user:pass@tcp(127.0.0.1:3306)/remlink?charset=utf8mb4`      |
| postgres | `postgres://user:pass@localhost/remlink?sslmode=verify-full` |
| mssql    | `sqlserver://user:pass@localhost?database=remlink`           |

- **首次安装即用外部数据库**（MySQL / PostgreSQL / MSSQL）：启动前在 `conf/db.json` 写入 `db_type` / `db_source`，或用 `--db_type` / `--db_source` 参数、环境变量 `LINK_DB_TYPE` / `LINK_DB_SOURCE` 指定。详见 [常见问题](doc/question.md)。
- **运行中切换数据库**：管理后台「软件配置」→ 数据库「切换」按钮，向导自动完成测试连接、数据迁移、写回配置并重启，支持 SQLite 与外部库互转。

## 网络模式

`link_mode` 支持 `tun` / `tap` / `macvtap` / `ipvtap` 四种模式，可在管理后台「软件配置 → 虚拟网络」中设置，或配置 `link_mode` 参数，修改后重启服务生效。其中 `tun` 既可作为三层 NAT 隧道（默认），也可配合内核 `proxy_arp` 用作 ARP 代理桥接（即 anylink 俗称的 `arp_proxy`）。

### tun 模式（推荐，三层 NAT 隧道）

客户端传输 IP 层数据，性能最佳。服务端自动设置 IP 转发和 NAT，客户端获得 VPN 私网 IP（如 `192.168.90.0/24`）后通过 NAT 访问外部与内网。

- 适用场景：绝大多数场景，尤其是云服务器、只需出网或访问指定内网网段。
- 配置要点：无需主网卡混杂模式；`global_nat=true`（默认）时自动添加全局 NAT 规则。

### tun + proxy_arp 桥接

`tun` 模式配合 Linux 内核 `proxy_arp` 可实现 ARP 代理桥接：客户端获得与内网同段的真实 IP，内网机器能直接二层访问客户端，无需 NAT、也无需混杂模式或网桥。

原理：客户端 IP 配在 tun 接口上；当 `ipv4_cidr` 与主网卡（如 `eth0`）同网段、且主网卡开启 `proxy_arp` 时，内网机器对客户端 IP 的 ARP 请求会由内核代答（内核知道该 IP 的路由走 tun 接口），从而把流量经 tun 转发给客户端。

- 适用场景：需要客户端使用内网真实 IP，但不想配置网桥 / 混杂模式的场景。
- 配置要点：手动开启内核 `proxy_arp`（`sysctl -w net.ipv4.conf.all.proxy_arp=1`，并写入 `/etc/sysctl.conf` 持久化）；关闭 NAT（`global_nat=false`）；`ipv4_cidr` 必须与 `ipv4_master` 网卡现有网段一致，`ipv4_gateway` 填主网卡自身 IP；无需混杂模式。
- 限制：与 tap / macvtap 相同，云环境通常不支持（网卡 MAC 加白、802.1x 认证网络受限）。

### tap 模式（桥接 / 用户态 ARP 代答）

服务端在用户态用 `arpdis` 对客户端做 ARP 代答，使客户端获得与内网同段的真实 IP。客户端传输二层帧，服务端需做链路层到 IP 层的转换，性能略低于 tun。注意：此处的用户态 ARP 代答**不同于**上文的 `proxy_arp`（后者是 tun 模式下由 Linux 内核完成的 ARP 代理）。

- 适用场景：需要客户端真正接入二层广播域（广播 / 多播 / 非 IP 协议穿透），或环境不支持 `macvtap` 内核模块时作为兼容 / 兜底。基于标准 Linux 网桥（`remlink0`），成熟可控，不依赖 `macvtap` 模块。
- 配置要点：主网卡开启混杂模式（`ip link set dev eth0 promisc on`）；关闭 NAT（`global_nat=false`）；正确设置 `ipv4_master` / `ipv4_cidr` / `ipv4_gateway`。

### macvtap 模式（桥接 / 内核态）

基于内核 `macvtap` 模块，由内核直接桥接，性能优于 tap。客户端同样获得内网真实 IP。

- 适用场景：支持 `macvtap`（`macvlan`）内核模块的 Linux 宿主机（虚拟化宿主、物理机），追求更好性能的二层桥接。注意：macvlan 的 vepa / private 等模式会限制接口间或与宿主机互访，且部分容器 / 受限环境无法加载该模块；此类情况改回 tap。
- 配置要点：主网卡开启混杂模式；关闭 NAT（`global_nat=false`）；需内核加载 `macvtap` 模块。

### ipvtap 模式（桥接 / 三层，内核态）

基于内核 `ipvtap`（`ipvlan`）模块，是 `macvtap` 的三层变体：虚拟网卡同样挂在主网卡上，但工作在三层、多个客户端共享主网卡 MAC 地址。客户端获得内网真实 IP，由内核直接路由桥接，性能与 macvtap 相当。

- 适用场景：需要 macvtap 的桥接能力，但主网卡所在网络对 MAC 地址数量敏感（例如云厂商对网卡 MAC 加白、交换机端口安全限制 MAC 数）的场景；客户端数量多、不希望每个客户端占用一个独立 MAC 时尤为合适。
- 配置要点：主网卡开启混杂模式；关闭 NAT（`global_nat=false`）；需内核加载 `ipvtap`（`ipvlan`）模块。与 macvtap 一样，在容器 / 受限环境可能无法加载该模块。

> 桥接模式（tun + proxy_arp / tap / macvtap / ipvtap）在云环境通常不支持，请使用 tun 默认 NAT 隧道模式。

## IPv6 双栈

RemLink 0.17.1 起支持 IPv4 / IPv6 双栈：客户端连接后可同时获得 IPv4 与 IPv6 地址，既能访问内网，也能访问公网 / 内网的 IPv6 资源。双栈叠加在任意网络模式（tun / tap / macvtap / ipvtap）之上，**无需改动你现有的 IPv4 配置**。

### 如何开启

在管理后台「软件配置 → 虚拟网络」中填写 **IPv6 地址段（ipv6_cidr）**，保存并重启服务即可。该字段**留空时，RemLink 行为与旧版本完全一致（仅 IPv4）**，不影响现有配置，可随时开启或关闭。

> 开启后，每个客户端会分配到一个独立的 IPv6 地址（网关为地址段网络地址 +1），连接后即可同时走 IPv4 / IPv6。

### IPv4 与 IPv6 的 NAT 各自独立

- **IPv4 全局 NAT（global_nat，默认开）**：控制 IPv4 是否走服务端公网 IP 出网（MASQUERADE）。这是传统默认行为，一般保持开启。
- **IPv6 全局 NAT / NAT66（global_nat6，默认开）**：**仅作用于 IPv6**。关闭它即可让 IPv6 走「纯路由模式」，且**不影响 IPv4 的 NAT**——两者可以分开配置。

> 也就是说，你可以「IPv4 继续 NAT 出网 + IPv6 纯路由」，只需关掉 IPv6 的全局 NAT，而不必像以前那样把 IPv4 也一起关掉。

### 两种出网方式（由「IPv6 全局 NAT」开关决定）

- **NAT 模式（默认，推荐）**：客户端 IPv6 流量经服务端转发后，以其公网 IPv6 地址出网。只要服务器本身具备公网 IPv6，即可直接使用，**无需任何上游配合**，最省心。
- **纯路由模式（关闭「IPv6 全局 NAT」）**：客户端使用真实的公网 IPv6 地址直接出网。前提是你所在的运营商 / 网络上游已把该 IPv6 地址段**路由指回 RemLink 服务器**（例如家庭宽带申请到的 IPv6 前缀委派）；否则外部无法回包。

> 不确定用哪种时，**保持默认的 NAT 模式即可**，任意合法的 IPv6 地址段都能正常出网。

### 地址段填写建议

- **本地服务器 / 家庭宽带（拿到运营商 IPv6 前缀委派）**：可使用前缀内一个未被内网占用的 /64 段作为地址池。
- **VPS（通常只有一个与其它机器共享的 /64）**：建议填写一段「站点本地地址（ULA，如 `fd00:c0de::/64`，任意以 `fd` 开头的 /64 均可）」并走默认的 NAT 模式。客户端即可经服务器公网 IPv6 访问公网，无需上游配合。
  - 说明：ULA 地址本身公网不可路由，只有在开启 NAT（默认）时才经服务器出公网；若关闭 NAT 走纯路由，ULA 地址只能在内网互通，无法访问公网（属预期行为）。

### 前置条件

- 服务器出网网卡需具备公网 IPv6 地址（执行 `ip -6 addr` 应能看到非 `fe80::` 开头的 global 地址）。
- 开启双栈后，RemLink 会**自动开启系统的 IPv6 转发**，无需手动设置。

## Web 代理（WebVPN）

RemLink 0.18.0 起内置 Web 代理：把内网 Web 应用（OA、Wiki、内部管理平台等）通过独立子域名安全发布到公网，**用户无需安装任何客户端、直接用浏览器访问**。它工作在七层（HTTP/HTTPS）反向代理模式，与 VPN 隧道完全独立——VPN 解决「设备入网」，Web 代理解决「单应用精细化发布」。

> 它与传统 VPN 不是一回事：VPN 给客户端下发整网访问权限；Web 代理只发布你指定的 Web 应用，并按身份精细控制谁能访问，不下发任何网络层权限。

### 如何开启

1. 在「证书管理」申请一张 **WebVPN 泛域名证书**（如 `*.app.example.com`，支持 Let's Encrypt 自动 DNS 验证或上传自定义证书）。该证书使用独立槽位，与 VPN 主证书互不覆盖。
2. 在「软件配置」中填写 **Web 代理域名**（如 `app.example.com`）。
3. 在域名控制台添加通配符 DNS 解析：`*.app.example.com` → 服务器公网 IP。
4. 配置即时生效，无需重启服务。

> 域名留空 = 功能关闭，RemLink 行为与旧版本完全一致，不影响现有配置，可随时开启或关闭。

### 发布一个应用

在「WebVPN → 应用」中添加：

- **子域名**：应用前缀，如 `oa` → 访问地址 `oa.app.example.com`
- **后端地址**：内网真实地址，如 `http://192.168.1.10:8080`
- **授权用户 / 用户组**：留空 = 所有已登录用户可访问；填写后按白名单限制（两者同时填写时为「交集」关系，需同时满足）
- **允许路径 / 来源 IP**：可选进一步限制可访问的路径前缀与客户端公网 IP

### 访问与认证

- 用户浏览器访问 `oa.app.example.com` → 未登录自动跳转到门户登录页 → 登录后自动回跳到应用。
- 已登录门户的用户可在「我的应用」中一键跳转，无需重复登录。
- 每次访问都按最新授权实时校验；管理员可在审计页一键踢出某用户，下次请求即失效。

### 访问审计

「WebVPN → 审计」记录每个用户的访问时间、应用、路径、HTTP 状态码、收发流量、风险等级，支持按用户 / 应用 / 时间筛选并导出 CSV。

### 前置条件

- 一个可用于泛域名解析的域名（需能添加 `*.xxx` 的通配符 DNS 记录）。
- Web 代理泛域名证书（独立槽位，与 VPN 主证书互不干扰）。
- RemLink 服务器能以内网地址访问到目标业务系统。

> 若业务系统放在反向代理 / CDN 之后，请确保其正确透传 Host 与访问协议，否则可能出现跳转异常或样式错乱。

## 客户端

| 客户端                                                   | 平台                                    |
| -------------------------------------------------------- | --------------------------------------- |
| [AnyConnect Secure Client](https://www.cisco.com/)        | Windows / macOS / Linux / Android / iOS |
| [OpenConnect](https://gitlab.com/openconnect/openconnect) | Windows / macOS / Linux                 |
| [三方客户端下载（推荐）](https://cisco.yydy.link/)        | 全平台                                  |

## 镜像加速

```bash
# DockerHub
docker pull wsczx/remlink:latest

# 阿里云（国内加速）
docker pull registry.cn-hangzhou.aliyuncs.com/wsczx/remlink:latest

# 镜像代理
docker pull docker.1ms.run/wsczx/remlink:latest
```

## 在线升级

管理后台「系统设置」→ 点击「检查更新」→ 有新版本时点击「立即升级」，自动下载、替换二进制、进程内重启，全程可视化进度。

> 升级前建议备份 `conf` 目录和数据库。

## 界面截图

<details>
<summary>点击展开查看界面截图</summary>

### 管理后台

![后台管理登录页](doc/screenshot/后台管理登录页.png)
![管理员安全设置](doc/screenshot/管理员安全设置.png)
![用户组配置](doc/screenshot/用户组配置.png)
![软件配置](doc/screenshot/软件配置.png)

### 用户门户

![用户门户登录页](doc/screenshot/用户门户登录页.png)
![门户首页](doc/screenshot/门户首页.png)
![自定义门户](doc/screenshot/自定义门户.png)
![自定义品牌](doc/screenshot/自定义品牌.png)

</details>

## 开发与构建

### 开发环境

- Linux 环境及 tun/tap、网络管理和防火墙相关内核能力
- Docker
- GNU Make

默认构建流程在 Docker 容器中完成，因此不要求宿主机安装 Go、Node.js 或 Yarn。只有使用 `make local`、`make go`、`make test`，或直接进行前端开发时，才需要在宿主机安装相应工具。项目使用 Yarn 管理前端依赖，不提交 `web/package-lock.json`。构建命令：

```bash
git clone https://github.com/wsczx/RemLink.git
cd RemLink

make help          # 查看所有命令
make web           # 构建前端并嵌入服务端
make local         # 本地编译二进制，产物位于 server/remlink
make build         # 使用 Docker 构建前端并构建 Docker 镜像
make docker        # 构建 Docker 镜像（需先完成前端构建）
make release       # 构建并提取二进制发布包，同时发布到 GitHub Release
```

前端构建建议通过 `make web` 或 Docker 执行，避免宿主机 Node.js 与旧版构建链的 OpenSSL 兼容问题。完整构建产物位于 `artifact-dist/`。

### 项目结构

```text
server/       Go 服务端、认证、VPN 协议和 Web API
web/          Vue 管理后台与用户门户
deploy/       systemd、Docker Compose、Kubernetes 和安装脚本
docker/       Docker 构建文件
scripts/      构建、发布和开发辅助脚本
doc/          用户文档、常见问题和截图
```

### 提交与贡献

欢迎提交 Issue、Pull Request 和文档改进。提交前请确认没有将数据库、环境变量文件、构建产物或密钥加入 Git，并完成必要的测试和文档更新。

## 常见问题

请前往 [常见问题文档](doc/question.md) 查看。

## 支持与打赏

如果您觉得 RemLink 对您有帮助，欢迎打赏支持项目持续开发。请[点击这里](doc/screenshot/shoukuanma.png)查看打赏二维码。

[打赏列表](doc/sponsors.md)

## License

RemLink 服务端及项目中未另行声明的部分采用 [GNU Affero General Public License v3.0](LICENSE)（AGPL-3.0）授权。前端代码沿用上游 AnyLink 的 [MIT License](https://github.com/bjdgyc/anylink/blob/main/web/LICENSE)，第三方依赖按照各自许可证授权。

RemLink 基于 [AnyLink](https://github.com/bjdgyc/anylink) 开发，感谢原作者及所有贡献者的开源贡献。使用、修改或分发 RemLink 时，请遵守相应许可证及第三方组件的授权条款。
