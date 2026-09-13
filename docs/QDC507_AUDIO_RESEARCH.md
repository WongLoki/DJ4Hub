# QDC507 通话音频研究

核验日期：2026-09-12。本文按阶段记录研究证据；最新测试范围如下，不代表所有设备兼容或完整双向通话质量认证。

## 最新实机测试与本地接入

最新交互改为自动音频：首次授权后，拨号前自动初始化并连接音频，进入电话页可提前初始化；挂断关闭电脑媒体流但保持模块待机，取消一小时硬上限，仍保留失联 45 秒恢复。正在响铃而音频未就绪时拒绝重连，可选择仅电话控制。以下“一小时上限”为早期实验记录，不是当前行为。新自动流程通过模拟回归测试；未额外拨号，长期待机与上网并行尚未实测。

常规程序与便携包已包含控制代码、本机依赖检查和安全导入命令，不再要求每次启动传环境变量。默认目录为 `~/Library/Application Support/DJ4Hub/experimental-audio`；`dj4ghub audio-install DIR` 仅导入哈希吻合的三个文件，`dj4ghub audio-check` 只检查本机文件、不操作硬件。ADB 来自该目录的 platform-tools/adb 或 PATH。第三方二进制未随包分发。

除下方两个内核模块外，还需同一固定版本的 `mavo-pcm-bridge.armv7`，SHA-256 为 `88d47c15e61d1428a59c821fed804c2e6490e82859a085062f21966b58d167fc`。

后续浏览器双向通话验证已完成（2026-09-12）：本机浏览器通过 getUserMedia / setSinkId 将 MacBook 麦克风送至模块 AS Interface，将模块 AC Interface 送至 MacBook 扬声器。设备先报告响铃，再报告通话中；测试者分别对电脑和手机说话后明确反馈“两边都清楚”。挂断后网页显示无语音通话、音频已断开。未保存录音。这是本台 QDC507GLEFM21 的短时实际双向验证，尚未覆盖长时间通话、所有浏览器、蓝牙耳机或其他固件。下面合成提示测试为较早阶段，保留其证据边界。

在用户明确授权的第二次电话测试中，AT 先报告呼叫中再报告接通，接听者确认听清英文合成测试提示。电脑向模块播放 8 kHz / 16-bit / mono PCM 成功。模块回传的 12 秒采样包含 96,768 个样本，RMS -37.31 dBFS、峰值 -5.99 dBFS，没有保存通话录音。这里只证明回传非零，尚未验证电脑扬声器可听质量或浏览器实时全双工表现。第一次测试虽然 AT 报告接通，用户未收到来电，不能作为成功证据。所有测试均已挂断，不内置号码或自动拨号。

本地新增可选的准备、心跳续期、停止恢复接口：固定文件哈希、固件与 USB 标识校验、单设备及启动 ID 绑定、本机同源限制、随机会话令牌。设备端独立看门狗在失去续期约 45 秒后恢复 USB；会话上限一小时。第三方运行文件由操作者在本机提供，不进入仓库或发行包。已验证手动停止恢复，以及不续心跳后状态进入 closed、USB 功能恢复 diag,serial,rmnet,ffs；长期稳定性仍需持续验证。

以下保留较早阶段的结论，不能混同最新测试范围。

## 当前结论

通话控制与声音传输是两条不同路径。USB 网卡本身不能当作麦克风；需要模块将通话音频导出为 PCM 或 USB 音频。当前不能靠选择 MacBook 麦克风作为“模块设备”实现通话。

最初只读研究未加载驱动。随后用户明确接受继续实验，进行了下述临时加载测试；没有刷写固件或拨打电话。

## 后续受控加载测试（2026-09-12）

- 加载前确认 root ADB、内核 3.18.44、无已加载模块、无声卡、无当前通话。
- 从上述 MaVo 固定提交下载两个模块；Mac 与设备端 SHA-256 均与表中一致。
- 文件仅放在设备 `/tmp/dj4hub-voice-audit-20260912`，没有安装启动项。
- APR 和 voice 模块均进入 Live 状态；ALSA card 0 出现，包含 VoLTE D4、AFE-PROXY D5 播放和 D6 采集。
- 设备节点经过短暂延迟出现，不能只依据 insmod 后立即执行的 ls 判断失败。
- 加载后 AT 通话状态查询仍成功返回空列表，未观察到设备重启。
- 日志包含 APR IPC log context 创建失败，以及多项 ASoC DAPM 路由和 debugfs 创建错误。尚未判断哪些是裁剪声卡的非关键告警，哪些影响实际通话。
- 未开启 UAC、未启动音频流、未采集麦克风，未验证非零样本或双向声音。
- 因这些异常停止进一步启用音频，使用重启清除临时驱动，不尝试可能存在 DSP 回调风险的热卸载。
- 重启后复核：`/proc/modules` 为空、ALSA 恢复无声卡、audio_enable 为 0、设备临时文件已清除，AT 状态查询成功。实验用本地 ADB server 已停止，此前授权启用的设备 ADB 接口保持不变。

