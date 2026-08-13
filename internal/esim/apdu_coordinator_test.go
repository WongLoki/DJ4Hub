package esim

import (
	"context"
	"sync"
	"testing"

	"github.com/WongLoki/dj-4g-hub/internal/apduarbiter"
)

func TestAPDUCoordinatorChanMuIsStablePerChannel(t *testing.T) {
	c := newAPDUCoordinator("TEST")
	a := c.getOrCreateChanMu(3)
	b := c.getOrCreateChanMu(3)
	if a != b {
		t.Fatal("getOrCreateChanMu(3) 应返回同一把锁")
	}
	if c.getOrCreateChanMu(4) == a {
		t.Fatal("不同 channel 应是不同锁")
	}
}

func TestAPDUCoordinatorSessionRegistry(t *testing.T) {
	c := newAPDUCoordinator("TEST")
	if c.hasSession(2) {
		t.Fatal("未绑定时 hasSession 应为 false")
	}
	c.bindSession(2, "esim")
	if !c.hasSession(2) {
		t.Fatal("绑定后 hasSession 应为 true")
	}
	if _, ok := c.takeSession(2); !ok {
		t.Fatal("takeSession 应取出已绑定会话")
	}
	if c.hasSession(2) {
		t.Fatal("takeSession 后会话应被移除")
	}
}

func TestAPDUCoordinatorAcquireLeaseNilArbiterReturnsNil(t *testing.T) {
	c := newAPDUCoordinator("MBIM")
	lease, err := c.acquireLease(context.Background(), 0, "owner", apduarbiter.APDUClassEUICCWrite, 0, apduarbiter.TransportScopeExclusive)
	if err != nil {
		t.Fatalf("nil arbiter 不应报错: %v", err)
	}
	if lease != nil {
		t.Fatal("nil arbiter 应返回 nil 租约(退化为仅互斥)")
	}
}

func TestAPDUCoordinatorAcquireLeaseUsesArbiterAndMode(t *testing.T) {
	c := newAPDUCoordinator("MBIM")
	arb := apduarbiter.New("test-dev", apduarbiter.Options{MaxSessions: 3, MaxQMITransports: 3})
	c.setArbiter(arb)
	lease, err := c.acquireLease(context.Background(), 0, "esim_session_open", apduarbiter.APDUClassEUICCWrite, 0, apduarbiter.TransportScopeExclusive)
	if err != nil {
		t.Fatalf("acquireLease 失败: %v", err)
	}
	if lease == nil {
		t.Fatal("有 arbiter 时应返回非 nil 租约")
	}
	lease.Release()
}

var _ = sync.Mutex{}
