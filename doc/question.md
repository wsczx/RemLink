## 常见问题

### AnyConnect 客户端问题

> 请使用官方 AnyConnect / OpenConnect 客户端，其他版本未经充分测试，不保证可用性。

### OTP 动态码

> 请使用手机安装 freeotp ，然后扫描otp二维码，生成的数字即是动态码

### 企业微信 / 飞书扫码登录

> 需要在管理后台「认证提供方」页面创建对应的 Provider（填写 AppID、Secret 等信息）
>
> 然后在用户组的「认证配置」中添加 `wxwork` 或 `feishu` 步骤即可
>
> 配置方法详见 [server/auth/README.md](../server/auth/README.md)

### RADIUS Access-Challenge 二次验证

> RemLink 支持 RADIUS Access-Challenge 协议，服务端会下发挑战信息给客户端
>
> 适用于需要短信验证码、硬件 Token 等二次验证场景
>
> 在认证 Pipeline 中添加 `radius` 步骤即可自动处理 Challenge 流程

### 认证 Pipeline 编排

> 用户组的认证流程由多个「步骤」按序组合，例如 `[local, otp]` 表示先本地密码、再 TOTP 动态码
>
> 每个步骤通过才继续下一步，任意步骤失败则认证终止
>
> 支持的步骤类型：`local`（本地密码）、`ldap`、`radius`、`cert`（客户端证书）、`otp`（TOTP）、`wxwork`（企微）、`feishu`（飞书）

### 用户策略问题

> 只要有用户策略，组策略就不生效，相当于覆盖了组策略的配置

### 远程桌面连接

> 本软件已经支持远程桌面里面连接anyconnect。

### 私有证书问题

> remlink 默认不支持私有证书
>
> 其他使用私有证书的问题，请自行解决

### 客户端连接名称

> 客户端连接名称在管理后台「软件配置」→「Profile 配置」中在线修改

```xml

<HostEntry>
    <HostName>VPN</HostName>
    <HostAddress>localhost</HostAddress>
</HostEntry>
```

### dpd timeout 设置问题

```yaml
#客户端失效检测时间(秒) dpd > keepalive
cstp_keepalive = 4
cstp_dpd = 9
mobile_keepalive = 7
mobile_dpd = 15
```

> 以上dpd参数为客户端的超时检测时间, 如一段时间内，没有数据传输，防火墙会主动关闭连接
>
> 如经常出现 timeout 的错误信息，应根据当前防火墙的设置，适当减小dpd数值

### 关于审计日志 audit_interval 参数

> 默认值 `audit_interval = 600` 表示相同日志600秒内只记录一次，不同日志首次出现立即记录
>
> 去重key的格式: 16字节源IP地址 + 16字节目的IP地址 + 2字节目的端口 + 1字节协议类型 + 16字节域名MD5

### 反向代理问题

> remlink 仅支持四层反向代理，不支持七层反向代理
>
> 如Nginx请使用 stream模块

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

> nginx实现 共用443端口 示例

```conf
stream {
    map $ssl_preread_server_name $name {
        vpn.xx.com        myvpn;
        default     defaultpage;
    }
    
    # upstream pool
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

### 性能问题

```
内网环境测试数据
虚拟服务器：  centos7 4C8G
remlink:    tun模式 tcp传输
客户端文件下载速度：240Mb/s
客户端网卡下载速度：270Mb/s
服务端网卡上传速度：280Mb/s
```

> 客户端tls加密协议、隧道header头都会占用一定带宽


### 登录防爆说明

```
1.用户 A 在 IP 1.2.3.4 上尝试登录:
  用户 A 在 IP 1.2.3.4 上尝试登录失败 5 次，触发了该 IP 上的用户 A 锁定 5 分钟。
  在这 5 分钟内，用户 A 从 IP 1.2.3.4 无法进行新的登录尝试。
2.用户 A 更换 IP 到 1.2.3.5 继续尝试登录:
  用户 A 在 IP 1.2.3.5 上继续尝试登录，并且累计失败 20 次，触发了全局用户 A 锁定 5 分钟。
  在这 5 分钟内，用户 A 从任何 IP 地址都无法进行新的登录尝试。
3.IP 1.2.3.4 上多个用户尝试登录:
  如果从 IP 1.2.3.4 上累计有 40 次失败登录尝试（无论来自多少不同的用户），触发了该 IP 的全局锁定 5 分钟。
  在这 5 分钟内，从 IP 1.2.3.4 的所有登录尝试都将被拒绝。

如果在 N 分钟内没有新的失败尝试，失败计数会在 N 分钟后（*_reset_time）重置。
```

### UFW / firewalld 兼容问题

> RemLink 支持 nftables 和 iptables 两种防火墙后端，UFW 冲突**仅在 nftables 后端下发生**。

**如何确认当前后端**：

```shell
grep "Firewall driver" /var/log/remlink.log
# "using nftables" → 可能与 UFW 冲突
# "using iptables"  → 不会与 UFW 冲突
```

**为什么 iptables 后端不冲突**：iptables 的 `ACCEPT` 是全局终止，匹配后不再进入 UFW 的 FORWARD 链。

**为什么 nftables 后端冲突**：nftables 的 `accept` 只终止当前 base chain，包继续进入 UFW 的 FORWARD 链（priority=0）执行 DROP。

**症状**：VPN 连接成功但无法访问外网，`ufw disable` 后恢复正常。

**根因**：nftables 的 `accept` 只终止当前 base chain，不阻止同 hook 上其他 base chain 的处理。UFW 的 FORWARD 链（priority=0）会在 RemLink 的链之后执行 DROP 策略。

**解决方案**：

1. **放行全局 VPN 网段**（默认 `192.168.90.0/24`，以实际配置为准）：

```shell
# UFW
ufw route allow from 192.168.90.0/24
ufw reload

# firewalld
firewall-cmd --permanent --add-rich-rule='rule source address="192.168.90.0/24" accept'
firewall-cmd --reload
```

2. **组级别独立网段也需要单独放行**（在用户组配置了独立 IP 段时）：

```shell
ufw route allow from 10.0.1.0/24
ufw reload
```

3. **启用 FakeDNS 时需放行 FakeIP 网段**（默认 `100.64.0.0/10`）：

```shell
ufw route allow from 100.64.0.0/10
ufw reload
```

4. **或者关闭系统防火墙**（不推荐，仅测试用）：

```shell
systemctl stop ufw
systemctl disable ufw
```

> 添加 UFW 规则后无需重启 RemLink，执行 `ufw reload` 即可立即生效。
> 如果使用 firewalld，`firewall-cmd --reload` 同理。