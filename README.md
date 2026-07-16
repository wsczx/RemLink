# RemLink

![GitHub release](https://img.shields.io/github/v/release/wsczx/RemLink)
![GitHub downloads](https://img.shields.io/github/downloads/wsczx/RemLink/total)
[![Docker pulls](https://img.shields.io/docker/pulls/wsczx/remlink.svg)](https://hub.docker.com/r/wsczx/remlink)

RemLink 是一个企业级远程办公软件，支持多人同时在线，兼容 AnyConnect(推荐) / OpenConnect 客户端。

> **声明**：RemLink 基于 [AnyLink](https://github.com/bjdgyc/anylink) 深度重构，在原项目基础上进行了认证架构重写、前端重构、安全加固与大量功能增强。感谢原作者 [bjdgyc](https://github.com/bjdgyc) 的开源贡献。

## 简介

RemLink 基于 [ietf-openconnect](https://tools.ietf.org/html/draft-mavrogiannopoulos-openconnect-02) 协议开发，借鉴 [ocserv](http://ocserv.gitlab.io/www/index.html) 思路，同时兼容 AnyConnect 客户端。使用 TLS/DTLS 进行数据加密，支持 RSA 或 ECC 证书（自签证书 / Let's Encrypt / TrustAsia）。

## 端口说明

| 端口       | 用途                    |
| ---------- | ----------------------- |
| 443 (TCP)  | VPN 连接（TLS-TCP）     |
| 443 (UDP)  | VPN 连接（DTLS-UDP）    |
| 8800 (TCP) | 管理后台 Web 界面 + API |

> 管理后台访问地址：`https://<IP>:8800`
> VPN 连接地址：`<域名或IP>:443`

## 下载

从 [Releases](https://github.com/wsczx/RemLink/releases) 下载对应平台的 `remlink-deploy.tar.gz`。

## 功能特性

<details>
<summary>点击展开完整功能列表</summary>

### 网络与基础设施

- IP 分配（IP、MAC 映射持久化）
- TLS-TCP 通道 / DTLS-UDP 通道
- 兼容 AnyConnect / OpenConnect 客户端
- tun 设备 NAT 模式 / tun/macvtap 设备桥接模式
- 支持 proxy protocol v1 & v2
- nftables/iptables 自动配置
- 流量压缩（LZS）、出口 IP 自动放行
- 空闲链接超时自动断开、流量速率限制
- 组级别独立 IP 池与 NAT 规则
- 内置 Let's Encrypt / TrustAsia ACME 证书自动申请

### 认证体系

- 本地密码认证
- TOTP 动态码认证
- 客户端证书认证（支持设备绑定、CSR 模式）
- LDAP / AD 认证
- RADIUS 认证（含 Access-Challenge 二次验证）
- 企业微信 OAuth2 扫码登录
- 飞书 OAuth2 扫码登录
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
tar xzf remlink-deploy.tar.gz
cd remlink-deploy
sudo ./remlink
```

### Systemd 服务

```bash
sudo cp remlink-deploy /usr/local/remlink-deploy -r
sudo chmod +x /usr/local/remlink-deploy/remlink
sudo cp deploy/remlink.service /usr/lib/systemd/system/  # CentOS
# Ubuntu: /lib/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now remlink
```

## 首次使用

1. 启动后查看日志获取随机生成的管理员密码
2. 浏览器访问 `https://<IP>:8800` 登录管理后台
3. 进入「系统设置 > 安全设置」修改管理员密码
4. 在「软件配置」中设置 `link_mode`（tun/macvtap）和网络参数
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

切换方式：管理后台「软件配置」→ 数据库「切换」按钮（支持自动数据迁移）。也可通过 `--db_type` 和 `--db_source` 命令行参数或 `conf/db.json` 首次启动时指定。

## 网络模式

### tun 模式（推荐）

客户端传输 IP 层数据，性能最佳。服务端自动设置 IP 转发和 NAT。

### 桥接模式（macvtap / arp_proxy）

客户端获得内网真实 IP，需主网卡开启混杂模式：

```bash
ip link set dev eth0 promisc on
```

> 云环境不支持桥接模式，请使用 tun 模式。

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

## 常见问题

请前往 [常见问题文档](doc/question.md) 查看。

## 支持与打赏

如果您觉得 RemLink 对您有帮助，欢迎打赏支持项目持续开发。请[点击这里](.github/FUNDING.md)查看打赏二维码。

## License

RemLink 为闭源商业软件，采用专属最终用户许可协议（EULA）。未经授权，不得复制、修改、反向工程或再分发本软件及其任何部分。完整条款见 [LICENSE](LICENSE)。
