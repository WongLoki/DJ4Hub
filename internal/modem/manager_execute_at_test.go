package modem

import (
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WongLoki/DJ4Hub/internal/config"
	"go.bug.st/serial"
)

func TestManagerExecuteATFailsFastWithoutATPort(t *testing.T) {
	m, err := New(config.DeviceConfig{
		ID:            "dev-qmi",
		DeviceBackend: "qmi",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if m.HasATPort() {
		t.Fatal("HasATPort() = true, want false")
	}
	if m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = true, want false")
	}

	if _, err := m.ExecuteAT("AT", 100*time.Millisecond); err == nil || err.Error() != "当前设备没有可用 AT 端口" {
		t.Fatalf("ExecuteAT() error = %v, want 当前设备没有可用 AT 端口", err)
	}
}

func TestManagerNewAllowsResolvedQMIWithoutATPort(t *testing.T) {
	m, err := New(config.DeviceConfig{
		ID:            "dev-qmi-resolved",
		ControlDevice: "/dev/cdc-wdm0",
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil for resolved QMI backend without AT port", err)
	}
	if m.HasATPort() {
		t.Fatal("HasATPort() = true, want false")
	}
	if m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = true, want false")
	}
}

func TestManagerExecuteATFailsFastWhenNotRunning(t *testing.T) {
	m, err := New(config.DeviceConfig{
		ID:            "dev-qmi",
		DeviceBackend: "qmi",
		ATPort:        "/dev/ttyUSB6",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !m.HasATPort() {
		t.Fatal("HasATPort() = false, want true")
	}
	if m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = true, want false")
	}

	if _, err := m.ExecuteAT("AT", 100*time.Millisecond); err == nil || err.Error() != "AT 管理器未启动或不可用" {
		t.Fatalf("ExecuteAT() error = %v, want AT 管理器未启动或不可用", err)
	}
}

func TestManagerStartSkipsATManagerForPureQMIBackend(t *testing.T) {
	m, err := New(config.DeviceConfig{
		ID:            "dev-qmi",
		DeviceBackend: "qmi",
		ATPort:        "/dev/vohive-test-at-port-that-must-not-open",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !m.HasATPort() {
		t.Fatal("HasATPort() = false, want true so the manual AT terminal can still see the configured port")
	}

	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v, want nil without opening the AT port", err)
	}
	if !m.WaitReady(20 * time.Millisecond) {
		t.Fatal("WaitReady() = false, want true after pure QMI start skips AT manager")
	}
	if m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = true, want false because pure QMI must not expose the automatic AT manager")
	}
}

func TestManagerStartSkipsATManagerForResolvedQMIBackend(t *testing.T) {
	m, err := New(config.DeviceConfig{
		ID:            "dev-qmi-resolved",
		ControlDevice: "/dev/cdc-wdm0",
		ATPort:        "/dev/vohive-test-at-port-that-must-not-open",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v, want nil without opening the AT port", err)
	}
	if !m.WaitReady(20 * time.Millisecond) {
		t.Fatal("WaitReady() = false, want true after resolved QMI start skips AT manager")
	}
	if m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = true, want false because resolved QMI must not expose the automatic AT manager")
	}
}

func TestManagerCanExecuteATWhenRunning(t *testing.T) {
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

	if !m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = false, want true")
	}
}

func TestManagerStopAndWaitWaitsForBackgroundLoops(t *testing.T) {
	m, err := New(config.DeviceConfig{
		ID:            "dev-qmi",
		DeviceBackend: "qmi",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	release := make(chan struct{})
	m.loopWG.Add(1)
	go func() {
		defer m.loopWG.Done()
		<-release
	}()

	if m.StopAndWait(20 * time.Millisecond) {
		t.Fatal("StopAndWait() = true while background loop is still running, want false")
	}

	close(release)
	if !m.StopAndWait(time.Second) {
		t.Fatal("StopAndWait() = false after background loop exited, want true")
	}
}

func TestManagerClassifiesFatalSerialRuntimeErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "input output", err: errors.New("input/output error"), want: true},
		{name: "no such device", err: errors.New("open /dev/ttyUSB2: no such device"), want: true},
		{name: "bad file descriptor", err: errors.New("bad file descriptor"), want: true},
		{name: "timeout", err: errors.New("timeout"), want: false},
		{name: "command error", err: errors.New("设备返回错误: ERROR"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFatalSerialRuntimeErr(tt.err); got != tt.want {
				t.Fatalf("isFatalSerialRuntimeErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

type failingSerialPort struct {
	writeErr error
	closed   atomic.Bool
}

func (p *failingSerialPort) SetMode(*serial.Mode) error { return nil }
func (p *failingSerialPort) Read([]byte) (int, error)   { return 0, io.EOF }
func (p *failingSerialPort) Write([]byte) (int, error)  { return 0, p.writeErr }
func (p *failingSerialPort) Drain() error               { return nil }
func (p *failingSerialPort) ResetInputBuffer() error    { return nil }
func (p *failingSerialPort) ResetOutputBuffer() error   { return nil }
func (p *failingSerialPort) SetDTR(bool) error          { return nil }
func (p *failingSerialPort) SetRTS(bool) error          { return nil }
func (p *failingSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return nil, nil
}
func (p *failingSerialPort) SetReadTimeout(time.Duration) error { return nil }
func (p *failingSerialPort) Close() error {
	p.closed.Store(true)
	return nil
}
func (p *failingSerialPort) Break(time.Duration) error { return nil }

type timeoutSerialPort struct {
	closed atomic.Bool
	writes atomic.Int32
}

func (p *timeoutSerialPort) SetMode(*serial.Mode) error { return nil }
func (p *timeoutSerialPort) Read([]byte) (int, error)   { return 0, io.EOF }
func (p *timeoutSerialPort) Write(b []byte) (int, error) {
	p.writes.Add(1)
	return len(b), nil
}
func (p *timeoutSerialPort) Drain() error             { return nil }
func (p *timeoutSerialPort) ResetInputBuffer() error  { return nil }
func (p *timeoutSerialPort) ResetOutputBuffer() error { return nil }
func (p *timeoutSerialPort) SetDTR(bool) error        { return nil }
func (p *timeoutSerialPort) SetRTS(bool) error        { return nil }
func (p *timeoutSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return nil, nil
}
func (p *timeoutSerialPort) SetReadTimeout(time.Duration) error { return nil }
func (p *timeoutSerialPort) Close() error {
	p.closed.Store(true)
	return nil
}
func (p *timeoutSerialPort) Break(time.Duration) error { return nil }

func TestHandleCommandNotifiesDisconnectOnFatalWriteError(t *testing.T) {
	m, err := New(config.DeviceConfig{ID: "dev-at", ATPort: "/dev/ttyUSB6", DeviceBackend: "at"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	port := &failingSerialPort{writeErr: errors.New("input/output error")}
	m.port = port
	m.running = true
	m.healthy = true
	disconnected := make(chan struct{}, 1)
	m.SetOnDisconnectWithReason(func(string) { disconnected <- struct{}{} })

	req := commandRequest{
		cmd:      "AT+CPIN?",
		timeout:  time.Second,
		respChan: make(chan string, 1),
		errChan:  make(chan error, 1),
	}
	m.handleCommand(req)

	if err := <-req.errChan; err == nil || !strings.Contains(err.Error(), "input/output error") {
		t.Fatalf("command error = %v, want input/output error", err)
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("disconnect callback was not called")
	}
	if m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = true after fatal serial error, want false")
	}
	if !port.closed.Load() {
		t.Fatal("serial port was not closed")
	}
}

func TestHandleCommandTriggersWatchdogAfterConsecutiveNormalTimeouts(t *testing.T) {
	m, err := New(config.DeviceConfig{ID: "dev-at", ATPort: "/dev/ttyUSB6", DeviceBackend: "at"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	port := &timeoutSerialPort{}
	m.port = port
	m.running = true
	m.healthy = true
	disconnected := make(chan string, 1)
	m.SetOnDisconnectWithReason(func(reason string) { disconnected <- reason })

	for i := 1; i <= atTimeoutWatchdogThreshold; i++ {
		req := commandRequest{
			cmd:      "AT+PING",
			timeout:  time.Millisecond,
			respChan: make(chan string, 1),
			errChan:  make(chan error, 1),
		}
		m.handleCommand(req)
		if err := <-req.errChan; err == nil || err.Error() != "命令执行超时" {
			t.Fatalf("timeout %d error=%v want 命令执行超时", i, err)
		}
		if i < atTimeoutWatchdogThreshold {
			select {
			case reason := <-disconnected:
				t.Fatalf("disconnect reason=%q before threshold %d", reason, i)
			default:
			}
		}
	}

	select {
	case reason := <-disconnected:
		if reason != "at_timeout_threshold" {
			t.Fatalf("reason=%q want at_timeout_threshold", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect callback was not called after AT timeout threshold")
	}
	if m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = true after timeout threshold, want false")
	}
	if !port.closed.Load() {
		t.Fatal("serial port was not closed after timeout threshold")
	}
}

func TestHandleCommandIgnoresHighPriorityTimeoutsForWatchdog(t *testing.T) {
	m, err := New(config.DeviceConfig{ID: "dev-at", ATPort: "/dev/ttyUSB6", DeviceBackend: "at"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	m.port = &timeoutSerialPort{}
	m.running = true
	m.healthy = true
	disconnected := make(chan string, 1)
	m.SetOnDisconnectWithReason(func(reason string) { disconnected <- reason })

	for i := 0; i < atTimeoutWatchdogThreshold+1; i++ {
		req := commandRequest{
			cmd:          "AT+HIGH",
			timeout:      time.Millisecond,
			respChan:     make(chan string, 1),
			errChan:      make(chan error, 1),
			highPriority: true,
		}
		m.handleCommand(req)
		if err := <-req.errChan; err == nil || err.Error() != "命令执行超时" {
			t.Fatalf("timeout %d error=%v want 命令执行超时", i, err)
		}
	}

	select {
	case reason := <-disconnected:
		t.Fatalf("disconnect reason=%q for high-priority timeouts, want none", reason)
	default:
	}
	if !m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = false after high-priority timeouts, want true")
	}
}

func TestHandleCommandResetsTimeoutWatchdogOnDeviceError(t *testing.T) {
	m, err := New(config.DeviceConfig{ID: "dev-at", ATPort: "/dev/ttyUSB6", DeviceBackend: "at"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	m.port = &timeoutSerialPort{}
	m.running = true
	m.healthy = true
	disconnected := make(chan string, 1)
	m.SetOnDisconnectWithReason(func(reason string) { disconnected <- reason })

	for i := 0; i < atTimeoutWatchdogThreshold-1; i++ {
		req := commandRequest{
			cmd:      "AT+PING",
			timeout:  time.Millisecond,
			respChan: make(chan string, 1),
			errChan:  make(chan error, 1),
		}
		m.handleCommand(req)
		<-req.errChan
	}

	req := commandRequest{
		cmd:      "AT+ERROR",
		timeout:  time.Second,
		respChan: make(chan string, 1),
		errChan:  make(chan error, 1),
	}
	go func() {
		time.Sleep(time.Millisecond)
		m.rxChan <- rxMsg{Data: "ERROR"}
	}()
	m.handleCommand(req)
	if err := <-req.errChan; err == nil || !strings.Contains(err.Error(), "设备返回错误") {
		t.Fatalf("device error=%v want 设备返回错误", err)
	}

	for i := 0; i < atTimeoutWatchdogThreshold-1; i++ {
		req := commandRequest{
			cmd:      "AT+PING",
			timeout:  time.Millisecond,
			respChan: make(chan string, 1),
			errChan:  make(chan error, 1),
		}
		m.handleCommand(req)
		<-req.errChan
	}

	select {
	case reason := <-disconnected:
		t.Fatalf("disconnect reason=%q after reset by device error, want none", reason)
	default:
	}
	if !m.CanExecuteAT() {
		t.Fatal("CanExecuteAT() = false after reset by device error, want true")
	}
}

func TestExecuteATReturnsFatalSerialErrorBeforeManagerStopped(t *testing.T) {
	m, err := New(config.DeviceConfig{ID: "dev-at", ATPort: "/dev/ttyUSB6", DeviceBackend: "at"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	m.port = &failingSerialPort{writeErr: errors.New("input/output error")}
	m.running = true
	m.healthy = true
	disconnected := make(chan struct{}, 1)
	m.SetOnDisconnectWithReason(func(string) { disconnected <- struct{}{} })

	go func() {
		req := <-m.cmdChan
		m.handleCommand(req)
	}()

	_, err = m.ExecuteAT("AT+CPIN?", time.Second)
	if err == nil || !strings.Contains(err.Error(), "input/output error") {
		t.Fatalf("ExecuteAT() error = %v, want input/output error", err)
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("disconnect callback was not called")
	}
}

func TestManagerIsURCTreatsCGLAAsSynchronousResponse(t *testing.T) {
	m, err := New(config.DeviceConfig{
		ID:            "dev-qmi",
		DeviceBackend: "qmi",
		ATPort:        "/dev/ttyUSB6",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if m.isURC(`+CGLA: 16,"BF41038101059000"`) {
		t.Fatal("isURC(+CGLA) = true, want false so APDU responses stay with the active command")
	}
}

func TestManagerExecuteATReturnsResponseWhenRunning(t *testing.T) {
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

	go func() {
		req := <-m.cmdChan
		req.respChan <- "OK"
	}()

	resp, err := m.ExecuteAT("AT", time.Second)
	if err != nil {
		t.Fatalf("ExecuteAT() error = %v", err)
	}
	if resp != "OK" {
		t.Fatalf("ExecuteAT() resp = %q, want %q", resp, "OK")
	}
}
