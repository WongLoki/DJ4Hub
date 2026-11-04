package main

import (
	"testing"
	"time"
)

// The tests in this file pin down the stuck-link auto recovery contract after a
// macOS sleep/wake cycle. The bug (2026-10-17): after wake + USB re-enumeration
// the module's ECM bridge can stay dead (en11 inactive, state "recovering")
// while AT still answers and PS stays attached; only AT+CFUN=1,1 rebooting the
// module brings the link back.
//
// Contract asserted here:
//   - 分层判定：每个错误场景直接判定到唯一便宜的 fallback（重开 AT 桥/等待/
//     复位模块），只有「AT 活 + 附着 + 链路死」才允许 CFUN=1,1
//   - 唤醒观察窗 deadline 一过，签名满足立即复位，不等满 20 秒
//   - 唤醒采样只在唤醒观察期运行（3 秒周期），恢复即收工；平时无常驻轮询
//   - 恢复过程写入通信历史（可观测）
const (
	// fastStuckWindow 卡死断言允许的最长持续时间（唤醒后正常 DHCP/重枚举恢复
	// 远小于此值）。
	fastStuckWindow = 20 * time.Second
	// wakeDeadline 唤醒观察窗：deadline 内链路未 ready 即判定卡死。
	wakeDeadline = 25 * time.Second
	// rebootWindow AT+CFUN=1,1 复位进行中的抑制窗口（实测约 45 秒后恢复）。
	rebootWindow = 60 * time.Second
)

type watchdogSample struct {
	probe    linkProbe
	at       time.Duration // 相对场景起点的时间偏移
	wantFire linkAction
}

func runWatchdogScenario(t *testing.T, samples []watchdogSample) {
	t.Helper()
	base := time.Date(2026, 10, 17, 16, 30, 0, 0, time.UTC)
	wd := &linkWatchdog{}
	for i, sample := range samples {
		got := wd.Observe(sample.probe, base.Add(sample.at))
		if got != sample.wantFire {
			t.Fatalf("sample %d (state=%q present=%t at=%t attached=%t at=%v): Observe() = %v, want %v",
				i, sample.probe.State, sample.probe.ModulePresent,
				sample.probe.ATAlive, sample.probe.PSAttached,
				sample.at, got, sample.wantFire)
		}
	}
}

// stuckProbe 半瘫状态：模块在位、AT 有响应、PS 已附着，但网卡链路一直 recovering。
func stuckProbe() linkProbe {
	return linkProbe{
		State:         networkStateRecovering,
		ModulePresent: true,
		ATAlive:       true,
		PSAttached:    true,
	}
}

