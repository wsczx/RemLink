# WebVPN 远程协议部署说明（SMB / FTP / RDP / VNC）

WebVPN 除 HTTP 反向代理外，还支持在浏览器中直接访问内网的非 HTTP 服务：

| 协议 | 能力 | 后端实现 | 默认端口 |
| --- | --- | --- | --- |
| ssh | 终端 + 文件 | 内置（x/crypto/ssh + SFTP） | 22 |
| sftp | 文件 | 内置 | 22 |
| smb | 文件 | 内置（go-smb2，SMB 2.0.2 ~ 3.1.1，NTLM） | 445 |
| ftp | 文件 | 内置（FTP / FTPS 显式 / FTPS 隐式，仅被动模式） | 21（隐式 TLS 为 990） |
| rdp | 远程桌面 | **guacd sidecar** | 3389 |
| vnc | 远程桌面 | **guacd sidecar** | 5900 |

目标地址只能由管理员在应用配置中指定，用户侧无法指定（防 SSRF）。所有远程会话都受同一套安全机制约束：
严格 Origin 校验、一次性 WebSocket 票据（30 秒、绑定用户/应用/IP）、文件根目录限定（Jail）、在线会话注册表
（管理员踢下线、吊销、应用变更时立即断开）、空闲超时、单用户并发上限、认证失败限流、文件操作日志。

## 一、SMB / FTP

无需额外组件，在后台新建应用时选择协议并填写扩展参数（`options`，JSON）：

### SMB

```json
{"share": "data", "root": "/projects", "domain": "CORP"}
```

- `share`（必填）：共享名，如 `data`、`C$`；不能包含 `\ / : * ? " < > |`。
- `root`（可选）：共享内的浏览根目录，默认为共享根。
- `domain`（可选）：NTLM 域；为空时若账号写成 `CORP\alice` 会自动拆分。
- 凭据模式支持 prompt / stored / passthrough（passthrough 适合 AD 域账号与门户账号一致的场景）。
- 符号链接 / 重解析点（含目录联接 junction）**一律不跟随**：经过它们的路径都会被拒绝，目录打包下载时跳过。
  注意：Windows 重复数据删除（Dedup）、OneDrive 占位文件等也是重解析点，同样无法下载。

### FTP / FTPS

```json
{"tls": "explicit", "encoding": "gbk", "root": "/pub", "skip_verify": false, "passive": true}
```

- `tls`：`none`（默认）/ `explicit`（AUTH TLS）/ `implicit`（端口默认 990）。
- `encoding`：`utf-8`（默认）/ `gbk`（老旧 Windows/国产 FTP 服务端的中文文件名）。
- `root`：浏览根目录；为空时取登录后的初始目录（PWD）。
- `skip_verify`：FTPS 跳过证书校验（自签证书时使用）。
- `passive`：仅支持被动模式，必须为 `true` 或省略。数据连接始终连接控制连接的对端 IP（忽略 PASV 返回的 IP，防 FTP bounce）。
- 每个会话最多 4 条并发控制连接（连接池），下载/上传/列目录可并发；断点续传使用 `REST`+`STOR`，服务端不支持时自动回退 `APPE`。

## 二、RDP / VNC（guacd）

远程桌面通过 Apache Guacamole 的 **guacd** 代理实现，RemLink 网关替代 guacamole-client（Java）完成握手并转发
Guacamole 协议，浏览器端使用 guacamole-common-js：

```
浏览器(guacamole-common-js) ⇄ wss://<应用>.<webvpn_domain>/webvpn/ws/desktop ⇄ RemLink 网关 ⇄ guacd:4822 ⇄ RDP/VNC 目标
```

连接参数（目标地址、凭据、安全模式、剪贴板、录像）全部由网关根据管理员配置生成，浏览器无法改写；
握手后网关还会二次过滤剪贴板流，并拦截 `select` / `connect` / 非凭据类 `argv` 等指令。

### 1. 配置项

在管理后台「系统设置 → 门户设置」或启动参数/环境变量中配置：

| 配置项 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `guacd_addr` | `LINK_GUACD_ADDR` | `127.0.0.1:4822` | guacd 地址；**留空表示不启用远程桌面**，此时建立 RDP/VNC 会话返回错误码 `guacd_unavailable` |
| `guacd_record_path` | `LINK_GUACD_RECORD_PATH` | 空 | guacd 视角下的录像目录（即 RemLink 的 `./conf/webvpn_record` 在 guacd 容器内的挂载点，如 `/record`）；为空时使用 RemLink 录像目录的绝对路径（guacd 与 RemLink 同机、同文件系统时适用） |

### 2. Docker Compose 部署

