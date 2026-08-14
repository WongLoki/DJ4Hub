package esim

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/WongLoki/DJ4Hub/internal/apduarbiter"
	"github.com/WongLoki/DJ4Hub/pkg/mbim"
	qmiq "github.com/iniwex5/quectel-qmi-go/pkg/qmi"
)

func TestNewQMIUIMTransportWithOptionsStoresClientOptions(t *testing.T) {
	transport := NewQMIUIMTransportWithOptions("/dev/cdc-wdm0", qmiq.ClientOptions{
		UseProxy:        true,
		ProxyPath:       "custom-qmi-proxy",
		ProxyExecutable: "/opt/vohive/bin/qmi-proxy",
	})

	if !transport.clientOptions.UseProxy {
		t.Fatal("UseProxy=false, want true")
	}
	if transport.clientOptions.ProxyPath != "custom-qmi-proxy" {
		t.Fatalf("ProxyPath=%q, want custom-qmi-proxy", transport.clientOptions.ProxyPath)
	}
	if transport.clientOptions.ProxyExecutable != "/opt/vohive/bin/qmi-proxy" {
		t.Fatalf("ProxyExecutable=%q, want /opt/vohive/bin/qmi-proxy", transport.clientOptions.ProxyExecutable)
	}
	if got := transport.ControlDevice(); got != "/dev/cdc-wdm0" {
		t.Fatalf("ControlDevice()=%q, want /dev/cdc-wdm0", got)
	}
}

type qmiChannelTransportFake struct {
	controlDevice    string
	openChannel      byte
	transmitResp     []byte
	transmitErr      error
	openedAID        []byte
	closed           []byte
	transmits        []qmiChannelTransmitCall
	closeDeadline    time.Time
	transmitDeadline time.Time
}

type qmiChannelTransmitCall struct {
	slot    byte
	channel byte
	command []byte
}

func (f *qmiChannelTransportFake) ControlDevice() string {
	if f.controlDevice == "" {
		return "/dev/cdc-wdm0"
	}
	return f.controlDevice
}

func (f *qmiChannelTransportFake) OpenEUICCLogicalChannel(ctx context.Context, slot byte, aid []byte) (byte, error) {
	f.openedAID = append([]byte(nil), aid...)
	if f.openChannel == 0 {
		f.openChannel = 2
	}
	return f.openChannel, nil
}

func (f *qmiChannelTransportFake) CloseEUICCLogicalChannel(ctx context.Context, slot byte, channel byte) error {
	f.closed = append(f.closed, channel)
	if deadline, ok := ctx.Deadline(); ok {
		f.closeDeadline = deadline
	}
	return nil
}

func (f *qmiChannelTransportFake) TransmitEUICCAPDU(ctx context.Context, slot byte, channel byte, command []byte) ([]byte, error) {
	if deadline, ok := ctx.Deadline(); ok {
		f.transmitDeadline = deadline
	}
	f.transmits = append(f.transmits, qmiChannelTransmitCall{
		slot:    slot,
		channel: channel,
		command: append([]byte(nil), command...),
	})
	if f.transmitErr != nil {
		return nil, f.transmitErr
	}
	if f.transmitResp == nil {
		return []byte{0x90, 0x00}, nil
	}
	return append([]byte(nil), f.transmitResp...), nil
}

func TestQMIUIMTransportAPDUSessionRegistryClearsSession(t *testing.T) {
	transport := NewQMIUIMTransportWithOptions("/dev/cdc-wdm0", qmiq.ClientOptions{})
	transport.bindAPDUSession(2, "test")

	if !transport.hasAPDUSession(2) {
		t.Fatal("hasAPDUSession()=false want true")
	}
	if _, ok := transport.takeAPDUSession(2); !ok {
		t.Fatal("takeAPDUSession() ok=false want true")
	}
	if transport.hasAPDUSession(2) {
		t.Fatal("session remained in registry")
	}
}

func TestQMIUIMTransportAcquireAPDUTransportLeaseAllowsConcurrentQMIChannels(t *testing.T) {
	transport := NewQMIUIMTransportWithOptions("/dev/cdc-wdm0", qmiq.ClientOptions{})
	transport.SetAPDUArbiter(apduarbiter.New("dev-qmi", apduarbiter.Options{MaxQMITransports: 3}))

	first, err := transport.acquireAPDUTransportLease(
		context.Background(),
		time.Second,
		"profile-a",
		apduarbiter.APDUClassEUICCWrite,
		2,
		apduarbiter.TransportScopeQMIChannel,
	)
	if err != nil {
		t.Fatalf("first acquire error=%v", err)
	}
	defer first.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	second, err := transport.acquireAPDUTransportLease(
		ctx,
		time.Second,
		"profile-b",
		apduarbiter.APDUClassEUICCWrite,
		3,
		apduarbiter.TransportScopeQMIChannel,
	)
	if err != nil {
		t.Fatalf("second acquire different channel error=%v", err)
	}
	defer second.Release()
}

func TestQMIChannelTransmitBeforeOpenFailsWithoutCallingTransport(t *testing.T) {
	transport := &qmiChannelTransportFake{}
	channel := NewQMIChannel(transport, 1)

	_, err := channel.Transmit([]byte{0x00, 0xA4})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "logical channel is not open") {
		t.Fatalf("error = %v, want logical channel is not open", err)
	}
	if len(transport.transmits) != 0 {
		t.Fatalf("TransmitEUICCAPDU calls = %#v, want none", transport.transmits)
	}
}

