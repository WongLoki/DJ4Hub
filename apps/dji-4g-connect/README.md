# DJI 4G Connect

![DJI 4G Connect icon](assets/AppIcon-1024-glass-logo.png)

DJI 4G Connect 是一个开源的 macOS 小工具，用于解决**大疆第一代 4G 模块**连接 Mac 后，首次 USB 枚举没有直接出现可用网卡的问题。

本项目现作为 [DJ 4G Hub monorepo](https://github.com/WongLoki/dj-4g-hub) 中的独立 App 维护，源码位于 `apps/dji-4g-connect/`，并继续使用本目录的 MIT License。

它不是完整的调制解调器管理器，也不会启动网页或常驻后台。插入模块后双击 App，它会完成一次检查和必要的恢复，显示 macOS 通知后自动退出。

## 它解决什么问题

部分大疆第一代 4G 模块已经保存了 ECM 上网模式，但冷插到 macOS 时可能只暴露厂商自定义 USB 接口。macOS 此时不会创建可用的 Ethernet 网卡；手动让模块软重启并重新枚举后，ECM 网卡才会出现。

DJI 4G Connect 将这套恢复流程封装成一次点击：

1. 检测 USB `2ca3:4006` 的大疆/百旺 `EG25G-QDC507` 模块；
2. 查找由模块旧枚举留下的 `Baiwang / EG25 / QDC507` 网络服务；
3. 只清理对应硬件已经不存在且仍处于启用状态的残留服务，不触碰 Wi-Fi、VPN、其他网卡或用户已经禁用的服务；
4. 如果 ECM 网卡已经工作并获得 `192.168.225.x` 地址，立即退出；
5. 否则通过 libusb 打开模块的 USB AT 接口；
6. 确认 `AT+QCFG="usbnet"` 为 ECM 模式 `1`，必要时写入；
7. 执行 `AT+CFUN=1,1` 软重启，等待 macOS 重新枚举并通过 DHCP 获得地址；
8. 使用 App 自身的原生通知显示结果，然后退出。

## 隐私与安全

- 完全在本机运行；
- 不启动 HTTP 服务，不监听端口；
- 不常驻后台，不安装 LaunchAgent；
- 不包含遥测、分析或自动更新；
- 不读取短信、通讯录、eSIM Profile 或浏览器数据；
- 不上传 SIM、设备或网络信息；
- 网络服务清理严格限制在 `Baiwang / EG25 / QDC507` 身份和已消失的设备节点。

日志仅写入本机：

```text
~/Library/Logs/DJI 4G Connect/connect.log
```

## 系统要求

- Apple Silicon Mac；
- macOS 13 Ventura 或更新版本；
- 大疆第一代 4G 模块，常见 USB ID 为 `2ca3:4006`；
- 可正常使用的数据 SIM。

目前没有 Intel Mac 构建。

## 下载与使用

1. 从 [monorepo Releases](https://github.com/WongLoki/dj-4g-hub/releases) 下载 `DJI-4G-Connect-macOS-arm64-*.zip`；
2. 可选：用同名 `.sha256` 文件核对下载；
3. 完整解压，将 `DJI 4G Connect.app` 拖入“应用程序”；
4. 插入模块，双击 App；
5. 等待原生通知显示激活结果。

如果 DJ 4G Hub 或其他程序正在占用模块 USB AT 接口，请先停止它。

### macOS 阻止打开

自动构建使用 ad-hoc 签名，没有 Apple Developer ID 公证。首次运行可能需要在“系统设置 → 隐私与安全性”中选择“仍要打开”。如果系统仍提示文件损坏，可对确认来源可信的 App 执行：

```sh
xattr -dr com.apple.quarantine "/Applications/DJI 4G Connect.app"
```

## 从源码构建

需要 Apple Silicon Mac、Go 1.26、Xcode Command Line Tools 和 `pkg-config`：

```sh
./scripts/build-app.sh dev
```

构建脚本会：

- 从 libusb 官方 Release 下载 1.0.30 源码；
- 验证固定 SHA-256；
- 构建并内置 `libusb-1.0.0.dylib`；
- 生成完整的 macOS `.icns` 图标；
- 生成并验证 ad-hoc 签名的 `.app`；
- 输出 ZIP 和 SHA-256 文件。

## 自动构建与发布

GitHub Actions 使用 `macos-15` Apple Silicon Runner：

- 每次 push 和 pull request 自动运行测试并打包；
- 构建结果作为 Actions Artifact 保存；
- 推送 `connect-v*` tag 时自动创建 GitHub Release，并上传 ZIP 与 SHA-256。

发布示例：

```sh
git tag connect-v0.3.0
git push origin connect-v0.3.0
```

## 开源许可证

本仓库中的独立实现使用 [MIT License](LICENSE)，允许个人和商业使用、修改、分发和再许可。动态链接的 libusb 使用 LGPL-2.1-or-later，完整许可证包含在发布 App 中，详情见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

从 `v0.2.0` 开始，当前实现已经独立重写并使用 MIT License。历史 `v0.1.x` 预览版仍按各自归档中附带的 PolyForm Noncommercial 条款授权，本次变更不追溯修改旧版本许可证。

DJI、DJI 标志、Quectel 和其他第三方名称或标志仍属于各自权利人；MIT License 不授予第三方商标权。本项目仅使用相关名称和标志说明兼容硬件。

## 非官方声明

本项目是独立的社区开源项目，与 DJI、百旺、Quectel、Apple、运营商或 SIM/eSIM 厂商不存在隶属、授权、赞助或合作关系。使用前请自行确认硬件保修、运营商资费和当地法律要求。

欢迎阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 并提交 Issue 或 Pull Request。
