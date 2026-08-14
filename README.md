<p align="center">
  <img src="docs/images/dj-4g-hub-icon.png" width="128" alt="DJ 4G Hub icon">
</p>

# DJ 4G Hub

DJ 4G Hub 是一个本地优先的 macOS 设备控制台，支持兼容的 **DJI 4G 模块**。它通过模块已有的 USB 接口，在 Mac 上提供设备状态、短信、eSIM Profile、蜂窝上网、网络活动和 AT 调试能力，不修改模块固件。

管理服务和网页均运行在本机，默认只监听 `127.0.0.1:7575`。SIM、短信、EID、ICCID 和网络连接信息不会由项目主动上传到远程服务器。

> [!IMPORTANT]
> DJ 4G Hub 是独立开发的非官方开源项目，未获得 DJI 的授权、赞助或认可，与 DJI、Quectel、运营商或 eSIM 卡片厂商不存在隶属或合作关系。DJI 及相关产品名称是其各自权利人的商标，仅用于说明兼容性。

## 界面预览

<p align="center">
  <img src="docs/images/dj4hub-console-overview-light.png" width="100%" alt="DJ 4G Hub 浅色主题设备概览与联网活动">
</p>

<table>
  <tr>
    <td width="50%" valign="top">
      <img src="docs/images/dj4hub-console-network-light.png" width="100%" alt="DJ 4G Hub 网络诊断与 USB 网卡模式">
    </td>
    <td width="50%" valign="top">
      <img src="docs/images/dj4hub-console-at-light.png" width="100%" alt="DJ 4G Hub AT 调试控制台">
    </td>
  </tr>
  <tr>
    <td align="center"><sub>网络诊断与 USB 网卡模式</sub></td>
    <td align="center"><sub>AT 调试控制台</sub></td>
  </tr>
</table>

## 为什么是一个新项目

