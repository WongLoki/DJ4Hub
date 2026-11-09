package main

import (
	"log"
	"time"

	"github.com/WongLoki/DJ4Hub/internal/modem"
)

// 休眠唤醒 + USB 重枚举后，模块内部 ECM 桥接可能卡死（2026-10-17 实测 en11
// 链路 inactive 挂约 7 小时、状态一直 recovering），此时 AT 层仍正常、重下发
// AT+QCFG="usbnet" 无效，只有 AT+CFUN=1,1 重启模块才能恢复。linkWatchdog 负责
// 判断链路是否卡死到需要自动重启模块。
const (
	// linkStuckGrace 卡死断言生效所需的持续时长：唤醒后正常 DHCP / USB 重新
	// 枚举恢复远小于此值，而网桥卡死会一直持续。断言生效后立即复位模块。
	linkStuckGrace = 20 * time.Second
	// moduleRebootInFlight AT+CFUN=1,1 复位进行中的抑制窗口（实测约 45 秒后
	// 链路恢复）。只防复位下发途中重复触发，不设次数/间隔上限——首次复位
	// 没修好时必须允许立刻再试，限流只会放大恢复失败。
	moduleRebootInFlight = 60 * time.Second
	// linkWakeWindow 唤醒观察窗：唤醒钩子触发后，deadline 内链路未恢复到
	// ready 即视为卡死，签名满足立即复位，不再等断言窗（唤醒后等待不可接受）。
	linkWakeWindow = 25 * time.Second
	// atReopenDebounce 重开 AT 桥的防抖窗口：重开桥需要时间，窗口内不重复重开。
	atReopenDebounce = 60 * time.Second
	// probeIntervalActive 唤醒观察期的采样周期。平时不采样：采样只服务唤醒
	// 场景，无常驻轮询。
	probeIntervalActive = 3 * time.Second
	// wakeSampleCap 唤醒采样的兜底上限：无人值守时防止采样无限延长。
	wakeSampleCap = 3 * time.Minute
)

// linkAction 看门狗对一次采样给出的分层动作：每个错误场景直接判定到唯一
// 便宜的 fallback，只有「AT 活 + 附着 + 链路死」才允许复位模块。
type linkAction int

const (
	linkActionNone       linkAction = iota // 状态正常或与模块无关，无需动作
	linkActionWait                         // 恢复中/注册中/重枚举中，继续观察
	linkActionReopenAT                     // AT 桥失效：先重开 AT 桥（轻 fallback）
	linkActionRebootModule                 // 半瘫断言生效或唤醒观察窗超时：复位模块
)

func (a linkAction) String() string {
	switch a {
	case linkActionWait:
		return "wait"
	case linkActionReopenAT:
		return "reopen_at"
	case linkActionRebootModule:
		return "reboot_module"
	default:
		return "none"
	}
}

// linkProbe 一次链路采样：网络服务状态 + 模块 USB/AT/PS 层的存活信号。
// 半瘫断言（ECM 网桥卡死）= ModulePresent && ATAlive && PSAttached 且
// State 一直为 recovering：模块固件活着、蜂窝附着正常，只有主机侧网桥死。
type linkProbe struct {
	State         string
	ModulePresent bool
	ATAlive       bool // AT 通道有响应
	PSAttached    bool // AT+CGATT? 返回 1
	ATSkipped     bool // 通话期间未探测 AT，不能把 ATAlive=false 当作 AT 桥失效
}

type linkWatchdog struct {
	stuckSince   time.Time // 连续处于卡死半瘫状态的起点
	rebootUntil  time.Time // 自动复位进行中的抑制截止时间
	reopenUntil  time.Time // 重开 AT 桥的防抖截止时间
	wakeUntil    time.Time // 唤醒观察窗 deadline（零值 = 不处于唤醒观察窗）
	recoveryFrom time.Time // 自动复位已下发、等待链路恢复的起点（零值 = 无进行中恢复）
	// wakeSamplerRunning 唤醒采样 goroutine 是否在跑（防重入，重复唤醒只
	// 延长观察窗，不另起采样）。
	wakeSamplerRunning bool
}

