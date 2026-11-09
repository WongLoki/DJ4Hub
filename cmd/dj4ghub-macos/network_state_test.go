package main

import (
	"errors"
	"testing"
)

// The tests in this file pin down the behaviour required after a macOS
// sleep/wake cycle. Before the fix commit they fail and reproduce the reported
// bugs (stale USB AT handle, wrong "network card error" state, session traffic
// jumps after an interface renumber).

func TestUSBTransportErrorIncludesPostSleepFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "device gone", err: errors.New("USB bulk read: LIBUSB_ERROR_NO_DEVICE"), want: true},
		{name: "not found", err: errors.New("LIBUSB_ERROR_NOT_FOUND"), want: true},
		{name: "command timeout", err: errors.New("USB AT command timed out without response"), want: true},
		// Sleep/wake typically surfaces as timeout/pipe/interrupted instead of
		// NO_DEVICE; the dead handle must be dropped in these cases too.
		{name: "bulk timeout after sleep", err: errors.New("USB bulk read: LIBUSB_ERROR_TIMEOUT"), want: true},
		{name: "bulk pipe error after sleep", err: errors.New("USB bulk write: LIBUSB_ERROR_PIPE"), want: true},
		{name: "interrupted transfer after sleep", err: errors.New("USB bulk read: LIBUSB_ERROR_INTERRUPTED"), want: true},
		// Ordinary AT-level failures keep the handle.
		{name: "plain AT error", err: errors.New("AT+CPIN? failed: ERROR"), want: false},
		{name: "nil error", err: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isUSBTransportError(test.err); got != test.want {
				t.Fatalf("isUSBTransportError(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}

func TestResetUSBATReleasesHandleOnTransportError(t *testing.T) {
	a := &app{usbAT: &usbAT{}}
	a.resetUSBATIfGone(errors.New("USB bulk read: LIBUSB_ERROR_TIMEOUT"))
	if a.usbAT != nil {
		t.Fatal("dead USB AT handle should be released after a sleep-timeout error")
	}
}

func TestClassifyNetworkStateDistinguishesRecovering(t *testing.T) {
	tests := []struct {
		name    string
		service *macNetworkService
		want    string
	}{
		{
			name:    "ready",
			service: &macNetworkService{InterfacePresent: true, InterfaceStatus: "active", IPv4: "192.168.225.23"},
			want:    networkStateReady,
		},
		{
			// DHCP has not renewed yet right after wake: still coming back.
			name:    "dhcp pending after wake",
			service: &macNetworkService{InterfacePresent: true, InterfaceStatus: "active"},
			want:    networkStateRecovering,
		},
		{
			// Link negotiation in progress after wake.
			name:    "link negotiating after wake",
			service: &macNetworkService{InterfacePresent: true, InterfaceStatus: "inactive", IPv4: "192.168.225.23"},
			want:    networkStateRecovering,
		},
		{
			// Interface renumbered (en11 -> en12) while the service still
			// points at the old name: the module is still there.
			name:    "interface renumbered",
			service: &macNetworkService{InterfacePresent: false, InterfaceStatus: "unknown"},
			want:    networkStateRecovering,
		},
		{
			name:    "disabled service",
			service: &macNetworkService{Disabled: true, InterfacePresent: true, InterfaceStatus: "active", IPv4: "192.168.225.23"},
			want:    networkStateError,
		},
		{
			name:    "no service",
			service: nil,
			want:    networkStateError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyNetworkState(test.service); got != test.want {
				t.Fatalf("classifyNetworkState() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNetworkServiceStateAfterRenumber(t *testing.T) {
	// After wake the module re-enumerates as en12 while the network service
	// still points at en11. The UI must not report a hard "error" for this.
	service := &macNetworkService{Name: "EG25G-QDC507", HardwarePort: "EG25G-QDC507", Device: "en11"}
	interfaces := []macNetInterface{
		{Name: "en12", Kind: "ethernet", Status: "active", IPv4: "192.168.225.28"},
	}
	got := currentDJINetworkService([]macNetworkService{*service}, interfaces, "EG25G-QDC507")
	if got == nil {
		t.Fatal("service should still be selected while the module re-enumerates")
	}
	if state := networkServiceState(got); state != networkStateRecovering {
		t.Fatalf("networkServiceState() = %q, want %q (interface renumbered en11 -> en12)", state, networkStateRecovering)
	}
}

func TestTrafficBaselineSurvivesRenumber(t *testing.T) {
	baselines := map[string]networkByteCounters{
		"en11": {RX: 1000, TX: 500},
	}
	// The same module comes back as en12 with counters that kept growing.
	current := networkByteCounters{RX: 5000, TX: 2000}
	baselines = rebindTrafficBaseline(baselines, "en11", "en12", current)

	if _, exists := baselines["en11"]; exists {
		t.Fatal("baseline of the vanished interface should be dropped")
	}
	rx, tx, _ := sessionTrafficFromCounters(current, baselines["en12"])
	if rx != 4000 || tx != 1500 {
		t.Fatalf("session traffic after renumber = rx:%d tx:%d, want rx:4000 tx:1500", rx, tx)
	}
}

func TestRebindTrafficBaselineReanchorsOnCounterReset(t *testing.T) {
	baselines := map[string]networkByteCounters{
		"en11": {RX: 10000, TX: 5000},
	}
	// The interface was re-created: counters restart from zero.
	current := networkByteCounters{RX: 128, TX: 64}
	baselines = rebindTrafficBaseline(baselines, "en11", "en12", current)

	rx, tx, _ := sessionTrafficFromCounters(current, baselines["en12"])
	if rx != 0 || tx != 0 {
		t.Fatalf("session traffic after counter reset = rx:%d tx:%d, want 0/0", rx, tx)
	}
}

func TestSelectUSBTrafficInterfaceIgnoresForeignEthernet(t *testing.T) {
	// The machine has ordinary wired NICs (en3/en4). When the default route is
	// a VPN tunnel the module (192.168.225.x) must still be picked.
	interfaces := []macNetInterface{
		{Name: "en3", Kind: "ethernet", Status: "active", IPv4: "192.168.1.29"},
		{Name: "en4", Kind: "ethernet", Status: "active", IPv4: "10.0.0.7"},
		{Name: "en12", Kind: "ethernet", Status: "active", IPv4: "192.168.225.28"},
	}
	if got := selectUSBTrafficInterface(interfaces, macDefaultRoute{Interface: "utun14"}); got != "en12" {
		t.Fatalf("selected interface = %q, want en12 (module subnet 192.168.225.x)", got)
	}
}

func TestRecoverAfterWakeDropsStaleState(t *testing.T) {
	a := &app{
		usbAT: &usbAT{},
		trafficBaselines: map[string]networkByteCounters{
			"en11": {RX: 4096, TX: 2048},
		},
	}
	a.recoverAfterWake()

	if a.usbAT != nil {
		t.Fatal("stale USB AT handle should be dropped after wake")
	}
	if len(a.trafficBaselines) != 0 {
		t.Fatalf("traffic baselines should be reset after wake, got %+v", a.trafficBaselines)
	}
}