示例见 [`docker/guacd/docker-compose.yml`](../docker/guacd/docker-compose.yml)：

```bash
cd docker/guacd
mkdir -p conf/webvpn_record
# guacamole/guacd 镜像以 uid 1000 运行，需能写入录像目录
chown 1000:1000 conf/webvpn_record && chmod 0770 conf/webvpn_record
docker compose up -d
```

要点：

- guacd **不发布任何端口**，只在 compose 内部网络中被 RemLink 以 `guacd:4822` 访问。guacd 本身没有认证，
  切勿把 4822 暴露到公网或办公网。
- RemLink 使用 host 网络时，guacd 只发布到回环地址：`ports: ["127.0.0.1:4822:4822"]`，`guacd_addr` 保持默认。
- guacd 容器需要能路由到 RDP/VNC 目标主机所在网段。
- 已有 RemLink 容器时，只需按示例加入 `guacd` 服务、共享网络与录像卷，并设置上述两个配置项。

### 3. 二进制部署

在同一台主机上安装 guacd（发行版仓库的 `guacd` 包，或编译 guacamole-server，需启用 FreeRDP 与 libvncserver），
让其仅监听回环地址：

```bash
guacd -b 127.0.0.1 -l 4822
```

`guacd_addr` 保持默认 `127.0.0.1:4822`，`guacd_record_path` 留空即可（guacd 直接写 RemLink 的录像目录，
需保证 guacd 运行用户对 `conf/webvpn_record` 有写权限）。

### 4. 应用扩展参数

RDP：

```json
{"security": "nla", "ignore_cert": true, "domain": "CORP"}
```

- `security`：`any`（默认，自动协商）/ `nla` / `nla-ext` / `tls` / `vmconnect` / `rdp`。
- `ignore_cert`：忽略目标主机证书（自签证书的 Windows 主机通常需要开启）。
- `domain`：Windows 域；为空时可在账号中写 `CORP\alice`。

VNC：无必填扩展参数；VNC 多为仅密码认证，账号与密码都可以为空（托管凭据模式也可不配置账号）。

应用的权限开关在远程桌面中的含义：

- `allow_clipboard`：关闭时 guacd 侧设置 `disable-copy` / `disable-paste`，网关同时丢弃双向的剪贴板流。
- 文件传输（RDP 驱动器重定向、guacd SFTP）当前一律关闭，`allow_upload` / `allow_download` 对桌面无效。
- `record`：开启后 guacd 录制 Guacamole 会话录像（不含键盘输入）。

### 5. 录像

- 路径：`conf/webvpn_record/<YYYYMMDD>/<sid>.guac`；同一会话断线重连产生的后续录像为 `<sid>.2.guac`、`<sid>.3.guac`…
- 回放 / 下载：管理后台「审计日志 → WebVPN 访问」，点击会话在详情抽屉中回放或下载（接口 `GET /api/v1/webvpn/remote/recording?sid=&part=N`，`part` 指定第 N 段，缺省为第 1 段）。
  响应头 `X-Recording-Format: guac`。
- 回放：
  - 浏览器内可用 guacamole-common-js 的 `Guacamole.SessionRecording` 直接回放 `.guac` 文件；
  - 或用 guacd 镜像内的 `guacenc` 转码为视频：

    ```bash
    docker run --rm -v $PWD/conf/webvpn_record:/record --entrypoint /opt/guacamole/bin/guacenc \
        guacamole/guacd:1.5.5 -s 1280x720 -r 2000000 /record/20261005/<sid>.guac
    # 生成 /record/20261005/<sid>.guac.m4v
    ```

    （不同镜像版本中 guacenc 路径可能不同；也可使用自行编译并启用 libavcodec 的 guacamole-server。）
- 权限：网关在开始录制前会把当天的日期目录设为 `0733 + sticky`，以便非 root 的 guacd 写入（不可列目录、不可删除他人文件）。

### 6. 故障排查

| 现象 | 排查 |
| --- | --- |
| 建会话返回 `guacd_unavailable` | `guacd_addr` 为空或 RemLink 无法连接 guacd；检查容器网络 / `docker logs remlink-guacd` |
| 桌面连接后立即断开，原因 `auth_failed` | 账号/密码/域错误，或 NLA 要求与 `security` 不符；该错误会计入认证失败限流（5 次/5 分钟） |
| 原因 `target_unreachable` | guacd 无法连接目标主机/端口，或 `security` 与目标不兼容（可先试 `any` + `ignore_cert`） |
| 录像文件不存在 | guacd 无录像目录写权限，或 `guacd_record_path` 与实际挂载点不一致；查看 guacd 日志中的 recording 报错 |