// decideLinkAction 把一次采样判定到唯一便宜的 fallback（纯决策函数，便于测试）。
// stuckFor 是半瘫签名已持续的时长；postWake 表示唤醒观察窗 deadline 已过。
func decideLinkAction(probe linkProbe, stuckFor time.Duration, postWake bool) linkAction {
	// error/missing 是服务被禁用之类的硬故障，与模块无关，不碰模块。
	if probe.State != networkStateReady && probe.State != networkStateRecovering {
		return linkActionNone
	}
	// USB 重枚举中/已拔出：等桥重新打开，重启模块无意义。
	if !probe.ModulePresent {
		return linkActionWait
	}
	// AT 桥失效：先重开 AT 桥（轻 fallback），不要上来就重启模块。
	// 通话期间没探测 AT，不能把 ATAlive=false 当作桥失效。
	if !probe.ATAlive {
		if probe.ATSkipped {
			return linkActionWait
		}
		return linkActionReopenAT
	}
	// AT 通但 PS 未附着：蜂窝注册中，等待是正确动作。
	if !probe.PSAttached {
		return linkActionWait
	}
	if probe.State == networkStateReady {
		return linkActionNone
	}
	// 半瘫签名（模块固件活着 + 已附着 + 链路死）：唤醒观察窗 deadline 已过
	// 立即复位；否则等断言窗确认持续卡死再复位。
	if postWake || stuckFor >= linkStuckGrace {
		return linkActionRebootModule
	}
	return linkActionWait
}

// observeWake 唤醒钩子：开启唤醒观察窗并给恢复一个全新观察期，避免把唤醒
// 过程中短暂的 recovering 误判为链路卡死。
func (w *linkWatchdog) observeWake(now time.Time) {
	w.stuckSince = time.Time{}
	w.wakeUntil = now.Add(linkWakeWindow)
}

// Observe 喂入一次网络状态采样；返回应当执行的分层动作。
// 时钟由调用方以 now 注入，便于测试。
func (w *linkWatchdog) Observe(probe linkProbe, now time.Time) linkAction {
	// 链路正常：清掉卡死窗口与唤醒观察窗，恢复正常采样节奏。
	if probe.State == networkStateReady {
		w.stuckSince = time.Time{}
		w.wakeUntil = time.Time{}
		return linkActionNone
	}
	stuck := probe.State == networkStateRecovering && probe.ModulePresent &&
		probe.ATAlive && probe.PSAttached
	if !stuck {
		w.stuckSince = time.Time{}
	} else if w.stuckSince.IsZero() {
		w.stuckSince = now
	}
	stuckFor := time.Duration(0)
	if !w.stuckSince.IsZero() {
		stuckFor = now.Sub(w.stuckSince)
	}
	postWake := !w.wakeUntil.IsZero() && !now.Before(w.wakeUntil)
	action := decideLinkAction(probe, stuckFor, postWake)
	switch action {
	case linkActionReopenAT:
		// 重开桥需要时间：防抖窗口内不重复重开。
		if now.Before(w.reopenUntil) {
			return linkActionWait
		}
		w.reopenUntil = now.Add(atReopenDebounce)
		return linkActionReopenAT
	case linkActionRebootModule:
		// 复位进行中不再重复触发；窗口一过立即允许再复位。
		if now.Before(w.rebootUntil) {
			return linkActionWait
		}
		w.rebootUntil = now.Add(moduleRebootInFlight)
		w.stuckSince = time.Time{}
		return linkActionRebootModule
	}
	return action
}

// noteNetworkProbe 将一次网络状态采样喂给链路看门狗，并执行判定出的分层
// 动作；链路恢复后补一条自动恢复留痕（写入通信历史，UI 可见）。
func (a *app) noteNetworkProbe(probe linkProbe) linkAction {
	a.linkWatchdogMu.Lock()
	action := a.linkWD.Observe(probe, time.Now())
	a.linkWatchdogMu.Unlock()
	switch action {
	case linkActionRebootModule:
		if a.enforceLinkWatchdog() {
			a.linkWatchdogMu.Lock()
			a.linkWD.recoveryFrom = time.Now()
			a.linkWatchdogMu.Unlock()
		}
	case linkActionReopenAT:
		a.reopenUSBATBridge()
	}
	if probe.State == networkStateReady {
		a.closeAutoRecovery("recovered", "ECM 网桥卡死，自动复位模块（AT+CFUN=1,1）后链路恢复", time.Now())
	}
	return action
}

// closeAutoRecovery 若有等待恢复的自动复位事件，写入通信历史并清零。
func (a *app) closeAutoRecovery(state, content string, ended time.Time) {
	a.linkWatchdogMu.Lock()
	started := a.linkWD.recoveryFrom
	a.linkWD.recoveryFrom = time.Time{}
	a.linkWatchdogMu.Unlock()
	if started.IsZero() {
		return
	}
	if err := a.appendHistory(newAutoRecoveryRecord(state, content, started, ended)); err != nil {
		log.Printf("自动恢复留痕保存失败：%v", err)
	}
}

