<p align="center">
  <img src="docs/images/dj-4g-hub-icon.png" width="128" alt="DJ 4G Hub icon">
</p>

# DJ 4G Hub

DJ 4G Hub 是一个本地优先的 macOS 设备控制台，支持兼容的 **DJI 4G 模块**。它通过模块已有的 USB 接口，在 Mac 上提供原生 macOS 客户端与 Web 控制台，提供设备状态、短信、SIM 电话、eSIM Profile、蜂窝上网、网络活动和 AT 调试能力，不修改模块固件。

管理服务和网页均运行在本机，默认只监听 `127.0.0.1:7575`。项目没有远程遥测或通信记录上传服务。选择云盘备份目录后，云盘客户端会同步其中的备份，备份包含短信正文、号码和 SIM 归属信息。

> [!IMPORTANT]
> DJ 4G Hub 是独立开发的非官方开源项目，未获得 DJI 的授权、赞助或认可，与 DJI、Quectel、运营商或 eSIM 卡片厂商不存在隶属或合作关系。DJI 及相关产品名称是其各自权利人的商标，仅用于说明兼容性。

## 本次新增

| 能力 | 现在可以做什么 |
| --- | --- |
| 原生 macOS App | SwiftUI 界面、内置设备服务，Web 继续独立可用 |
| SIM 电话与电脑音频 | 拨号、接听、挂断、语音菜单按键；可选模块音频后台准备，通话时连接电脑音频 |
| 最近通话 | 按日期分组，区分来去电和未接，查看时长与 SIM 归属，选择号码填入拨号框 |
| SQLite 通信历史 | 按 ICCID 保存短信和通话，自动迁移旧 JSON，换卡后保留历史 |
| 云盘备份 | 自选目录、变化后定期快照、保留 10 份、手动备份与安全合并恢复 |
| 来电／短信提醒 | macOS 系统通知和提示音；Web 提示音需单独开启 |
| 界面与识别修复 | 原生控件与点击区域优化、Web APN 自定义下拉、兼容 ICCID 末尾填充字符 |

原生 App 当前为 **Apple Silicon / macOS 13+ 开发预览**，本地签名、未公证。打包成功不等于长期音频稳定性已验收；新功能源码也不代表 GitHub Release 已更新。

- [原生客户端：构建、启动与云盘备份](apps/hub-macos/README.md)
- [通信记录：卡片归属、SQLite 迁移与限制](docs/COMMUNICATION_HISTORY.md)
- [可选音频：依赖来源、实验记录与兼容边界](docs/QDC507_AUDIO_RESEARCH.md)

## 界面预览

以下为 **Web 控制台预览**，不是原生 macOS App 截图。截图仅用于展示界面，不作为设备或音频兼容性的证据。

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

- SwiftUI 原生 macOS 客户端与本地 Web 共用设备服务，保留两种使用入口。
- 重新设计本地网页控制台，统一浅色、深色和响应式界面。
- SIM 通话、可选电脑音频、系统通知，以及按 SIM 保存的 SQLite 通信历史和云盘备份。
- 面向 macOS 的 USB 设备发现、libusb AT 通信、热插拔恢复和换卡刷新。
- 短信收发、自动轮询、验证码提取、长短信分片和模块旧短信清理。
- 实体 eUICC 卡片的 Profile 读取、下载、启用、改名、删除及号码备注。
- 短信模式与 USB 网卡模式切换；进入上网模式后等待 DHCP，并通过多个公网地址自动验证联网能力。
- 一次性 `activate` 工具：清理残留网络服务、确认 `usbnet=1`、重启模块并等待 macOS DHCP。
- 实时上下行速度、本次会话流量、USB 网卡、默认出口和代理诊断。
- 联网链路识别，例如 `en9 → utun → 应用`，并展示应用、目标地址、端口、协议和累计流量。
- 本地服务启动器、日志管理、Apple Silicon 发行包和 GitHub Actions 自动构建。

## 功能状态

客户端和 Web 电话页面支持拨号、接听、挂断、通话状态和语音菜单按键，无预置号码快捷拨号。Web 音频连接需要模块已枚举为 USB 声卡，并使用支持输出设备选择的浏览器；原生客户端使用 AudioQueue，不依赖浏览器。2026-09-12 的 QDC507GLEFM21 真机测试中，通过浏览器连接 MacBook 麦克风、扬声器与模块 AC / AS Interface，测试者确认电脑到手机、手机到电脑的声音均清楚；这是单台设备的短时双向通话验证，不代表长期稳定性或所有设备兼容。APN 设置提供 One NZ `web` 预设和自定义主数据 APN，保存后在下一次数据连接生效，保留 IMS 数据配置。

#### 实验模块音频（本机可选）

仅针对已验证的 QDC507GLEFM21 / Linux 3.18.44，并要求 ADB 已开启且获授权。标准 `adb root` 可用时会自动尝试并重新校验；不绕过授权、不利用漏洞、不解锁或刷机。程序自动检查 `~/Library/Application Support/DJ4Hub/experimental-audio`，ADB 自动从该目录的 `platform-tools/adb` 或 PATH 寻找，无需每次设置环境变量。需要时仍可用 `DJ4GHUB_ADB_PATH`、`DJ4GHUB_MODULE_VOICE_DIR` 覆盖路径。

