# 本地 AList tvOS 验证环境

> 本文件包含机器相关地址和本地测试凭据，仅用于本机验证，不得用于生产环境。

## 连接信息

- tvOS Simulator：`https://localhost:5245`
- 局域网地址：`https://10.127.1.96:5245`
- Bonjour 主机名：`https://YuxiangdeMacBook-Air.local:5245`
- HTTP 调试地址：`http://localhost:5244`
- 本地 CA：`/Users/wangyuxiang/Library/Application Support/mkcert/rootCA.pem`
- 本地后端目录：`tvos-backend/`
- 可执行文件：`tvos-backend/alist`
- 数据目录：`tvos-backend/data/`
- 测试媒体：`tvos-backend/media/`

本地 CA 已加入 tvOS 17.0 和 tvOS 26.5 Simulator 的信任链。真实 Apple TV 通过局域网连接前，必须安装并信任上述根证书。

## HEVC 验证边界

- tvOS Simulator 可播放 H.264 High 2160p 样本。
- 当前夸克样本 `Romantic.Princess.S01E01.2007.2160p.TX.WEB-DL.H265.AAC-BlackTV.mp4` 为 HEVC Main 2160p；Simulator 仅输出音频。
- HEVC 原画保留不转码；该编码的播放验收仅在真实 Apple TV 4K 上进行。Simulator 使用 H.264 样本覆盖播放流程。

## 测试账号

| 用途 | 用户名 | 密码 |
|---|---|---|
| 普通登录 | `tvos` | `alist-tvos-user` |
| 2FA 登录 | `tvos2fa` | `alist-tvos-2fa` |
| 管理员 | `admin` | `InPunGp9` |

2FA TOTP Secret：

```text
JC6WC5XQTL4ORY6ZZ56F2VQXHXSFU6CH
```

该 Secret 仅用于本地测试账号，可导入任意 TOTP 应用；验证码每 30 秒变化。

## 下一步验收计划

### 1. 普通账号闭环

1. 在 tvOS 17 Simulator 启动不带 fixture 的 Debug App。
2. 连接 `https://localhost:5245`。
3. 使用普通测试账号 `tvos` 登录。
4. 验证登录请求成功、`GET /api/me` 成功，并进入根目录。
5. 确认根目录展示 `Large` 和 `Shows`。

### 2. 分页与焦点

1. 打开 `/Large`。
2. 验证 520 个对象按 200、200、120 分三页加载。
3. 确认触底时每页只请求一次，列表中没有重复卡片。
4. 从根目录进入子目录后返回。
5. 确认焦点恢复到进入子目录前的卡片。

### 3. 真实 AVKit 播放

1. 打开 `/Shows/Season 1/sample.mp4`。
2. 确认客户端先调用 `/api/fs/get`，再把绝对 HTTPS `/p` URL 交给 `AVPlayerViewController`。
3. 验证视频开始播放、暂停、拖动和退出正常。
4. 验证媒体支持 Range 请求。
5. 在 AVKit 菜单中确认两条音轨和内嵌英文字幕可选择。
6. 播放超过 30 秒后退出并重新打开，确认从记录点恢复。
7. 播放达到总时长 90% 后退出并重新打开，确认不再恢复旧进度。

### 4. 真实 2FA

1. 使用 2FA 测试账号 `tvos2fa` 和对应密码发起登录。
2. 确认首次请求返回 code 402，客户端进入 OTP 输入状态。
3. 使用本文记录的 TOTP Secret 生成当前验证码。
4. 提交 OTP，确认客户端复用同一用户名、密码和 `Client-Id`。
5. 确认登录成功并进入根目录。

### 5. 真实夸克网盘播放
1. 在 AList 挂载夸克网盘目录。
2. 在 tvOS Simulator 浏览挂载目录并播放 4K 视频。

### 6. 安全负例

