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