func TestDecideLinkActionClassifiesErrorScenarios(t *testing.T) {
	tests := []struct {
		name     string
		probe    linkProbe
		stuckFor time.Duration
		postWake bool
		want     linkAction
	}{
		{
			// 服务被禁用等硬故障：与模块无关，不碰模块。
			name:  "hard error needs no module action",
			probe: linkProbe{State: networkStateError, ModulePresent: true, ATAlive: true, PSAttached: true},
			want:  linkActionNone,
		},
		{
			// USB 重枚举中/已拔出：等桥重新打开，重启模块无意义。
			name:  "usb renumbering waits",
			probe: linkProbe{State: networkStateRecovering},
			want:  linkActionWait,
		},
		{
			// AT 桥失效：先重开 AT 桥（轻 fallback），不要上来就重启模块。
			name:  "at bridge down reopens bridge first",
			probe: linkProbe{State: networkStateRecovering, ModulePresent: true},
			want:  linkActionReopenAT,
		},
		{
			// AT 通但 PS 未附着：蜂窝注册中，等待是正确动作。
			name:  "registration pending waits",
			probe: linkProbe{State: networkStateRecovering, ModulePresent: true, ATAlive: true},
			want:  linkActionWait,
		},
		{
			name:  "ready needs nothing",
			probe: linkProbe{State: networkStateReady, ModulePresent: true, ATAlive: true, PSAttached: true},
			want:  linkActionNone,
		},
		{
			// 半瘫签名未持续到断言窗：继续观察。
			name:     "stuck signature below grace keeps observing",
			probe:    stuckProbe(),
			stuckFor: fastStuckWindow - time.Second,
			want:     linkActionWait,
		},
		{
			// 半瘫签名持续满断言窗：重启模块是唯一手段。
			name:     "stuck signature at grace reboots module",
			probe:    stuckProbe(),
			stuckFor: fastStuckWindow,
			want:     linkActionRebootModule,
		},
		{
			// 唤醒观察窗 deadline 已过：不需要再等断言窗，立即复位。
			name:     "wake deadline passed reboots immediately",
			probe:    stuckProbe(),
			stuckFor: 0,
			postWake: true,
			want:     linkActionRebootModule,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := decideLinkAction(test.probe, test.stuckFor, test.postWake); got != test.want {
				t.Fatalf("decideLinkAction() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLinkWatchdogFiresWithinTwentySeconds(t *testing.T) {
	runWatchdogScenario(t, []watchdogSample{
		{probe: stuckProbe(), at: 0, wantFire: linkActionWait},
		{probe: stuckProbe(), at: fastStuckWindow - time.Second, wantFire: linkActionWait},
		// 断言生效（卡死状态持续 20 秒）后必须立即触发，不得再多等。
		{probe: stuckProbe(), at: fastStuckWindow, wantFire: linkActionRebootModule},
	})
}

func TestLinkWatchdogWakeDeadlineTriggersWithoutGrace(t *testing.T) {
	base := time.Date(2026, 10, 19, 19, 10, 0, 0, time.UTC)
	wd := &linkWatchdog{}
	// 唤醒钩子在 base 时刻触发，观察窗 deadline = 25 秒。
	wd.observeWake(base)
	// deadline 之前链路一直 recovering：正常 DHCP 可能还在路上，不触发。
	if got := wd.Observe(stuckProbe(), base.Add(10*time.Second)); got != linkActionWait {
		t.Fatalf("before deadline: Observe() = %v, want %v", got, linkActionWait)
	}
	// deadline（25s）已过、签名满足：立即复位，不等 20 秒断言窗。
	if got := wd.Observe(stuckProbe(), base.Add(wakeDeadline+time.Second)); got != linkActionRebootModule {
		t.Fatalf("after deadline: Observe() = %v, want %v", got, linkActionRebootModule)
	}
}

func TestLinkWatchdogReopenATIsCheapAndDebounced(t *testing.T) {
	base := time.Date(2026, 10, 19, 19, 10, 0, 0, time.UTC)
	wd := &linkWatchdog{}
	deadAT := linkProbe{State: networkStateRecovering, ModulePresent: true}
	// AT 桥失效：动作是重开 AT 桥，而不是重启模块。
	if got := wd.Observe(deadAT, base); got != linkActionReopenAT {
		t.Fatalf("first dead-AT sample: Observe() = %v, want %v", got, linkActionReopenAT)
	}
	// 重开桥需要一点时间，短窗口内不得重复重开（防抖）。
	if got := wd.Observe(deadAT, base.Add(10*time.Second)); got != linkActionWait {
		t.Fatalf("within reopen debounce: Observe() = %v, want %v", got, linkActionWait)
	}
	// 防抖期过后仍然 AT 不通：允许再次重开。
	if got := wd.Observe(deadAT, base.Add(61*time.Second)); got != linkActionReopenAT {
		t.Fatalf("after reopen debounce: Observe() = %v, want %v", got, linkActionReopenAT)
	}
}

func TestLinkWatchdogRetriesWithoutTenMinuteLockout(t *testing.T) {
	runWatchdogScenario(t, []watchdogSample{
		{probe: stuckProbe(), at: 0, wantFire: linkActionWait},
		{probe: stuckProbe(), at: fastStuckWindow, wantFire: linkActionRebootModule},
		// 复位进行中（约 45 秒恢复）不得重复触发。
		{probe: stuckProbe(), at: 40 * time.Second, wantFire: linkActionWait},
		{probe: stuckProbe(), at: rebootWindow - 10*time.Second, wantFire: linkActionWait},
		// 复位窗口过后仍卡死：首次复位没修好必须允许再触发。
		{probe: stuckProbe(), at: 95 * time.Second, wantFire: linkActionRebootModule},
	})
}

func TestLinkWatchdogReadyInterruptsStuckWindow(t *testing.T) {
	ready := linkProbe{State: networkStateReady, ModulePresent: true, ATAlive: true, PSAttached: true}
	runWatchdogScenario(t, []watchdogSample{
		{probe: stuckProbe(), at: 0, wantFire: linkActionWait},
		{probe: stuckProbe(), at: 10 * time.Second, wantFire: linkActionWait},
		// 链路自己恢复了：卡死窗口必须清零。
		{probe: ready, at: 15 * time.Second, wantFire: linkActionNone},
		// 新窗口重新计时，旧窗口的时长不得累计。
		{probe: stuckProbe(), at: 16 * time.Second, wantFire: linkActionWait},
		{probe: stuckProbe(), at: fastStuckWindow, wantFire: linkActionWait},
		{probe: stuckProbe(), at: 35 * time.Second, wantFire: linkActionWait},
		{probe: stuckProbe(), at: 36 * time.Second, wantFire: linkActionRebootModule},
	})
}

func TestLinkWatchdogIgnoresMissingModule(t *testing.T) {
	missing := linkProbe{State: networkStateRecovering, ATAlive: true, PSAttached: true}
	runWatchdogScenario(t, []watchdogSample{
		// 模块不在位（USB 重枚举中/已拔出）时重启无意义，不得触发。
		{probe: missing, at: 0, wantFire: linkActionWait},
		{probe: missing, at: 30 * time.Minute, wantFire: linkActionWait},
	})
}

func TestLinkWatchdogIgnoresHardError(t *testing.T) {
	hardError := linkProbe{State: networkStateError, ModulePresent: true, ATAlive: true, PSAttached: true}
	missingState := linkProbe{State: "missing", ModulePresent: true, ATAlive: true, PSAttached: true}
	runWatchdogScenario(t, []watchdogSample{
		// error（服务被禁用等硬故障）与 missing 不是链路卡死，重启模块解决不了。
		{probe: hardError, at: 0, wantFire: linkActionNone},
		{probe: hardError, at: 30 * time.Minute, wantFire: linkActionNone},
		{probe: missingState, at: 61 * time.Minute, wantFire: linkActionNone},
	})
}

// 唤醒采样只服务唤醒场景：链路恢复就收工，绝不演变成平时的常驻轮询；
// 兜底上限防止无人值守时无限采样。
func TestWakeSamplingStopsWhenRecovered(t *testing.T) {
	ready := linkProbe{State: networkStateReady, ModulePresent: true, ATAlive: true, PSAttached: true}
	if !wakeSampleDone(ready, false, 10*time.Second) {
		t.Fatal("ready link must end wake sampling")
	}
	if wakeSampleDone(ready, true, 10*time.Second) {
		t.Fatal("recovery trail still pending: keep sampling until it closes")
	}
	if wakeSampleDone(stuckProbe(), false, 10*time.Second) {
		t.Fatal("stuck link must keep sampling for wake recovery")
	}
	if !wakeSampleDone(stuckProbe(), false, wakeSampleCap) {
		t.Fatal("safety cap must end wake sampling")
	}
}

func TestAutoRecoveryHistoryRecord(t *testing.T) {
	started := time.Date(2026, 10, 19, 19, 11, 22, 0, time.UTC)
	ended := started.Add(28 * time.Second)
	record := newAutoRecoveryRecord("recovered", "ECM 网桥卡死自动恢复", started, ended)

	if record.Kind != "auto_recovery" {
		t.Fatalf("Kind = %q, want %q", record.Kind, "auto_recovery")
	}
	if record.State != "recovered" {
		t.Fatalf("State = %q, want %q", record.State, "recovered")
	}
	if record.Content == "" {
		t.Fatal("Content should describe the recovery for the UI")
	}
	if !record.Started.Equal(started) || record.Ended == nil || !record.Ended.Equal(ended) {
		t.Fatalf("Started/Ended mismatch: %+v", record)
	}
	if record.Duration != 28 {
		t.Fatalf("Duration = %d, want 28", record.Duration)
	}
}
