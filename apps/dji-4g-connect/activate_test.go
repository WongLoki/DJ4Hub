//go:build darwin && cgo

package main

import "testing"

func TestUSBNetMode(t *testing.T) {
	for input, want := range map[string]string{
		"AT+QCFG=\"usbnet\"\r\n+QCFG: \"usbnet\",0\r\nOK": "0",
		"+QCFG: \"usbnet\",1\r\nOK":                       "1",
		"ERROR":                                           "",
	} {
		if got := usbnetMode(input); got != want {
			t.Fatalf("usbnetMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseHardwarePorts(t *testing.T) {
	ports := parseHardwarePorts("Hardware Port: EG25G-QDC507\nDevice: en9\nEthernet Address: 00:00:00:00:00:00\n")
	if len(ports) != 1 || ports[0].name != "EG25G-QDC507" || ports[0].device != "en9" {
		t.Fatalf("unexpected ports: %#v", ports)
	}
}

func TestParseNetworkServices(t *testing.T) {
	input := `(1) EG25G-QDC507
(Hardware Port: EG25G-QDC507, Device: en9)

(*) Baiwang
(Hardware Port: Baiwang, Device: en11)`
	services := parseNetworkServices(input)
	if len(services) != 2 || services[0].device != "en9" || !services[1].disabled {
		t.Fatalf("unexpected services: %#v", services)
	}
}

func TestResponseParsing(t *testing.T) {
	if !responseFinished("AT\r\r\nOK\r\n") || !responseOK("AT\r\nOK") {
		t.Fatal("OK response should be complete and successful")
	}
	if !responseFinished("\r\n+CME ERROR: 3\r\n") {
		t.Fatal("CME error response should be complete")
	}
}
