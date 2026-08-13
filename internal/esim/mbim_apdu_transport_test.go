package esim

import (
	"context"
	"testing"

	"github.com/WongLoki/dj-4g-hub/internal/apduarbiter"
)

type fakeUICC struct {
	openedAID []byte
	channel   uint32
	lastCmd   []byte
}

func (f *fakeUICC) ControlDevice() string { return "/dev/cdc-wdm0" }
func (f *fakeUICC) OpenChannel(_ context.Context, aid []byte) (uint32, error) {
	f.openedAID = aid
	f.channel = 1
	return 1, nil
}
func (f *fakeUICC) CloseChannel(_ context.Context, ch uint32) error { return nil }
func (f *fakeUICC) TransmitAPDU(_ context.Context, ch uint32, cmd []byte) ([]byte, error) {
	f.lastCmd = cmd
	return []byte{0x90, 0x00}, nil
}

func TestMBIMAPDUTransport(t *testing.T) {
	u := &fakeUICC{}
	tr := NewMBIMAPDUTransport(u)
	if tr.ControlDevice() != "/dev/cdc-wdm0" {
		t.Fatalf("ControlDevice = %q", tr.ControlDevice())
	}
	aid := []byte{0xA0, 0x00, 0x00, 0x05, 0x59, 0x10, 0x10, 0xFF, 0xFF, 0xFF, 0xFF, 0x89, 0x00, 0x00, 0x01, 0x00}
	ch, err := tr.OpenEUICCLogicalChannel(context.Background(), 0, aid)
	if err != nil {
		t.Fatalf("OpenEUICCLogicalChannel: %v", err)
	}
	resp, err := tr.TransmitEUICCAPDU(context.Background(), 0, ch, []byte{0x80, 0xE2})
	if err != nil {
		t.Fatalf("TransmitEUICCAPDU: %v", err)
	}
	if len(resp) != 2 || resp[0] != 0x90 {
		t.Fatalf("resp = %x", resp)
	}
	if err := tr.CloseEUICCLogicalChannel(context.Background(), 0, ch); err != nil {
		t.Fatalf("CloseEUICCLogicalChannel: %v", err)
	}
	var _ QMIAPDUTransport = tr
}

func TestMBIMAPDUTransportAcquiresExclusiveLease(t *testing.T) {
	f := &fakeUICC{}
	tr := NewMBIMAPDUTransport(f)
	arb := apduarbiter.New("mbim-dev", apduarbiter.Options{MaxSessions: 3, MaxQMITransports: 3})
	tr.SetAPDUArbiter(arb)

	ch, err := tr.OpenEUICCLogicalChannel(context.Background(), 1, []byte{0xA0})
	if err != nil {
		t.Fatalf("OpenEUICCLogicalChannel: %v", err)
	}
	if !tr.coord.hasSession(ch) {
		t.Fatal("打开通道后应登记会话")
	}
	if _, err := tr.TransmitEUICCAPDU(context.Background(), 1, ch, []byte{0x00, 0xA4}); err != nil {
		t.Fatalf("TransmitEUICCAPDU: %v", err)
	}
	if err := tr.CloseEUICCLogicalChannel(context.Background(), 1, ch); err != nil {
		t.Fatalf("CloseEUICCLogicalChannel: %v", err)
	}
	if tr.coord.hasSession(ch) {
		t.Fatal("关闭通道后会话应被移除")
	}
}

func TestMBIMAPDUTransportForwardsArbiterToSource(t *testing.T) {
	f := &arbiterAwareFakeUICC{}
	tr := NewMBIMAPDUTransport(f)
	arb := apduarbiter.New("mbim-dev", apduarbiter.Options{MaxSessions: 3, MaxQMITransports: 3})
	tr.SetAPDUArbiter(arb)
	if f.gotArbiter != arb {
		t.Fatal("SetAPDUArbiter 应转发给底层 src")
	}
}

func TestMBIMAPDUTransportIsArbiterAware(t *testing.T) {
	var _ interface {
		SetAPDUArbiter(*apduarbiter.Arbiter)
	} = (*MBIMAPDUTransport)(nil)
}

type arbiterAwareFakeUICC struct {
	fakeUICC
	gotArbiter *apduarbiter.Arbiter
}

func (f *arbiterAwareFakeUICC) SetAPDUArbiter(a *apduarbiter.Arbiter) { f.gotArbiter = a }