这证明该二进制能够在本次设备上创建内部声卡，并不证明声音已通或驱动适合自动集成。

## 校准及临时 UAC 枚举测试（2026-09-12）

- 固定版本 PCM helper 的 SHA-256 为 `88d47c15e61d1428a59c821fed804c2e6490e82859a085062f21966b58d167fc`；`--check` 成功解析设备厂商库符号。
- 在没有校准时，15 秒限时 `--voice-route-session` 能使 D4 收发端进入 RUNNING，但内核报告缺少 cal 2/3/4/7 和音量命令 ADSP_EFAILED。RUNNING 不能等同于正常音频。
- 使用设备自带 `alsaucm_test`，选择 Tomtom I2S / VoLTE / Auxpcm Rx、Tx，出现 `Sent VocProc Cal!`。仍存在部分 AFE/ANC 数据缺失提示，不应隐藏。
- 校准后第二次 10 秒限时路由测试仍可启动；本次启动期间没有新增上述缺校准及音量失败日志。未验证实际采样。
- 原 USB functions 为 `diag,serial,rmnet,ffs`，没有 audio。仅设置 audio_enable=1 不会让 Mac 枚举声卡。
- 通过 sysfs 临时将 functions 改为 `diag,serial,rmnet,ffs,audio`（不改 AT 持久配置），使用 30 秒自动恢复脚本保护。
- macOS `system_profiler SPAudioDataType` 实测出现 BAIWANG `AC Interface` 输入和 `AS Interface` 输出，均为 USB、8 kHz、单声道。Mac 内建默认输入/输出未改变。
- 没有拨号、录音或使用电脑麦克风。USB 枚举成功与实际双向语音成功是不同验收项。
- 预设自动恢复脚本在随后检查时未恢复 functions；不能依赖 ADB 启动的后台 shell 在 USB 重枚举后可靠执行清理。改用设备重启清理；后续集成必须另有可验证的恢复机制。
- 重启后已确认 functions 恢复 `diag,serial,rmnet,ffs`、audio_enable=0、无加载模块及声卡，AT 状态接口正常；本地实验 ADB server 已停止。

## 音频流与恢复复测（2026-09-12）

采用 `setsid` 脱离 ADB 会话后，12 秒限时 USB 枚举实验成功自行恢复：设备日志记录了 timer-complete、restore-start 和 restore-confirmed，functions 与 enable 的读取值及 AT 查询均正常。单次成功不能代替故障注入与长期稳定性测试。

随后进行了 70 秒完整会话，顺序为加载驱动、厂商校准、启动 VoLTE hostless 路由、临时添加 USB audio。到期停止本次创建的 helper 和校准进程，关闭 audio_enable，恢复原 USB functions。结果：

- Mac 从明确选中的 `AC Interface` 读取 3 秒、8 kHz 单声道音频；分析器报告 24,576 个样本，全部为零。未发生实际通话，这只证明数据流可读，不证明下行声音正常。
- 独立 CoreAudio 探针按名称 `AS Interface` 和厂商 `BAIWANG` 唯一匹配输出，再核对 AudioQueue 的设备 UID，发送 3 秒数字静音，返回成功。没有输出到系统默认扬声器。
- 设备 D5 播放和 D6 采集均进入 RUNNING，硬件指针推进。USB/PCM 传输链路已得到比“声卡出现”更强的证据。
- 70 秒后日志出现 cleanup-confirmed，D4 收发均为 closed、audio_enable=0、原 functions 恢复，AT 查询正常。
- 同一次开机内再次建立 15 秒会话，Mac 同时向模块发送数字静音并读取模块音频，两个进程均正常完成。仍然没有使用电脑麦克风或保存录音。
- 第二次会话也自动 cleanup-confirmed，收发通道关闭，AT usbcfg 持久配置未改变。本次开机日志未匹配到 BUG/Oops/panic/Unable to handle、ADSP_EFAILED 或 Voice_get_cal failed。最后重启并复核无已加载模块、无声卡、原 USB functions 和 audio_enable=0，结束实验 ADB server。

正式功能的剩余验收：授权测试号码的实际双向通话、DTMF/挂断、运行中拔插及故障恢复。不能将空闲全零音频或 RUNNING 状态作为真实通话成功的标准；在这些验收通过前不自动安装第三方运行文件，也不宣称软件音频功能已完成。

