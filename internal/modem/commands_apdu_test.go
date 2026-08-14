package modem

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/WongLoki/DJ4Hub/internal/apduarbiter"
	"github.com/WongLoki/DJ4Hub/internal/config"
)

func newRunningTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := New(config.DeviceConfig{
		ID:            "dev-at",
		DeviceBackend: "at",
		ATPort:        "/dev/ttyUSB6",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	m.running = true
	m.healthy = true
	return m
}

func respondToCommands(t *testing.T, m *Manager, count int, respond func(commandRequest)) []string {
	t.Helper()
	commands := make(chan string, count)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < count; i++ {
			req := <-m.cmdChan
			commands <- req.cmd
			respond(req)
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %d commands", count)
	}
	close(commands)

	return drainCommands(commands)
}

func drainCommands(commands <-chan string) []string {
	out := make([]string, 0)
	for {
		select {
		case cmd, ok := <-commands:
			if !ok {
				return out
			}
			out = append(out, cmd)
		default:
			return out
		}
	}
}

func TestManagerClearLogicalChannelsAttemptsChannelsOneThroughFour(t *testing.T) {
	m := newRunningTestManager(t)

	done := make(chan []string, 1)
	go func() {
		done <- respondToCommands(t, m, 4, func(req commandRequest) {
			req.respChan <- "OK"
		})
	}()

	m.ClearLogicalChannels()

	var got []string
	select {
	case got = <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ClearLogicalChannels commands")
	}
	want := []string{"AT+CCHC=1", "AT+CCHC=2", "AT+CCHC=3", "AT+CCHC=4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerClearLogicalChannelsWaitsForAPDUTransport(t *testing.T) {
	m := newRunningTestManager(t)
	arb := apduarbiter.New("dev-at", apduarbiter.Options{MaxLeaseHold: time.Second})
	m.SetAPDUArbiter(arb)

	holder, err := arb.AcquireTransport(context.Background(), apduarbiter.Request{
		Owner: "holder",
		Mode:  "AT",
		Class: apduarbiter.APDUClassEUICCWrite,
	})
	if err != nil {
		t.Fatalf("AcquireTransport(holder) error = %v", err)
	}

	commands := make(chan string, 4)
	go func() {
		for i := 0; i < 4; i++ {
			req := <-m.cmdChan
			commands <- req.cmd
			req.respChan <- "OK"
		}
		close(commands)
	}()

	done := make(chan struct{})
	go func() {
		m.ClearLogicalChannels()
		close(done)
	}()

	select {
	case cmd := <-commands:
		t.Fatalf("ClearLogicalChannels sent %s while APDU transport was held", cmd)
	case <-time.After(50 * time.Millisecond):
	}

	holder.Release()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ClearLogicalChannels")
	}

	got := drainCommands(commands)
	want := []string{"AT+CCHC=1", "AT+CCHC=2", "AT+CCHC=3", "AT+CCHC=4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerOpenSIMAuthLogicalChannelBlockedBySwitchBarrier(t *testing.T) {
	m := newRunningTestManager(t)
	arb := apduarbiter.New("dev-at", apduarbiter.Options{MaxLeaseHold: time.Second})
	m.SetAPDUArbiter(arb)

	barrier, err := arb.BeginBarrier(context.Background(), apduarbiter.Request{
		Owner: "esim_switch",
		Mode:  "AT",
		Class: apduarbiter.APDUClassSwitchBarrier,
	}, apduarbiter.BarrierPolicy{BlockedClasses: []apduarbiter.APDUClass{
		apduarbiter.APDUClassUSIMAKA,
		apduarbiter.APDUClassSMSC,
	}})
	if err != nil {
		t.Fatalf("BeginBarrier() error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := m.OpenSIMAuthLogicalChannel("A0000000871002")
		done <- err
	}()

	select {
	case req := <-m.cmdChan:
		t.Fatalf("OpenSIMAuthLogicalChannel sent %s while switch barrier was active", req.cmd)
	case err := <-done:
		t.Fatalf("OpenSIMAuthLogicalChannel returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	barrier.Release()
	req := <-m.cmdChan
	if req.cmd != `AT+CCHO="A0000000871002"` {
		t.Fatalf("command = %q, want USIM CCHO", req.cmd)
	}
	req.respChan <- "+CCHO: 2\r\nOK"

	if err := <-done; err != nil {
		t.Fatalf("OpenSIMAuthLogicalChannel() error = %v", err)
	}
}

func TestManagerOpenEUICCLogicalChannelAllowedBySwitchBarrier(t *testing.T) {
	m := newRunningTestManager(t)
	arb := apduarbiter.New("dev-at", apduarbiter.Options{MaxLeaseHold: time.Second})
	m.SetAPDUArbiter(arb)

	barrier, err := arb.BeginBarrier(context.Background(), apduarbiter.Request{
		Owner: "esim_switch",
		Mode:  "AT",
		Class: apduarbiter.APDUClassSwitchBarrier,
	}, apduarbiter.BarrierPolicy{BlockedClasses: []apduarbiter.APDUClass{
		apduarbiter.APDUClassUSIMAKA,
		apduarbiter.APDUClassSMSC,
	}})
	if err != nil {
		t.Fatalf("BeginBarrier() error = %v", err)
	}
	defer barrier.Release()

	done := make(chan error, 1)
	go func() {
		_, err := m.OpenLogicalChannel("A0000005591010FFFFFFFF8900000100")
		done <- err
	}()

	req := <-m.cmdChan
	if req.cmd != `AT+CCHO="A0000005591010FFFFFFFF8900000100"` {
		t.Fatalf("command = %q, want eUICC CCHO", req.cmd)
	}
	req.respChan <- "+CCHO: 3\r\nOK"

	if err := <-done; err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
}

func TestManagerSIMAuthChannelAPDUBlockedBySwitchBarrier(t *testing.T) {
	m := newRunningTestManager(t)
	arb := apduarbiter.New("dev-at", apduarbiter.Options{MaxLeaseHold: time.Second})
	m.SetAPDUArbiter(arb)
	m.bindAPDUSession(2, "sim_aka", apduarbiter.APDUClassUSIMAKA)

	barrier, err := arb.BeginBarrier(context.Background(), apduarbiter.Request{
		Owner: "esim_switch",
		Mode:  "AT",
		Class: apduarbiter.APDUClassSwitchBarrier,
	}, apduarbiter.BarrierPolicy{BlockedClasses: []apduarbiter.APDUClass{
		apduarbiter.APDUClassUSIMAKA,
		apduarbiter.APDUClassSMSC,
	}})
	if err != nil {
		t.Fatalf("BeginBarrier() error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := m.TransmitAPDU(2, "00A40000")
		done <- err
	}()

	select {
	case req := <-m.cmdChan:
		t.Fatalf("TransmitAPDU sent %s while switch barrier was active", req.cmd)
	case err := <-done:
		t.Fatalf("TransmitAPDU returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	barrier.Release()
	req := <-m.cmdChan
	if req.cmd != `AT+CGLA=2,8,"00A40000"` {
		t.Fatalf("command = %q, want CGLA on channel 2", req.cmd)
	}
	req.respChan <- "+CGLA: 4,\"9000\"\r\nOK"

	if err := <-done; err != nil {
		t.Fatalf("TransmitAPDU() error = %v", err)
	}
}

func TestManagerTransmitBasicAPDURetriesWithByteLengthOn6700(t *testing.T) {
	m := newRunningTestManager(t)
	commands := make(chan string, 2)
	go func() {
		req := <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"6700\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"9000\"\r\nOK"
	}()

	resp, err := m.TransmitBasicAPDU("00A40400")
	if err != nil {
		t.Fatalf("TransmitBasicAPDU() error = %v", err)
	}
	if resp != "9000" {
		t.Fatalf("TransmitBasicAPDU() = %q, want %q", resp, "9000")
	}

	got := []string{<-commands, <-commands}
	want := []string{"AT+CSIM=8,\"00A40400\"", "AT+CSIM=4,\"00A40400\""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerTransmitBasicAPDURetriesWithByteLengthOnExecutionError(t *testing.T) {
	m := newRunningTestManager(t)
	commands := make(chan string, 2)
	go func() {
		req := <-m.cmdChan
		commands <- req.cmd
		req.errChan <- errors.New("bad length")
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"9000\"\r\nOK"
	}()

	resp, err := m.TransmitBasicAPDU("00A40400")
	if err != nil {
		t.Fatalf("TransmitBasicAPDU() error = %v", err)
	}
	if resp != "9000" {
		t.Fatalf("TransmitBasicAPDU() = %q, want %q", resp, "9000")
	}

	got := []string{<-commands, <-commands}
	want := []string{"AT+CSIM=8,\"00A40400\"", "AT+CSIM=4,\"00A40400\""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerTransmitBasicAPDUFollowsGetResponseOn61xx(t *testing.T) {
	m := newRunningTestManager(t)
	commands := make(chan string, 2)
	go func() {
		req := <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"611C\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 8,\"62149000\"\r\nOK"
	}()

	resp, err := m.TransmitBasicAPDU("00A40400")
	if err != nil {
		t.Fatalf("TransmitBasicAPDU() error = %v", err)
	}
	if resp != "62149000" {
		t.Fatalf("TransmitBasicAPDU() = %q, want %q", resp, "62149000")
	}

	got := []string{<-commands, <-commands}
	want := []string{"AT+CSIM=8,\"00A40400\"", "AT+CSIM=10,\"00c000001C\""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerResolveSIMAuthAIDUsesEFDirFullUSIMAID(t *testing.T) {
	m := newRunningTestManager(t)
	commands := make(chan string, 3)
	go func() {
		req := <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"9000\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 58,\"621482054221000A018002000A83022F008A01058B032F06019000\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 42,\"61114F0CA0000000871002FF49FF01895001019000\"\r\nOK"
	}()

	aid, source, err := m.ResolveSIMAuthAID("usim", "A0000000871002")
	if err != nil {
		t.Fatalf("ResolveSIMAuthAID() error = %v", err)
	}
	if aid != "A0000000871002FF49FF0189" {
		t.Fatalf("resolved aid = %s, want full USIM AID", aid)
	}
	if source != "at_ef_dir" {
		t.Fatalf("source = %s, want at_ef_dir", source)
	}

	got := []string{<-commands, <-commands, <-commands}
	want := []string{
		`AT+CSIM=14,"00a40004023f00"`,
		`AT+CSIM=14,"00a40004022f00"`,
		`AT+CSIM=10,"00b201040a"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerResolveSIMAuthAIDUsesEFDirFullISIMAIDWithGetResponse(t *testing.T) {
	m := newRunningTestManager(t)
	commands := make(chan string, 6)
	go func() {
		req := <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"612A\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"9000\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"611C\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 58,\"621482054221000A018002000A83022F008A01058B032F06019000\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"6115\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 50,\"61154F10A0000000871004FFFFFFFF89030200005001019000\"\r\nOK"
	}()

	aid, source, err := m.ResolveSIMAuthAID("isim", "A0000000871004")
	if err != nil {
		t.Fatalf("ResolveSIMAuthAID() error = %v", err)
	}
	if aid != "A0000000871004FFFFFFFF8903020000" {
		t.Fatalf("resolved aid = %s, want full ISIM AID", aid)
	}
	if source != "at_ef_dir" {
		t.Fatalf("source = %s, want at_ef_dir", source)
	}

	got := []string{<-commands, <-commands, <-commands, <-commands, <-commands, <-commands}
	want := []string{
		`AT+CSIM=14,"00a40004023f00"`,
		`AT+CSIM=10,"00c000002A"`,
		`AT+CSIM=14,"00a40004022f00"`,
		`AT+CSIM=10,"00c000001C"`,
		`AT+CSIM=10,"00b201040a"`,
		`AT+CSIM=10,"00c0000015"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerResolveSIMAuthAIDReturnsNotReadyOnEFDirError(t *testing.T) {
	m := newRunningTestManager(t)
	go func() {
		req := <-m.cmdChan
		req.respChan <- "ERROR"
	}()

	aid, source, err := m.ResolveSIMAuthAID("usim", "A0000000871002")
	if err == nil {
		t.Fatal("ResolveSIMAuthAID() err=nil, want not-ready")
	}
	if aid != "" || !strings.Contains(source, "not_ready") {
		t.Fatalf("aid=%q source=%q, want empty not-ready result", aid, source)
	}
}

func TestManagerResolveSIMAuthAIDReturnsNotReadyWhenEFDirHasNoMatch(t *testing.T) {
	m := newRunningTestManager(t)
	commands := make(chan string, 3)
	go func() {
		req := <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 4,\"9000\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 58,\"621482054221000A018002000A83022F008A01058B032F06019000\"\r\nOK"
		req = <-m.cmdChan
		commands <- req.cmd
		req.respChan <- "+CSIM: 26,\"610B4F06A00000015100005001019000\"\r\nOK"
	}()

	aid, source, err := m.ResolveSIMAuthAID("usim", "A0000000871002")
	if err == nil {
		t.Fatal("ResolveSIMAuthAID() err=nil, want not-ready")
	}
	if aid != "" || !strings.Contains(source, "not_ready") {
		t.Fatalf("aid=%q source=%q, want empty not-ready result", aid, source)
	}

	got := []string{<-commands, <-commands, <-commands}
	want := []string{
		`AT+CSIM=14,"00a40004023f00"`,
		`AT+CSIM=14,"00a40004022f00"`,
		`AT+CSIM=10,"00b201040a"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerTransmitBasicAPDUReturnsBothAttemptErrors(t *testing.T) {
	m := newRunningTestManager(t)
	go func() {
		req := <-m.cmdChan
		req.errChan <- errors.New("hex length rejected")
		req = <-m.cmdChan
		req.errChan <- errors.New("byte length rejected")
	}()

	_, err := m.TransmitBasicAPDU("00A40400")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "hex length rejected") || !strings.Contains(msg, "byte length rejected") {
		t.Fatalf("error = %q, want both attempt errors", msg)
	}
}

func TestManagerTransmitBasicAPDUDoesNotRetryParseFailure(t *testing.T) {
	m := newRunningTestManager(t)
	commands := make(chan string, 2)
	stop := make(chan struct{})
	go func() {
		for i := 0; i < 2; i++ {
			select {
			case req := <-m.cmdChan:
				commands <- req.cmd
				if i == 0 {
					req.respChan <- "OK"
				} else {
					req.respChan <- "+CSIM: 4,\"9000\"\r\nOK"
				}
			case <-stop:
				return
			}
		}
	}()

	_, err := m.TransmitBasicAPDU("00A40400")
	close(stop)
	if err == nil || !strings.Contains(err.Error(), "解析 CSIM 响应失败") {
		t.Fatalf("TransmitBasicAPDU() error = %v, want parse failure", err)
	}
	got := drainCommands(commands)
	want := []string{"AT+CSIM=8,\"00A40400\""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestManagerTransmitBasicAPDURejectsInvalidHex(t *testing.T) {
	for _, apduHex := range []string{"00A", "nothex"} {
		t.Run(apduHex, func(t *testing.T) {
			m := newRunningTestManager(t)
			commands := make(chan string, 1)
			stop := make(chan struct{})
			go func() {
				select {
				case req := <-m.cmdChan:
					commands <- req.cmd
					req.respChan <- "+CSIM: 4,\"9000\"\r\nOK"
				case <-stop:
				}
			}()

			_, err := m.TransmitBasicAPDU(apduHex)
			close(stop)
			if err == nil || !strings.Contains(err.Error(), "APDU hex 解码失败") {
				t.Fatalf("TransmitBasicAPDU() error = %v, want APDU hex 解码失败", err)
			}
			if got := drainCommands(commands); len(got) != 0 {
				t.Fatalf("commands = %#v, want none", got)
			}
		})
	}
}

func TestManagerAPDUSessionRegistryClearsSession(t *testing.T) {
	m := newRunningTestManager(t)
	m.bindAPDUSession(1, "test")

	if !m.hasAPDUSession(1) {
		t.Fatal("hasAPDUSession()=false want true")
	}
	if _, ok := m.takeAPDUSession(1); !ok {
		t.Fatal("takeAPDUSession() ok=false want true")
	}
	if m.hasAPDUSession(1) {
		t.Fatal("session remained in registry")
	}
}
