# DJOneActivator

DJOneActivator 是大疆第一代 4G 模块（USB `2ca3:4006`）的 macOS 一次性上网激活工具。

双击 `DJOneActivator.app` 后，它会：

1. 识别大疆/百旺 `EG25G-QDC507` 模块；
2. 清理或禁用已经失效的 `Baiwang / EG25 / QDC507` 网络服务；
3. 如果 ECM 网卡已经可用，立即提示结果并退出；
4. 否则确认模块使用 `usbnet=1`，执行一次软重启；
5. 等待 macOS 重新枚举 ECM 网卡并取得 DHCP 地址，然后退出。

它不会启动网页服务，也不会常驻后台。

## 系统要求

- Apple Silicon Mac
- macOS 13 Ventura 或更新版本
- 大疆第一代 4G 模块
- 可正常使用的数据 SIM

## 使用

从 Releases 下载 ZIP，完整解压，将 `DJOneActivator.app` 拖入“应用程序”，插入模块后双击运行。

执行结果会通过 macOS 通知显示，详细日志位于：

```text
~/Library/Logs/DJOneActivator/activator.log
```

如果 DJOneHub 正在运行，请先停止它，以释放 USB AT 接口。

### macOS 阻止打开

当前预览构建使用临时签名，没有 Apple Developer ID 公证。请在“系统设置 → 隐私与安全性”中选择“仍要打开”。如果系统仍提示文件损坏，可对可信下载执行：

```sh
xattr -dr com.apple.quarantine /Applications/DJOneActivator.app
```

## 从源码构建

需要 Go 1.26、`pkg-config`、Clang 和 Xcode Command Line Tools：

```sh
./scripts/build-app.sh dev
```

输出的 ZIP 位于 `dist/`；构建脚本会在临时目录签名，并重新解压验证 App 后才结束。

## 许可证

本项目派生自 DJOneHub 的 USB AT 和激活流程，使用 [PolyForm Noncommercial License 1.0.0](LICENSE)。构建产物动态链接 libusb，并在 App Resources 中包含其许可证。

Required Notice: Copyright iniwex5 (https://github.com/iniwex5/vohive)