func TestQMIChannelTransmitUsesOpenedLogicalChannel(t *testing.T) {
	transport := &qmiChannelTransportFake{openChannel: 3, transmitResp: []byte{0x61, 0x10}}
	channel := NewQMIChannel(transport, 1)

	opened, err := channel.OpenLogicalChannel([]byte{0xA0, 0x00})
	if err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
	if opened != 3 {
		t.Fatalf("OpenLogicalChannel() = %d, want 3", opened)
	}
	resp, err := channel.Transmit([]byte{0x80, 0xE2})
	if err != nil {
		t.Fatalf("Transmit() error = %v", err)
	}
	if !reflect.DeepEqual(resp, []byte{0x61, 0x10}) {
		t.Fatalf("Transmit() = % X, want 61 10", resp)
	}
	if len(transport.transmits) != 1 || transport.transmits[0].channel != 3 {
		t.Fatalf("TransmitEUICCAPDU calls = %#v, want one call on channel 3", transport.transmits)
	}
}

func TestQMIChannelTransmitWrapsMBIMInvalidChannelStatusAsCardReset(t *testing.T) {
	transport := &qmiChannelTransportFake{
		openChannel: 3,
		transmitErr: &mbim.StatusError{Op: "UICC_APDU", Status: mbim.StatusMSInvalidLogicalChannel},
	}
	channel := NewQMIChannel(transport, 1)
	if _, err := channel.OpenLogicalChannel([]byte{0xA0, 0x00}); err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}

	_, err := channel.Transmit([]byte{0x80, 0xE2})
	if !errors.Is(err, ErrMBIMUICCInvalidChannel) {
		t.Fatalf("Transmit() error = %v, want wrapped ErrMBIMUICCInvalidChannel", err)
	}
}

func TestQMIChannelTransmitUsesInjectedContext(t *testing.T) {
	transport := &qmiChannelTransportFake{openChannel: 3}
	channel := NewQMIChannel(transport, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	wantDeadline, _ := ctx.Deadline()
	channel.SetContext(ctx)

	if _, err := channel.OpenLogicalChannel([]byte{0xA0, 0x00}); err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
	if _, err := channel.Transmit([]byte{0x80, 0xE2}); err != nil {
		t.Fatalf("Transmit() error = %v", err)
	}
	if !transport.transmitDeadline.Equal(wantDeadline) {
		t.Fatalf("TransmitEUICCAPDU deadline = %s, want injected deadline %s", transport.transmitDeadline, wantDeadline)
	}
}

func TestQMIChannelCloseUsesIndependentTimeoutAfterInjectedContextCancelled(t *testing.T) {
	transport := &qmiChannelTransportFake{openChannel: 3}
	channel := NewQMIChannel(transport, 1)
	ctx, cancel := context.WithCancel(context.Background())
	channel.SetContext(ctx)
	opened, err := channel.OpenLogicalChannel([]byte{0xA0, 0x00})
	if err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
	cancel()

	start := time.Now()
	if err := channel.CloseLogicalChannel(opened); err != nil {
		t.Fatalf("CloseLogicalChannel() error = %v", err)
	}
	remaining := time.Until(transport.closeDeadline)
	elapsedDeadline := transport.closeDeadline.Sub(start)
	if remaining <= 0 || elapsedDeadline < 9*time.Second || elapsedDeadline > 11*time.Second {
		t.Fatalf("CloseLogicalChannel deadline elapsed=%s remaining=%s, want independent ~10s timeout", elapsedDeadline, remaining)
	}
}

func TestQMIChannelTransmitAfterCloseFailsWithoutUsingBasicChannel(t *testing.T) {
	transport := &qmiChannelTransportFake{openChannel: 4}
	channel := NewQMIChannel(transport, 1)
	opened, err := channel.OpenLogicalChannel([]byte{0xA0, 0x00})
	if err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
	if err := channel.CloseLogicalChannel(opened); err != nil {
		t.Fatalf("CloseLogicalChannel() error = %v", err)
	}

	_, err = channel.Transmit([]byte{0x80, 0xE2})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "logical channel is not open") {
		t.Fatalf("error = %v, want logical channel is not open", err)
	}
	if len(transport.transmits) != 0 {
		t.Fatalf("TransmitEUICCAPDU calls = %#v, want none", transport.transmits)
	}
}

func TestQMIChannelCanReopenAfterClose(t *testing.T) {
	transport := &qmiChannelTransportFake{openChannel: 2}
	channel := NewQMIChannel(transport, 1)
	opened, err := channel.OpenLogicalChannel([]byte{0xA0, 0x00})
	if err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
	if err := channel.CloseLogicalChannel(opened); err != nil {
		t.Fatalf("CloseLogicalChannel() error = %v", err)
	}
	transport.openChannel = 5
	if _, err := channel.OpenLogicalChannel([]byte{0xA0, 0x01}); err != nil {
		t.Fatalf("second OpenLogicalChannel() error = %v", err)
	}
	if _, err := channel.Transmit([]byte{0x80, 0xE2}); err != nil {
		t.Fatalf("Transmit() after reopen error = %v", err)
	}
	if len(transport.transmits) != 1 || transport.transmits[0].channel != 5 {
		t.Fatalf("TransmitEUICCAPDU calls = %#v, want one call on channel 5", transport.transmits)
	}
}

func TestQMIChannelConnectStillReportsMissingTransport(t *testing.T) {
	channel := NewQMIChannel(nil, 1)
	if err := channel.Connect(); !errors.Is(err, ErrQMITransportNotAvailable) {
		t.Fatalf("Connect() error = %v, want ErrQMITransportNotAvailable", err)
	}
}
