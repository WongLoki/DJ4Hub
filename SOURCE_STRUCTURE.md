# DJ 4G Hub Monorepo 源码结构

仓库同时维护完整的 DJ 4G Hub 控制台和独立的 DJI 4G Connect 激活 App。主项目按 `cmd/dj4ghub-macos` 的真实 Go 依赖图保留必要源码；Connect 作为独立 Go module 位于 `apps/`，两者由根目录 `go.work` 组成统一开发工作区。

## 目录树

```text
DJ-4G-Hub-source-minimal/
├── cmd/
│   └── dj4ghub-macos/       # macOS 主程序、USB AT、短信、网络与内嵌网页
│       └── web/              # 当前实际显示的原生管理页面
├── apps/
│   └── dji-4g-connect/       # 一次性 USB 网卡激活 App（独立 MIT Go module）
├── internal/
│   ├── apduarbiter/          # SIM/eUICC APDU 通道并发协调
│   ├── backend/              # AT、MBIM、QMI 后端的统一能力接口
│   ├── config/               # 运行配置与设备配置
│   ├── esim/                 # eUICC/Profile 读取、下载、切换和删除
│   ├── modem/                # 调制解调器发现、AT 指令和状态解析
│   └── simaid/               # SIM 应用 AID 发现与选择
├── pkg/
│   ├── logger/               # 日志适配
│   ├── mbim/                 # MBIM 协议实现
│   └── smscodec/             # SMS PDU 编解码与长短信重组
├── packaging/
│   ├── dj4ghub              # 终端 start/stop/status/logs/open 启动器
│   ├── install               # /usr/local 安装脚本
│   ├── README.md             # 发行包内的安装说明
│   └── THIRD_PARTY_NOTICES.md
├── scripts/
│   ├── build-macos.sh        # 本地开发构建
│   └── package-macos-arm64.sh# Apple Silicon 发行包构建
├── third_party/              # 当前构建实际使用的本地第三方源码
├── go.mod
├── go.work                   # DJ 4G Hub 与 DJI 4G Connect 工作区
├── go.sum
├── LICENSE
├── THIRD_PARTY_NOTICES.md
├── README.md
└── MACOS.md
```

## 关键入口

- `cmd/dj4ghub-macos/main.go`：HTTP 服务、设备状态、短信、eSIM、网络和流量 API。
- `cmd/dj4ghub-macos/usbat_darwin.go`：macOS 上通过 libusb 接管大疆模块 USB AT 接口。
- `cmd/dj4ghub-macos/usbat_esim_channel.go`：经 AT/APDU 访问实体 eUICC 卡片。
- `cmd/dj4ghub-macos/web/`：由 `go:embed` 编译进二进制的网页界面。

## 为什么仍有 internal、pkg 和 third_party

Go 以“包”为编译边界。macOS 主程序虽然集中在 `cmd/dj4ghub-macos`，但短信 PDU、eUICC、SIM APDU、MBIM/QMI 和日志能力依赖共享包，因此这些目录不能直接删除。

`third_party` 中只保留当前依赖图实际使用的本地替换模块。保留本地副本可以确保当前修改版协议实现与已验证发行包一致，同时保留各上游组件的许可证和来源信息。

## 已排除内容

- `web/` 旧 Vue/Vite 管理前端及约 601 MB 的 `node_modules`
- 原 Linux 服务端入口、容器配置和网络命名空间工具
- Telegram、飞书、QQ 等机器人与转发功能
- 原项目未被 macOS 入口引用的 API、任务、数据库和后台页面
- `dist/`、下载包、日志、缓存及其他生成文件

## 验证方式

运行全部保留包的测试：

```sh
go test -mod=mod ./...
```

生成 Apple Silicon 发行包：

```sh
./scripts/package-macos-arm64.sh v0.1.0-preview
```

构建脚本会从 libusb 官方 Release 下载源码、核对 SHA-256，并将编译后的动态库与 DJ 4G Hub 一起打包。

## 模块与来源

当前 Go module 路径为 `github.com/WongLoki/dj-4g-hub`。仓库作为独立项目维护，但从 DJOneHub、VoHive 和第三方模块演进而来的代码仍保留其原始许可证与声明，详见根目录 `LICENSE` 和 `THIRD_PARTY_NOTICES.md`。