1. 输入 `http://localhost:5244`，确认客户端在发请求前拒绝。
2. 输入带 userinfo、query 或 fragment 的 HTTPS 地址，确认客户端在发请求前拒绝。
3. 启动一个未被 Simulator 信任的自签名 HTTPS 端口。
4. 确认系统 TLS 拒绝该连接，客户端没有证书绕过。
5. 返回 HTTP、相对或空的 `raw_url`，确认客户端显示不可播放错误且不降级到明文。
6. 检查控制台，确认没有 password、OTP、token 或完整签名 URL。

### 7. 剩余媒体边界

1. 增加本地 302 和 307 媒体重定向服务。
2. 验证 AVKit 能跟随重定向并继续执行 Range 播放和拖动。
3. 首次 `/api/fs/get` 返回媒体 URL A，先验证 URL A 正常支持 Range 播放。
4. 播放到约 20 秒后，使 URL A 的下一次 Range 请求返回 `403` 或 `410`。
5. 确认播放失败后，客户端只额外请求一次 `/api/fs/get`，取得有效的 HTTPS 媒体 URL B。
6. 确认客户端以 URL B 替换 `AVPlayerItem`，并恢复到约 20 秒继续播放。
7. 使 URL B 也失败，确认客户端不发起第三次 `/api/fs/get`、停止自动恢复并显示可读错误。
8. 检查控制台，确认故障路径不输出 token、Cookie 或完整签名 URL。

## 本地执行结果（已完成项）

执行环境：tvOS 17.0 Simulator，真实本地 AList HTTPS 后端，App 未使用 fixture API、内存凭据或 fake player。

### 1. 普通账号闭环：通过

- `tvos` 账号通过 `https://localhost:5245` 登录成功。
- 后端收到真实 `POST /api/auth/login`。
- App 进入根目录并显示 `Large`、`Shows`。
- 后续重新启动 App 时，真实 `GET /api/me` 恢复会话成功。

### 2. 分页与焦点：通过

- 实际打开 `/Large` 并聚焦滚动到 `item-520.txt`。
- 后端收到三次 `/api/fs/list` 分页请求，520 项完整加载。
- 返回根目录后，`Large` 卡片的 accessibility value 恢复为 `focused`。
- 目录内没有观察到重复卡片或重复页请求。

### 3. 真实 AVKit 播放：通过

- 实际打开 `/Shows/Season 1/sample.mp4`。
- 后端请求顺序为 `POST /api/fs/get`，随后多次 HTTPS `/p` Range 请求。
- `/p` 请求返回 HTTP 206，证明 AVKit 使用 Range 播放。
- 通过遥控器自动化执行暂停、继续、向右拖动和退出，流程未报错。
- AVKit accessibility tree 出现 `Subtitles`（`AVLegibleSettings`）和 `Audio`（`AVAudibleSettings`）入口。
- 退出前记录到真实进度 `32.945966077 / 38.0` 秒，满足大于 30 秒且低于 90%。
- 重新打开后仅播放约 3 秒即越过 90%，进度记录被删除；这同时证明恢复 seek 和 90% 清理生效。

### 4. 真实 2FA：通过

- `tvos2fa` 首次提交后进入 OTP 输入状态。
- 测试根据 TOTP Secret 动态生成当前验证码。
- 后端连续收到两次 `POST /api/auth/login`：首次 challenge、第二次携带 OTP。
- OTP 登录成功并进入真实根目录。

### 5. 真实夸克网盘播放：通过

- tvOS Simulator 能浏览 AList 挂载的夸克网盘目录。
- 4K 视频可通过真实挂载正常播放。

### 6. 安全负例：通过

- 已验证不安全或不匹配的连接不能继续登录。
- 将 HTTPS 连接指向本地 HTTP 调试端口 `5244` 时，tvOS 显示 TLS 连接失败，未绕过证书或降级连接。

### 文件封面：通过

- tvOS Simulator 在真实夸克网盘视频目录中展示视频封面。
- 视频卡片从占位符加载为约第 1 秒的视频帧。
