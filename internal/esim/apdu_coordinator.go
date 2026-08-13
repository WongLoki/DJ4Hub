package esim

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/WongLoki/dj-4g-hub/internal/apduarbiter"
	"github.com/WongLoki/dj-4g-hub/pkg/logger"
)

type apduSessionInfo struct {
	Channel  byte
	Owner    string
	OpenedAt time.Time
}

// apduCoordinator 提供 eUICC APDU 传输共用的串行化与仲裁:
// 按通道互斥 + apduarbiter 租约 + 逻辑通道会话登记。QMI/MBIM 两个传输共用。
type apduCoordinator struct {
	mode string

	chanMuMu sync.RWMutex
	chanMu   map[byte]*sync.Mutex

	leaseMu  sync.Mutex
	arbiter  *apduarbiter.Arbiter
	sessions map[byte]apduSessionInfo
}

func newAPDUCoordinator(mode string) *apduCoordinator {
	return &apduCoordinator{
		mode:     strings.TrimSpace(mode),
		chanMu:   make(map[byte]*sync.Mutex),
		sessions: make(map[byte]apduSessionInfo),
	}
}

func (c *apduCoordinator) getOrCreateChanMu(channel byte) *sync.Mutex {
	c.chanMuMu.RLock()
	mu := c.chanMu[channel]
	c.chanMuMu.RUnlock()
	if mu != nil {
		return mu
	}
	c.chanMuMu.Lock()
	defer c.chanMuMu.Unlock()
	if mu = c.chanMu[channel]; mu != nil {
		return mu
	}
	mu = &sync.Mutex{}
	c.chanMu[channel] = mu
	return mu
}

// resetChanMu 清空所有 per-channel 锁(用于传输 Stop:确保没有飞行中的 Transmit 复用旧锁)。
func (c *apduCoordinator) resetChanMu() {
	c.chanMuMu.Lock()
	c.chanMu = make(map[byte]*sync.Mutex)
	c.chanMuMu.Unlock()
}

func (c *apduCoordinator) setArbiter(arbiter *apduarbiter.Arbiter) {
	c.leaseMu.Lock()
	defer c.leaseMu.Unlock()
	if c.sessions == nil {
		c.sessions = make(map[byte]apduSessionInfo)
	}
	if c.arbiter == arbiter {
		return
	}
	clear(c.sessions)
	c.arbiter = arbiter
}

func (c *apduCoordinator) acquireLease(ctx context.Context, timeout time.Duration, owner string, class apduarbiter.APDUClass, channel byte, scope apduarbiter.TransportScope) (*apduarbiter.Lease, error) {
	c.leaseMu.Lock()
	arbiter := c.arbiter
	c.leaseMu.Unlock()
	if arbiter == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return arbiter.AcquireTransport(ctx, apduarbiter.Request{
		Owner:   strings.TrimSpace(owner),
		Mode:    c.mode,
		Class:   class,
		Channel: int(channel),
		Scope:   scope,
	})
}

func (c *apduCoordinator) bindSession(channel byte, owner string) {
	c.leaseMu.Lock()
	defer c.leaseMu.Unlock()
	if c.sessions == nil {
		c.sessions = make(map[byte]apduSessionInfo)
	}
	c.sessions[channel] = apduSessionInfo{
		Channel:  channel,
		Owner:    strings.TrimSpace(owner),
		OpenedAt: time.Now(),
	}
}

func (c *apduCoordinator) hasSession(channel byte) bool {
	c.leaseMu.Lock()
	defer c.leaseMu.Unlock()
	_, ok := c.sessions[channel]
	return ok
}

func (c *apduCoordinator) takeSession(channel byte) (apduSessionInfo, bool) {
	c.leaseMu.Lock()
	defer c.leaseMu.Unlock()
	session, ok := c.sessions[channel]
	delete(c.sessions, channel)
	return session, ok
}

func (c *apduCoordinator) releaseAllSessions(controlDevice, reason string) {
	c.leaseMu.Lock()
	count := len(c.sessions)
	clear(c.sessions)
	c.leaseMu.Unlock()
	if count > 0 {
		logger.Warn("APDU logical session registry 已清理",
			"transport", c.mode,
			"control_device", strings.TrimSpace(controlDevice),
			"reason", strings.TrimSpace(reason),
			"session_count", count)
	}
}
