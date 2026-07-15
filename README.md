# RemLink

![GitHub release](https://img.shields.io/github/v/release/wsczx/RemLink)
![GitHub downloads](https://img.shields.io/github/downloads/wsczx/RemLink/total)
[![Docker pulls](https://img.shields.io/docker/pulls/wsczx/remlink.svg)](https://hub.docker.com/r/wsczx/remlink)

RemLink 是一个企业级远程办公 SSL VPN 软件，可以支持多人同时在线使用。

使用 RemLink，你可以随时随地安全的访问你的内部网络。

> **声明**：RemLink 基于 [AnyLink](https://github.com/bjdgyc/anylink) 深度重构，在原项目基础上进行了认证架构重写、前端重构、安全加固与大量功能增强。感谢原作者 [bjdgyc](https://github.com/bjdgyc) 的开源贡献。

## 简介

RemLink 基于 [ietf-openconnect](https://tools.ietf.org/html/draft-mavrogiannopoulos-openconnect-02) 协议开发，并且借鉴了 [ocserv](http://ocserv.gitlab.io/www/index.html) 的开发思路，使其可以同时兼容 AnyConnect 客户端。

RemLink 使用 TLS/DTLS 进行数据加密，因此需要 RSA 或 ECC 证书，可以使用私有自签证书，可以通过 Let's Encrypt 和 TrustAsia 申请免费的 SSL 证书。

## 下载

从 [Releases](https://github.com/wsczx/RemLink/releases) 页面下载对应平台的 `remlink-deploy.tar.gz`，解压后即可使用。

## 功能特性

### 网络与基础设施
- IP 分配（实现 IP、MAC 映射信息的持久化）
- TLS-TCP 通道 / DTLS-UDP 通道
- 兼容 AnyConnect / OpenConnect
- 基于 tun 设备的 nat 访问模式
- 基于 tun / macvtap 设备的桥接访问模式
- 支持 proxy protocol v1&v2
- nftables 后端（优先使用，自动回退 iptables）
- FakeDNS + FakeIP（域名规则匹配 + DNS 缓存加速）
- 流量压缩、出口 IP 自动放行、空闲链接超时自动断开、流量速率限制

### 认证体系
- 本地密码认证（bcrypt）
- TOTP 动态码认证
- 客户端证书认证（支持绑定设备）
- LDAP/AD 认证
- RADIUS 认证（含 Access-Challenge 二次验证）
- 企业微信 OAuth2 扫码登录
- 飞书 OAuth2 扫码登录
- SMS 短信验证码（腾讯云 + 阿里云）
- WebAuth 浏览器端证书认证
- 认证 Pipeline 可编排架构（多步骤自由组合 + 断点恢复）
- Provider 统一管理第三方认证配置
- 登录防爆（用户+IP 三级锁定策略）
- 自动同步 LDAP / 企微 / 飞书用户

### 用户门户
- 客户端下载页面
- 证书自助申请与下载
- 在线设备管理与踢下线
- 密码自助重置
- OTP 动态码绑定

### 管理与运维
- Web 管理后台
- 用户 / 组 / 策略管理
- 用户活动审计日志
- IP 访问审计（支持多端口、连续端口）
- 管理员操作审计日志
- 系统日志实时推送（WebSocket）
- 数据库在线切换（SQLite / MySQL / PostgreSQL / MSSQL）
- 数据备份与还原
- 在线升级
- 配置全面数据库化（无需手动编辑配置文件）
- 敏感字段 AES-256-GCM 加密存储 + API 脱敏
- 自适应响应式前端界面
- 支持 Docker 非特权模式

## 快速开始

### 二进制部署

1. 从 [Releases](https://github.com/wsczx/RemLink/releases) 下载 `remlink-deploy.tar.gz`
2. 解压并运行：

```bash
tar xzf remlink-deploy.tar.gz
cd remlink-deploy
sudo ./remlink
```

3. 浏览器访问管理后台 `https://<服务器IP>:8800`
4. 首次启动会在日志中打印随机管理员密码，登录后请立即修改

### Docker 部署

```bash
# 特权模式（简单）
docker run -itd --name remlink --privileged \
    -p 443:443 -p 8800:8800 -p 443:443/udp \
    -v /home/myconf:/app/conf \
    --restart=always \
    wsczx/remlink

# 非特权模式（推荐）
docker run -itd --name remlink \
    -p 443:443 -p 8800:8800 -p 443:443/udp \
    -v /dev/net/tun:/dev/net/tun --cap-add=NET_ADMIN \
    -v /home/myconf:/app/conf \
    --restart=always \
    wsczx/remlink
```

### Docker Compose

```yaml
version: '3'
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

## 管理命令

```bash
# 查看帮助
./remlink -h

# 查看所有配置项
./remlink tool -d

# 重置管理员密码（需先停止服务）
pkill remlink && ./remlink --reset-admin-password && ./remlink

# 强制禁用管理员两步验证（OTP 密钥丢失时使用，需先停止服务）
pkill remlink && ./remlink --disable-admin-otp && ./remlink
```

## 网络模式

### tun 模式（推荐）

```bash
# 开启 IP 转发
sysctl -w net.ipv4.ip_forward=1

# 服务端自动设置 NAT 转发，无需手动配置 iptables
```

### macvtap 桥接模式

```bash
# 主网卡开启混杂模式
ip link set dev eth0 promisc on
```

在管理后台「软件配置」页面设置 `link_mode`、`ipv4_master`、`ipv4_cidr` 等参数。

## 客户端

- [AnyConnect Secure Client](https://www.cisco.com/) (Windows/macOS/Linux/Android/iOS)
- [OpenConnect](https://gitlab.com/openconnect/openconnect) (Windows/macOS/Linux)
- [【推荐】三方客户端下载地址](https://cisco.yydy.link/) (Windows/macOS/Linux/Android/iOS)

## 常见问题

请前往 [常见问题文档](https://github.com/wsczx/RemLink/wiki) 查看。

## 支持与打赏

如果您觉得 RemLink 对您有帮助，欢迎打赏支持项目持续开发。

<p>
    <img src="https://img.wsczx.com/weixinshoukuan.jpg" width="400" />
</p>

## License

RemLink 为闭源商业软件，采用专属最终用户许可协议（EULA）。未经授权，不得复制、修改、反向工程或再分发本软件及其任何部分。
