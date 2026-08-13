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
