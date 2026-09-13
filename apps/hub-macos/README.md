# DJ 4G Hub · 原生 macOS 客户端

SwiftUI 原生界面，不使用 WebView。短信、电话、eSIM、网络与 AT 操作复用 Go 服务；Web 保留独立运行。4G Connect 子仓库未改动。

## 通信记录云盘备份

设置 → 通信记录备份 → 选择自己的私密云盘文件夹，然后打开自动备份或点击立即备份。
主数据库仍在本机 Application Support/DJ4Hub；不会直接在云盘运行 SQLite。
服务运行时每分钟检查一次，每次成功备份后至少间隔 30 分钟，数据有变化才生成新快照。
使用 VACUUM INTO 包含已提交的 WAL 数据，本机生成后以临时文件写入所选目录并完成重命名。
仅保留该安装实例在所选目录的最近 10 个快照，不删除其他文件；云盘不可用时显示错误并等待重试。
成功表示本地备份文件已生成，不保证云盘上传完成；请在 Finder 确认同步状态。
备份包含短信正文和电话号码，未额外加密，不要放进公开共享文件夹。

从备份恢复会校验 SQLite 与记录格式，先在本机 DJ4Hub/Restore Backups 保存安全副本，再事务性补回缺失记录。
同 ID 的现有记录保持不变，不覆盖、删除当前记录，不写入 SIM 卡；恢复前的安全副本不会自动清理。
自动备份只覆盖 SQLite 通信记录（SIM 归属、短信、通话），不包含驱动、音频设置或其他配置文件。

## 构建

需要 Apple Silicon、macOS 13+、Xcode Command Line Tools、Go 与便携包构建所需依赖。

```sh
sh scripts/package-macos-arm64.sh native-backend
PKG_CONFIG_PATH="$TMPDIR/dj4ghub-macos-package-arm64/libusb-source" \
  sh scripts/package-macos-app.sh dist/release/DJ-4G-Hub-macOS-arm64-native-backend
```

输出 `dist/native/DJ 4G Hub.app`。目标已存在时不会覆盖，可用第二个参数指定新的绝对 `.app` 路径。应用仅本地 ad-hoc 签名，未公证，不代表可对外正式发行。

## 使用

打开 App 后优先复用本机 7576 / 7575 的兼容服务，没有服务则启动内置后端。菜单栏或侧栏可直接打开 Web。退出 App 会关闭本应用的音频流；只停止本 App 启动的后端，不结束用户独立启动的 Web 服务。两端共用同一音频会话锁，不能同时占用模块。

界面和只读数据可用 `open "dist/native/DJ 4G Hub.app" --args --demo` 检查：演示后端使用 7578，不操作实际 USB。切勿将演示数据当作真机验证。

## 原生音频的边界

仅在明确连接音频或拨号时请求麦克风权限，不使用浏览器，不保存录音。AudioQueue 分别选择电脑/模块设备 UID，以 8 kHz 单声道 PCM 实时传输，支持静音和收听音量；拔插或失去模块心跳时关闭流。实验驱动仍需要本机预先配置，来源、哈希和限制见仓库音频研究记录。

Web 双向通话已有短时实测，**新的原生 AudioQueue 通道尚需单独真机验证**。客户端不能绕过设备 ADB 初始化失败。没有模块声卡时，不会假装通话音频成功。

原生页面覆盖主要日常操作；更详细的卡片兼容性、号码备注等高级工具目前继续使用 Web。长时间通话、热插拔、休眠恢复、多设备等仍需验收。
