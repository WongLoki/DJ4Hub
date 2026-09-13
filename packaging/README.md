# DJ 4G Hub for macOS（Apple Silicon）

这是 DJ 4G Hub 的完整便携发行包，已经包含程序、启动器和 `libusb`，无需安装 Go 或 Homebrew。

## 安装

在完整解压后的目录执行：

```sh
./install
```

安装完成后：

```sh
dj4ghub start
```

浏览器会自动打开 `http://127.0.0.1:7575/`。启动终端需要保持运行；按 `Control+C` 或在另一个终端执行以下命令停止：

```sh
dj4ghub stop
```

## 免安装运行

也可以留在当前目录直接运行：

```sh
./dj4ghub start
```

## 常用命令

```text
dj4ghub status       查看状态
dj4ghub activate     不启动网页；清理残留网卡并激活上网
dj4ghub logs         查看实时日志
dj4ghub open         重新打开管理页面
dj4ghub start --demo 启动无硬件演示界面
```

## 电话与实验音频

电话页面已包含拨号、接听、挂断与电脑双向音频控制。仅 QDC507GLEFM21 / Linux 3.18.44 已完成短时双向实测。需要已授权 root ADB；程序不会自动开启 ADB 或刷写固件。

可选音频依赖不随发行包分发。安装官方 Android Platform Tools（adb 加入 PATH），从已核验来源准备音频文件后执行：

```sh
dj4ghub audio-install /本机/运行文件目录
dj4ghub audio-check
dj4ghub start
```

自动读取 `~/Library/Application Support/DJ4Hub/experimental-audio`；也支持该目录中的 `platform-tools/adb`。来源和固定哈希见 [研究记录](docs/QDC507_AUDIO_RESEARCH.md)。导入不会执行驱动或覆盖已有无效文件。

电话页拨号会自动初始化并连接电脑音频，首次需要允许短暂 USB 重连；以后进入电话页提前初始化，挂断只关闭电脑音频、保留模块待机。关闭页面或丢失心跳后恢复 USB，可手动停止待机。建议戴耳机。正在响铃但音频未就绪时不强行重连，可取消“自动使用电脑通话音频”仅接听。没有依赖时也可取消该选项使用拨号控制。驱动在模块重启后清除。

## macOS 安全提示

当前预览包尚未经过 Apple Developer ID 公证。请优先核对 Release 提供的 SHA-256。若 macOS 仍阻止已确认来源的文件，可在当前发行包目录执行：

```sh
xattr -dr com.apple.quarantine ./dj4ghub ./bin ./lib
./dj4ghub start
```

## 日志

```text
~/Library/Logs/DJ 4G Hub/dj4ghub.log
```

项目来源、非官方声明和许可证信息请查看仓库根目录的 `README.md`、`LICENSE` 与 `THIRD_PARTY_NOTICES.md`。