## 授权号码拨测：未通过（2026-09-12）

向用户提供的测试号码发起一次拨号，设置最长 45 秒后自动挂断。AT 报告 active，Mac 向明确选中的 BAIWANG 输出发送合成测试提示，同时仅对模块输入进行 12 秒实时统计，不保存录音、不采集电脑麦克风。下行约 96,768 个样本出现非零信号（RMS 约 -14 dBFS，峰值达到满幅），但用户明确反馈手机没有收到来电，要求暂停换卡。

因此不能将本次 active 或非零音频认定为对端接听或双向语音成功；可能是运营商提示音，未做内容识别，原因未确认。自动挂断已完成，AT 确认无通话。暂停进一步拨号与正式集成，等待换卡后的测试。

## 此前实机只读结果

设备为 QDC507GLEFM21，内核为 3.18.44。用户授权启用 ADB 后的检查发现：

- 存在厂商音频库 `libql_lib_audio.so.1` 和 `/dev/ttyGS0`。
- 存在 `/sys/class/android_usb/f_audio/audio_enable`，读取值为 0。
- 存在 `alsaucm_test` 和 `/run/voc_svr`。
- `/proc/asound/cards` 无声卡；`/proc/asound/pcm` 为空；`/dev/snd` 仅有 timer。

这些是当时的设备快照，不保证设备重新插拔后仍保持相同状态。厂商库和 USB 端点存在，不等于内部 PCM 音频链路已就绪。

## 公开实现溯源

MaVo 固定版本：`0443dfdaf8aec086fd76ba2ee9152fd908114524`。

本轮检查了 CellDock main 的文件树、模块报告，以及以下两个文件的 SHA-256；它们与上述 MaVo 版本一致：

| 文件 | SHA-256 |
| --- | --- |
| qdc507_aprv3.ko | `3d82d3dec4f1e323201bba87156df9d41438e08314097353f2607f9117211d4a` |
| qdc507_voice.ko | `ed3821682d5309969a01c764192c83feff9669c61ef237c69475cd1619cf296c` |

因此它们不是两套独立的驱动兼容性证据。CellDock main 是可变引用，后续应重新核验。

检查到的两个项目公开树有用户态 PCM bridge 源码，但未找到与这两个内核模块对应的完整适配源码、补丁、配置和构建输入。不能据此断言作者未在其他位置发布。

另一个重要缺口：公开 MODULE-REPORT 描述的是旧版 `qdc507_afe.ko`，不是当前打包的 `qdc507_voice.ko`。报告记载旧版初始化曾触发 NULL dereference；修订后的构建仍需实机验证。不能将这份旧报告视为当前二进制的安全验证。

## 自行构建的可行性边界

公开基础内核提交 `82ed00908b3e8efc3ff0de27d2b5a7c0524ecd7f` 包含 `mdm9607.c`、Q6 AFE、voice、hostless PCM 等基础代码。

但其 Makefile 将部分音频对象设为内建（`obj-y`），并未提供现成的 QDC507 APR/voice 模块构建目标。报告还提到了自定义初始化封装、声卡索引固定、设备树绑定和错误回滚修改。仅编译公开内核不能复现当前两个模块。

下一步所需材料：

1. 与当前二进制版本一致的适配源码、补丁和 Makefile。
2. 实际内核配置、符号表及可复现工具链。
3. 离线构建、导入符号和初始化/退出路径审查。
4. 最后才是受控的设备内存加载、非零 PCM 样本和双向通话验证。

内核版本字符串相同、符号名称匹配，都不能单独证明私有 ABI 或 DSP 路由兼容。即使不刷写固件，加载不兼容驱动也可能使模块崩溃。当前不应自动下载并加载这些文件，也不应向用户宣称电脑通话音频已可用。

## 来源

- [MaVo 固定版本运行文件](https://github.com/moluncn/mavo/tree/0443dfdaf8aec086fd76ba2ee9152fd908114524/Resources/ModuleVoice)
- [CellDock 模块运行文件](https://github.com/celldock/celldock-for-mac/tree/main/Resources/ModuleVoice)
- [CellDock 模块报告](https://github.com/celldock/celldock-for-mac/blob/main/docs/MODULE-REPORT.md)
- [CellDock 第三方说明](https://github.com/celldock/celldock-for-mac/blob/main/docs/THIRD_PARTY_NOTICES.md)
- [公开基础内核](https://github.com/the-modem-distro/quectel_eg25_kernel/tree/82ed00908b3e8efc3ff0de27d2b5a7c0524ecd7f)