DJ 4G Hub 最初从 [ZenGeekLabs/DJOneHub](https://github.com/ZenGeekLabs/DJOneHub) 的代码与实践出发，也使用了其上游 [iniwex5/vohive](https://github.com/iniwex5/vohive) 的部分基础能力。随着 macOS 端持续开发，本项目已经重新设计了产品界面、设备工作流、网络诊断和发行方式，因此以独立项目继续维护。

独立维护不代表抹去来源。仓库继续保留原许可证要求的声明、上游作者署名以及第三方组件许可证。详细来源见 [项目来源与许可](#项目来源与许可) 和 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

## 相关项目

4G Connect 是独立开发、独立发布的 MIT 项目。本仓库只通过 Git submodule 固定一个经过验证的版本，方便一起检出和联调，不将其源码、提交、Issue、Actions、Release 或许可证合并进 DJ 4G Hub。

| 项目 | 位置 | 用途 | 维护方式 |
| --- | --- | --- | --- |
| DJ 4G Hub | 当前仓库 | 完整设备控制台：短信、eSIM、网络、流量和 AT 调试 | 当前仓库独立维护 |
| [4G Connect](https://github.com/WongLoki/4G-Connect) | `apps/4g-connect` submodule | 双击即用的一次性 USB 网卡激活 App | 独立仓库、MIT License |

克隆时需要同时取回子仓库：

```sh
git clone --recurse-submodules https://github.com/WongLoki/DJ4Hub.git
```

已经克隆主仓库时：

```sh
git submodule update --init --recursive
```

## 我们重新实现和新增了什么

- 重新设计本地网页控制台，统一浅色、深色和响应式界面。
- 面向 macOS 的 USB 设备发现、libusb AT 通信、热插拔恢复和换卡刷新。
- 短信收发、自动轮询、验证码提取、长短信分片和模块旧短信清理。
- 实体 eUICC 卡片的 Profile 读取、下载、启用、改名、删除及号码备注。
- 短信模式与 USB 网卡模式切换，并明确显示当前工作模式。
- 一次性 `activate` 工具：清理残留网络服务、确认 `usbnet=1`、重启模块并等待 macOS DHCP。
- 实时上下行速度、本次会话流量、USB 网卡、默认出口和代理诊断。
- 联网链路识别，例如 `en9 → utun → 应用`，并展示应用、目标地址、端口、协议和累计流量。
- 本地服务启动器、日志管理、Apple Silicon 发行包和 GitHub Actions 自动构建。

## 功能状态

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 设备自动识别 | 可用 | 识别受支持的 DJI 4G 模块，处理拔出与重新连接 |
| 实时状态 | 可用 | 运营商、信号、网络制式、SIM、工作模式和流量 |
| 短信 | 可用 | 收发、轮询、验证码、长短信与模块存储清理 |
| eSIM / 卡片 | 可用 | 管理插在模块卡槽中的兼容实体 eUICC 卡片 |
| USB 4G 上网 | 可用 | 切换 USB 网卡模式并检查 macOS 网络接口 |
| 联网活动 | 可用 | 展示连接元数据，不读取 HTTPS 页面内容 |
| AT 调试 | 可用 | 直接向模块发送 AT 指令 |
| Apple Silicon | 可用 | 当前发行包面向 macOS 13+、M 系列芯片 |
| Intel Mac | 未验证 | 尚未发布经过真机验证的发行包 |
| iPhone / iPad | 规划中 | 需要独立的移动端架构、权限和安全设计 |

## 硬件与系统

- 受支持的 DJI 4G 模块，常见 USB 标识为 `2ca3:4006`
- 可用的实体 SIM，或兼容的实体 eUICC/eSIM 卡片
- 支持数据传输的 USB-C 线缆
- Apple Silicon Mac
- macOS 13 Ventura 或更新版本

发行包会携带所需的 `libusb`。普通用户不需要安装 Go、Node.js 或 Homebrew。

## 工作模式

| 模式 | 页面名称 | 主要用途 |
| --- | --- | --- |
| `usbnet=0` | 短信模式 | 状态、短信、eSIM 和 AT 调试 |
| `usbnet=1` | 上网模式 | 向 macOS 暴露 USB 网卡并使用 SIM 数据 |
| `usbnet=2/3` | 实验模式 | 用途和稳定性尚未完成验证 |

切换模式会触发 USB 重新枚举，页面短暂显示断开属于正常现象。不要在 eSIM Profile 写入过程中拔出模块或切换模式。

## 下载与安装

从项目 [Releases](https://github.com/WongLoki/DJ4Hub/releases) 下载名称包含 `DJ-4G-Hub-macOS-arm64` 的 ZIP，并按需使用同名 `.sha256` 校验文件。只需要一次性激活工具时，请前往 [4G Connect Releases](https://github.com/WongLoki/4G-Connect/releases)。

```sh
shasum -a 256 DJ-4G-Hub-*.zip
```

完整解压后，在发行包目录执行：

```sh
./install
```

程序默认安装到：

```text
/usr/local/libexec/dj4ghub
```

命令入口位于：

```text
/usr/local/bin/dj4ghub
```

## 使用

连接模块后启动：

```sh
dj4ghub start
```

管理页面会自动打开：

```text
http://127.0.0.1:7575
```

常用命令：

```text
dj4ghub start          启动并自动打开管理页面
dj4ghub start --demo   启动无硬件演示模式
dj4ghub activate       不启动网页；清理残留网卡并激活上网模式
dj4ghub stop           停止服务
dj4ghub status         查看运行状态
dj4ghub logs           查看实时日志
dj4ghub open           重新打开管理页面
```

`dj4ghub activate` 是一次性命令。它只处理与第一代 DJI 4G 模块匹配的残留网络服务，并在需要时确认 `usbnet=1`、软重启模块、等待 ECM 网卡与 DHCP 地址，完成后立即退出。

## 页面能力

### 概览

显示运营商、信号、LTE 注册、SIM 状态、当前工作模式、实时速度和本次运行期间的累计流量。

“联网活动”会尝试还原真实链路：

```text
应用 → 系统隧道（可选）→ 兼容模块 USB 网卡 → 蜂窝网络
```

页面可以显示应用名、目标域名或 IP、端口、协议和累计上下行字节。域名依赖 macOS 本地解析缓存；HTTPS 页面路径和通信内容不可见。

### 短信

支持接收、发送、自动轮询、验证码提取和长短信分片。国际号码请使用完整格式，例如 `+86138XXXXXXXX`。

### eSIM / 卡片

这里管理的是插在模块实体 SIM 卡槽中的兼容 eUICC 卡片，不是 Mac 内置 eSIM。Profile 下载、启用、改名和删除会真实修改卡片，操作过程中不要拔出设备。

### 网络

显示 macOS 是否识别 USB 网卡、物理接口、默认出口、蜂窝 IP、PDP/APN 信息，并提供 4G 出口与代理检测。

### AT 调试

AT 调试面向诊断和开发。不了解作用的写入类命令不要执行，也不要照搬来源不明的刷机指令。

## 本地数据与隐私

新版本使用以下目录：

```text
~/Library/Logs/DJ 4G Hub/dj4ghub.log
~/Library/Application Support/DJ 4G Hub
```

Profile 号码备注会兼容读取旧的 `DJOneHub` 和 `VoHive macOS` 数据目录，后续写入统一保存到 `DJ 4G Hub`。

发布 Issue、截图或日志前，请隐藏手机号、短信验证码、EID、ICCID、IMSI 和其他个人信息。

## 从源码开发

```sh
go test ./...
./scripts/build-macos.sh
./scripts/package-macos-arm64.sh v0.1.0-preview
```

主要目录：

```text
cmd/dj4ghub-macos/       macOS 服务、USB AT 和内嵌网页
apps/4g-connect/         独立 4G Connect 仓库的 submodule 引用
internal/                设备后端、短信、eSIM 与配置能力
pkg/                     MBIM、短信编码和日志组件
packaging/               安装器、启动器与发行说明
scripts/                 本地构建和 Apple Silicon 打包脚本
```

DJ 4G Hub 在本仓库运行测试与发布流程，推送 `v*` 标签时构建 Release。4G Connect 在自己的仓库中运行独立 Actions 和发布流程。

## 移动设备路线图

移动端不会简单地把当前 macOS 二进制搬到 iPhone 或 iPad。后续计划先拆分设备层与控制 API，再评估：

1. 带鉴权和加密的局域网远程控制模式。
2. iPhone / iPad Companion App，用于状态、短信和流量查看。
3. 对移动系统 USB 权限、后台运行和 App Store 规则的可行性验证。
4. 在不暴露短信、SIM 身份和控制接口的前提下设计配对流程。

在安全模型完成前，服务仍默认只监听本机回环地址。

## 当前限制

- 发行包目前只支持 Apple Silicon。
- 不同 SIM、eUICC、运营商、漫游环境和模块固件可能存在差异。
- 联网活动只显示连接元数据，不能看到 HTTPS 内容，也不等同于运营商账单。
- 当前发行包使用临时签名，尚未经过 Apple Developer ID 公证。
- 模式 2 和模式 3 仍属于实验功能。

## 项目来源与许可

本仓库是独立维护项目，但包含从以下项目演进或借鉴的工作：

- [ZenGeekLabs/DJOneHub](https://github.com/ZenGeekLabs/DJOneHub)
- [iniwex5/vohive](https://github.com/iniwex5/vohive)
- libusb 及仓库中列出的其他第三方开源组件

DJ 4G Hub 主项目包含从 VoHive 演进而来的代码，因此根目录代码继续遵循 [PolyForm Noncommercial License 1.0.0](LICENSE)，不是 MIT、Apache-2.0 等宽松许可证。源码公开不代表可以忽略非商业限制。

4G Connect 是另一个独立仓库，使用其自身的 [MIT License](https://github.com/WongLoki/4G-Connect/blob/main/LICENSE)。本仓库中的 submodule 只是对外部提交的引用，该 MIT 授权不改变 DJ 4G Hub 的许可证，DJ 4G Hub 的条款也不覆盖 Connect 仓库。

必须保留的上游声明：

```text
Required Notice: Copyright iniwex5 (https://github.com/iniwex5/vohive)
```

libusb 1.0.30 使用 GNU Lesser General Public License v2.1 or later；其他依赖遵循各自许可证。完整信息见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) 和各 `third_party/` 目录中的许可证文件。

## 贡献

欢迎提交兼容性结果、问题日志、UI 改进和新设备适配。涉及网络、短信、eSIM 写入或 USB 模式切换的改动，请同时说明硬件型号、固件、macOS 版本和验证方式，并先清理隐私数据。
