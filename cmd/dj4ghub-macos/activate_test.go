//go:build darwin && cgo

package main

import "testing"

func TestParseMacNetworkServiceOrder(t *testing.T) {
	input := `An asterisk (*) denotes that a network service is disabled.
(1) EG25G-QDC507
(Hardware Port: EG25G-QDC507, Device: en9)

(*) Baiwang
(Hardware Port: Baiwang, Device: en11)`
	got := parseMacNetworkServiceOrder(input)
	if len(got) != 2 {
		t.Fatalf("service count = %d, want 2: %+v", len(got), got)
	}
	if got[0].Name != "EG25G-QDC507" || got[0].Device != "en9" {
		t.Fatalf("first service = %+v", got[0])
	}
	if got[1].Name != "Baiwang" || got[1].Device != "en11" {
		t.Fatalf("second service = %+v", got[1])
	}
	if !got[1].Disabled {
		t.Fatal("disabled Baiwang service was not marked disabled")
	}
}

func TestStaleDJINetworkServicesPreservesCurrentProduct(t *testing.T) {
	services := []macNetworkService{
		{Name: "EG25G-QDC507", HardwarePort: "EG25G-QDC507", Device: "en9"},
		{Name: "Baiwang", HardwarePort: "Baiwang", Device: "en11"},
		{Name: "Wi-Fi", HardwarePort: "Wi-Fi", Device: "en0"},
	}
	present := map[string]bool{"en0": true}
	got := staleDJINetworkServices(services, present, "EG25G-QDC507")
	if len(got) != 1 || got[0].Name != "Baiwang" {
		t.Fatalf("stale services = %+v, want only Baiwang", got)
	}
}

func TestStaleDJINetworkServicesPreservesPresentDevice(t *testing.T) {
	services := []macNetworkService{
		{Name: "Baiwang", HardwarePort: "Baiwang", Device: "en11"},
	}
	got := staleDJINetworkServices(services, map[string]bool{"en11": true}, "EG25G-QDC507")
	if len(got) != 0 {
		t.Fatalf("stale services = %+v, want none", got)
	}
}

func TestStaleDJINetworkServicesSkipsDisabledService(t *testing.T) {
	services := []macNetworkService{
		{Name: "Baiwang", HardwarePort: "Baiwang", Device: "en11", Disabled: true},
	}
	got := staleDJINetworkServices(services, nil, "EG25G-QDC507")
	if len(got) != 0 {
		t.Fatalf("stale services = %+v, want disabled service ignored", got)
	}
}

func TestHasECMUSBInterfaces(t *testing.T) {
	if !hasECMUSBInterfaces([]usbInterfaceStatus{{Class: 255}, {Class: 2}, {Class: 10}}) {
		t.Fatal("CDC control and data interfaces should be recognized as ECM")
	}
	if hasECMUSBInterfaces([]usbInterfaceStatus{{Class: 255}, {Class: 2}}) {
		t.Fatal("CDC control without data interface should not be recognized as ECM")
	}
}

func TestMatchModuleNetworkInterfaceAfterWake(t *testing.T) {
	services := []macNetworkService{
		{Name: "EG25G-QDC507", HardwarePort: "EG25G-QDC507", Device: "en12"},
	}
	interfaces := []macNetInterface{
		{Name: "en12", Kind: "ethernet", Status: "active", IPv4: "192.168.225.28"},
	}

	// After wake the USB descriptor can be briefly vendor-only (no CDC ECM
	// interfaces) while the network service already points at the live NIC.
	vendorOnly := &usbDeviceStatus{Product: "EG25G-QDC507", Interfaces: []usbInterfaceStatus{{Class: 255}}}
	name, address := matchModuleNetworkInterface(vendorOnly, services, interfaces)
	if name != "en12" || address != "192.168.225.28" {
		t.Fatalf("vendor-only enumeration: got %q/%q, want en12/192.168.225.28", name, address)
	}

	// The module must also be found when it uses a subnet other than the
	// hardcoded 192.168.225.x.
	otherSubnet := []macNetInterface{
		{Name: "en12", Kind: "ethernet", Status: "active", IPv4: "10.64.0.2"},
	}
	name, address = matchModuleNetworkInterface(vendorOnly, services, otherSubnet)
	if name != "en12" || address != "10.64.0.2" {
		t.Fatalf("other subnet: got %q/%q, want en12/10.64.0.2", name, address)
	}

	// A foreign NIC must not be mistaken for the module.
	ecmDevice := &usbDeviceStatus{Product: "EG25G-QDC507", Interfaces: []usbInterfaceStatus{{Class: 2}, {Class: 10}}}
	name, _ = matchModuleNetworkInterface(ecmDevice, services, []macNetInterface{
		{Name: "en3", Kind: "ethernet", Status: "active", IPv4: "192.168.1.29"},
	})
	if name != "" {
		t.Fatalf("foreign ethernet should not match the module, got %q", name)
	}
}

func TestCurrentDJINetworkServicePrefersPresentInterface(t *testing.T) {
	services := []macNetworkService{
		{Name: "EG25G-QDC507", HardwarePort: "EG25G-QDC507", Device: "en9"},
		{Name: "Baiwang", HardwarePort: "Baiwang", Device: "en11", Disabled: true},
	}
	interfaces := []macNetInterface{
		{Name: "en11", Kind: "ethernet", Status: "unknown"},
	}

	got := currentDJINetworkService(services, interfaces, "Baiwang")
	if got == nil || got.Name != "Baiwang" || !got.InterfacePresent || !got.Disabled {
		t.Fatalf("current network service = %+v, want present disabled Baiwang service", got)
	}
}

func TestNetworkServiceReadyRequiresActiveIPv4(t *testing.T) {
	tests := []struct {
		name    string
		service *macNetworkService
		want    bool
	}{
		{name: "ready", service: &macNetworkService{InterfacePresent: true, InterfaceStatus: "active", IPv4: "192.168.225.23"}, want: true},
		{name: "disabled", service: &macNetworkService{Disabled: true, InterfacePresent: true, InterfaceStatus: "active", IPv4: "192.168.225.23"}},
		{name: "no address", service: &macNetworkService{InterfacePresent: true, InterfaceStatus: "active"}},
		{name: "inactive", service: &macNetworkService{InterfacePresent: true, InterfaceStatus: "inactive", IPv4: "192.168.225.23"}},
		{name: "missing", service: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := networkServiceReady(test.service); got != test.want {
				t.Fatalf("networkServiceReady() = %t, want %t", got, test.want)
			}
		})
	}
}