首次使用先从可信来源准备运行文件，执行 `dj4ghub audio-install /本机/运行文件目录`，再执行 `dj4ghub audio-check`。导入前验证固定哈希，不覆盖已有无效目录、不接触硬件；ADB 请单独安装官方 Android Platform Tools。所需文件、来源和哈希见 [音频研究记录](docs/QDC507_AUDIO_RESEARCH.md)。项目不分发、自动下载第三方驱动，也不会自动开启 ADB。常规便携包已包含音频控制代码，但新机器仍需准备这些可选依赖。

Web 电话页面默认自动使用电脑通话音频：首次拨号提示允许临时 USB 重连，随后自动初始化、连接音频再拨号；以后进入电话页会提前初始化。挂断只关闭电脑麦克风和播放，模块保留待机供下一通复用。关闭页面或心跳中断约 45 秒后恢复 USB，也可手动“停止待机并恢复 USB”。不再设置一小时强制中断，但长时间稳定性尚未验证。来电时若未初始化，不强行重连 USB；可取消“自动使用电脑通话音频”仅操作拨号/接听。建议使用耳机。临时驱动保留至模块重启，不热卸载、不写入固件。

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 原生客户端 | 开发预览 | SwiftUI 界面、内置服务，保留 Web 备用入口 |
| 电话与音频 | 部分实验 | 通话控制可用；模块驱动、固件与音频路径需匹配，长期稳定性未完成验收 |
| 通信历史 | 可用 | SQLite 存储、JSON 自动迁移、按 ICCID 筛选；历史观测有局限 |
| 云盘备份 | 可用 | 客户端设置中选择目录，备份／合并恢复；不代表云端上传已完成 |
| 设备自动识别 | 可用 | 识别受支持的 DJI 4G 模块，处理拔出与重新连接 |
| 实时状态 | 可用 | 运营商、信号、网络制式、SIM、本机号码、工作模式和流量 |
| 短信 | 可用 | 收发、轮询、验证码、长短信与模块存储清理 |
| eSIM / 卡片 | 可用 | 管理插在模块卡槽中的兼容实体 eUICC 卡片 |
| USB 4G 上网 | 可用 | 切换 USB 网卡模式，恢复 DHCP，并通过百度、Google 等多个地址自动验证公网 |
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

## 原生 macOS 客户端（开发预览）

新增独立的 **DJ 4G Hub.app**：SwiftUI 原生界面，内置 Go 设备服务，不是网页套壳。提供概览、短信、电话、eSIM、网络、AT 调试和菜单栏入口；Web 保留独立使用，客户端也可以复用已运行的本地服务。独立的 4G Connect 子仓库保持不变。

客户端可在识别到设备后延迟后台准备可选模块音频，不在待机时开启麦克风；通话时才连接电脑音频。初始化可能触发 USB 短暂重连。USB 配置支持已验证的 RMNET / ECM 组合，停止时恢复初始化前的组合，不为启用音频强行切换网卡模式。固件、内核、USB 身份或 ADB 权限不匹配时停止，而非冒险继续。

构建与使用见 [原生客户端说明](apps/hub-macos/README.md)。目前是本地签名的开发预览，未公证；原生音频通道、长时间通话和休眠恢复仍需真机验收，不能将此前 Web 通话验证视作原生验证。卡片兼容性、号码备注等高级操作继续保留在 Web。

## 下载与安装

**原生 App：** 当前请按[构建说明](apps/hub-macos/README.md#构建)生成 `DJ 4G Hub.app`，可复制到“应用程序”。以下 ZIP 安装流程是便携 Web 服务，不要与原生 App 混淆。


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

### 电话与最近通话

电话页支持拨号、接听、挂断、按键和音量控制。原生客户端顶部“最近通话”按日期展示记录，未接来电标红；右侧电话按钮只填入拨号框，不立即拨出。记录为空时可检查“全部 SIM 卡”或“未归属”，不会将无法证明归属的旧记录强行绑定到当前卡。

### 云盘备份

原生客户端 **设置 → 通信记录备份 → 选择文件夹**，选择自己的私密 iCloud Drive、Documents 或其他云盘目录，再开启自动备份。服务运行时有变化才备份，两次成功备份至少间隔 30 分钟；仅清理本安装实例超过 10 份的旧快照。主数据库留在本机，不参与云盘实时同步。

“立即备份”生成独立 SQLite 快照；“从备份恢复”先保存本机安全副本，再补回缺失记录，不覆盖现有同 ID 记录，不写入 SIM。文件未加密；请在 Finder 确认云盘同步完成。详情见[备份说明](apps/hub-macos/README.md#通信记录云盘备份)。

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
~/Library/Application Support/DJ 4G Hub                 # Profile 备注等
~/Library/Application Support/DJ4Hub/communication-history.sqlite
~/Library/Application Support/DJ4Hub/history-backup.json
~/Library/Application Support/DJ4Hub/Restore Backups/   # 恢复前安全副本
~/Library/Application Support/DJ4Hub/experimental-audio/ # 可选音频依赖
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
apps/hub-macos/          SwiftUI 原生 App 与 NativeAudio 音频桥
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