// noteWake 唤醒钩子：开启唤醒观察窗（deadline 内链路未恢复即判卡死），
// 并给 DHCP / USB 重新枚举一个全新的观察期；同时启动唤醒采样（3 秒周期）。
// 采样只在唤醒观察期运行，链路恢复即收工，平时无常驻轮询。
func (a *app) noteWake() {
	if a.demo {
		return
	}
	a.linkWatchdogMu.Lock()
	a.linkWD.observeWake(time.Now())
	start := !a.linkWD.wakeSamplerRunning
	if start {
		a.linkWD.wakeSamplerRunning = true
	}
	a.linkWatchdogMu.Unlock()
	if start {
		go a.runWakeSampler()
	}
}

// reopenUSBATBridge AT 桥失效时的轻 fallback：释放旧句柄并立即重开。
// 看门狗侧已有防抖，这里不再走断连冷却。
func (a *app) reopenUSBATBridge() {
	if a.usbAT != nil {
		log.Printf("link watchdog: AT 桥无响应，释放旧句柄并重开")
		a.usbAT.Close()
		a.usbAT = nil
	}
	a.usbATBackoffUntil = time.Time{}
	a.usbATBackoffErr = ""
	if err := a.ensureUSBAT(); err != nil {
		log.Printf("link watchdog: 重开 AT 桥失败：%v", err)
	}
}

// linkProbeFrom 从一次诊断/采样的 AT 结果组装看门狗采样：AT 层任一命令有响应
// 即认为模块 AT 层存活；PS 附着以 AT+CGATT? 解析结果为准。
func linkProbeFrom(state string, modulePresent bool, raw map[string]string) linkProbe {
	probe := linkProbe{State: state, ModulePresent: modulePresent}
	if resp, ok := raw["cgatt"]; ok {
		probe.ATAlive = true
		probe.PSAttached, _ = modem.ParseCGATT(resp)
		return probe
	}
	if len(raw) > 0 {
		probe.ATAlive = true
	}
	return probe
}

// wakeSampleDone 判定唤醒采样是否收工（纯函数，便于测试）：链路恢复且恢复
// 留痕已落定即停，绝不演变成平时的常驻轮询；兜底上限防止无人值守时无限采样。
func wakeSampleDone(probe linkProbe, recoveryPending bool, elapsed time.Duration) bool {
	if probe.State == networkStateReady && !recoveryPending {
		return true
	}
	return elapsed >= wakeSampleCap
}

// newAutoRecoveryRecord 构造一条自动恢复留痕（写入通信历史，UI 可见）。
func newAutoRecoveryRecord(state, content string, started, ended time.Time) historyRecord {
	end := ended
	return historyRecord{
		Kind:     "auto_recovery",
		Content:  content,
		State:    state,
		Started:  started,
		Ended:    &end,
		Duration: int64(ended.Sub(started).Seconds()),
	}
}

// runWakeSampler 唤醒观察期的链路采样：3 秒周期喂给看门狗，链路恢复即收工
// （wakeSampleDone 判定），绝不演变成平时的常驻轮询。恢复中持续采样，确保
// CFUN=1,1 复位后链路真正恢复并落下恢复留痕。
func (a *app) runWakeSampler() {
	defer func() {
		a.linkWatchdogMu.Lock()
		a.linkWD.wakeSamplerRunning = false
		a.linkWatchdogMu.Unlock()
	}()
	start := time.Now()
	ticker := time.NewTicker(probeIntervalActive)
	defer ticker.Stop()
	for range ticker.C {
		probe := a.sampleLinkProbe()
		a.noteNetworkProbe(probe)
		a.linkWatchdogMu.Lock()
		pending := !a.linkWD.recoveryFrom.IsZero()
		a.linkWatchdogMu.Unlock()
		if wakeSampleDone(probe, pending, time.Since(start)) {
			return
		}
	}
}

// sampleLinkProbe 采样一次链路状态（网络服务状态 + 模块 AT/PS 存活信号）。
func (a *app) sampleLinkProbe() linkProbe {
	device := a.currentUSBDevice()
	product := ""
	if device != nil {
		product = device.Product
	}
	service := currentDJINetworkService(discoverMacNetworkServices(), discoverMacNetworkInterfaces(), product)
	state := networkServiceState(service)
	present := service != nil

	// 通话/音频会话期间不占用 AT 通道：标记 ATSkipped，不能当作 AT 桥失效。
	a.audioMu.Lock()
	audioBusy := a.audioSession != nil && time.Since(a.audioSession.lastLease) < 50*time.Second
	a.audioMu.Unlock()
	if audioBusy {
		return linkProbe{State: state, ModulePresent: present, ATSkipped: true}
	}

	if resp, err := a.runATCommand("AT+CGATT?", 3*time.Second); err == nil {
		attached, _ := modem.ParseCGATT(resp)
		return linkProbe{State: state, ModulePresent: present, ATAlive: true, PSAttached: attached}
	}
	return linkProbe{State: state, ModulePresent: present}
}
