# OpenRemote 远程桌面

OpenRemote 是一个开箱即用的远程桌面客户端：在两台电脑上登录同一个中转节点，就能互相看到并远程控制，无需公网 IP、无需路由器端口映射。

- 后端：Go 1.25 + Wails v3
- 前端：Vue 3 + Vite + PrimeVue 4 + Pinia
- 穿透协议：OpenPenetrate —— 内网穿透可视化管理工具
- 传输格式：ffmpeg 编码 H.264（Annex-B），浏览器端 WebCodecs 硬解

## 视频演示

[OpenRemote 远程桌面演示（哔哩哔哩）](https://www.bilibili.com/video/BV1TPW86xEvi/)

## 功能特性

- **实时画面**：ffmpeg 采集并编码 H.264，优先硬件编码（NVENC / QSV / AMF / VideoToolbox），不可用时回退 libx264
- **画质与帧率可调**：原画 / 高清 / 流畅 / 省流四档，15 / 30 / 60 FPS 可选，按带宽调整码率
- **完整键鼠控制**：鼠标移动、按键、滚轮；键盘按扫描码注入，支持组合键、功能键、Win 键、方向键与长按连发
- **双向剪切板**：`Ctrl+V` 把本机剪切板发送到远端并粘贴，`Ctrl+C` 把远端剪切板同步回本机
- **文件传输**：控制端上传 / 下载文件，被控端保存到 `Downloads/OpenRemote`
- **悬浮工具栏**：默认悬浮在右上角，可拖拽、3 秒自动隐藏，全屏时同样可用
- **一键部署节点**：填写服务器 SSH 信息，客户端自动上传并启动中转节点
- **中英文界面**：在设置中一键切换

## 工作原理

```
控制端  ──WS /vnc/{deviceId}?token=──▶  中转节点  ◀──WS /tunnel/{token}──  被控端
```

- 中转节点默认监听 `:9000`，提供 `/ws` 信令、`/vnc/{deviceId}` 控制端接入、`/tunnel/{token}` 被控端回连、`/health` 健康检查
- 两端就绪后由节点双向搬运数据：视频与文件分片走二进制帧（首字节标记类型），鼠标、键盘、剪切板等控制事件走 JSON 文本帧
- 被控端持续把画面编码为 H.264 Annex-B 码流下发，控制端用 `VideoDecoder` 解码并绘制到画布
- 发送端使用有界队列，网络拥塞时主动丢弃非关键帧，避免延迟堆积

节点程序内嵌在客户端中（`assets/openremote-node-linux-*`），部署时通过 SSH 上传到服务器，无需手动安装。

## 快速开始

环境要求：

- Go 1.25+
- Node.js 18+
- `ffmpeg` 已安装并加入 `PATH`
- Wails v3 CLI（`wails3`）

开发模式（前端热更新）：

```bash
wails3 dev
```

构建生产版本（产物在 `build` 目录）：

```bash
wails3 build
```

手动构建前端与可执行文件：

```bash
npm install --prefix frontend
npm run build --prefix frontend
go build -o bin/OpenRemote.exe .
```

修改 Go 侧导出方法后重新生成前端绑定：

```bash
wails3 generate bindings -clean=true
```

交叉编译 Linux 中转节点（需在打包前放回 `assets/`）：

```bash
GOOS=linux GOARCH=amd64 go build -o assets/openremote-node-linux-amd64 ./cmd/openremote-node
GOOS=linux GOARCH=arm64 go build -o assets/openremote-node-linux-arm64 ./cmd/openremote-node
```

## 使用说明

1. 在「添加服务器」中填写主机地址、SSH 端口、用户名与密码，保存时自动部署中转节点（默认 9000 端口，需在安全组放行）
2. 在两台电脑上分别运行 OpenRemote 并连接同一个节点，设备会出现在彼此的在线列表里
3. 点击「远程控制」，被控端同意后开始传输画面
4. 通过悬浮工具栏切换画质与帧率、收发文件、同步剪切板

> 如果服务器带宽只有 1 Mbps，建议选择「省流」（960 px / 700 kbps），并适当降低帧率，可获得更跟手的操作体验。

## 项目结构

```
.
├── main.go                # 应用入口
├── app.go                 # Wails 应用结构与导出方法
├── hubclient.go           # 节点信令与设备管理
├── screencast.go          # 屏幕采集、H.264 编码与码流下发
├── input_windows.go       # 鼠标事件注入
├── keyboard_windows.go    # 键盘扫描码注入
├── clipboard_*.go         # 各平台剪切板读写
├── file_transfer.go       # 文件分片传输
├── sshdeploy.go           # 通过 SSH 部署中转节点
├── internal/node/         # 中转节点（信令、会话转发）
├── cmd/openremote-node/   # 节点独立可执行程序
├── assets/                # 内嵌的节点程序与图片资源
└── frontend/              # Vue 3 前端
    ├── src/views/         # 设备、添加服务器、远程控制、推荐服务器、设置
    └── src/stores/        # Pinia 状态与多语言文案
```

## License

Copyright 2026 puweixiang

本项目基于 [Apache License 2.0](LICENSE) 开源，你可以自由使用、修改与分发，并需保留版权与许可声明。

```
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
