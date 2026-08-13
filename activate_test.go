//go:build darwin && cgo

package main

import "testing"

func TestParseUSBNetMode(t *testing.T) {
	if got := parseUSBNetMode("+QCFG: \"usbnet\",1\r\nOK"); got != "1" {
		t.Fatalf("mode = %q, want 1", got)
	}
}

func TestParseMacNetworkServiceOrder(t *testing.T) {
	input := `(1) EG25G-QDC507
(Hardware Port: EG25G-QDC507, Device: en9)

(*) Baiwang
(Hardware Port: Baiwang, Device: en11)`
	got := parseMacNetworkServiceOrder(input)
	if len(got) != 2 || got[0].Device != "en9" || got[1].Device != "en11" || !got[1].Disabled {
		t.Fatalf("services = %+v", got)
	}
}

func TestStaleDJINetworkServices(t *testing.T) {
	services := []macNetworkService{
		{Name: "EG25G-QDC507", HardwarePort: "EG25G-QDC507", Device: "en9"},
		{Name: "Baiwang", HardwarePort: "Baiwang", Device: "en11"},
		{Name: "Wi-Fi", HardwarePort: "Wi-Fi", Device: "en0"},
	}
	got := staleDJINetworkServices(services, map[string]bool{"en0": true}, "EG25G-QDC507")
	if len(got) != 1 || got[0].Name != "Baiwang" {
		t.Fatalf("stale services = %+v, want Baiwang", got)
	}
}

func TestHasECMUSBInterfaces(t *testing.T) {
	if !hasECMUSBInterfaces([]usbInterfaceStatus{{Class: 255}, {Class: 2}, {Class: 10}}) {
		t.Fatal("CDC control and data interfaces should be ECM")
	}
}
