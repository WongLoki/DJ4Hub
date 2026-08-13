package main

import "testing"

func TestParseMacInterfaceCountersUsesLinkRow(t *testing.T) {
	input := `Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll
en9 1500 <Link#22> aa:bb:cc:dd:ee:ff 120 0 4096 80 0 2048 0
en9 1500 192.168.225 192.168.225.20 120 - 4096 80 - 2048 -`

	got := parseMacInterfaceCounters(input)["en9"]
	if got.RX != 4096 || got.TX != 2048 {
		t.Fatalf("en9 counters = %+v, want RX=4096 TX=2048", got)
	}
}

func TestSelectUSBTrafficInterfacePrefersDefaultRoute(t *testing.T) {
	interfaces := []macNetInterface{
		{Name: "en0", Kind: "ethernet", Status: "active"},
		{Name: "en8", Kind: "ethernet", Status: "active"},
		{Name: "en9", Kind: "ethernet", Status: "active"},
	}
	if got := selectUSBTrafficInterface(interfaces, macDefaultRoute{Interface: "en9"}); got != "en9" {
		t.Fatalf("selected interface = %q, want en9", got)
	}
}

func TestSelectUSBTrafficInterfaceUsesActiveNonWiFiEthernet(t *testing.T) {
	interfaces := []macNetInterface{
		{Name: "en0", Kind: "ethernet", Status: "active", IPv4: "192.168.1.29"},
		{Name: "en3", Kind: "ethernet", Status: "inactive"},
		{Name: "en9", Kind: "ethernet", Status: "active", IPv4: "192.168.225.23"},
	}
	if got := selectUSBTrafficInterface(interfaces, macDefaultRoute{Interface: "utun14"}); got != "en9" {
		t.Fatalf("selected interface = %q, want en9", got)
	}
}

func TestSessionTrafficIsDownloadPlusUpload(t *testing.T) {
	rx, tx, total := sessionTrafficFromCounters(
		networkByteCounters{RX: 8192, TX: 4096},
		networkByteCounters{RX: 2048, TX: 1024},
	)
	if rx != 6144 || tx != 3072 || total != 9216 {
		t.Fatalf("session traffic = rx:%d tx:%d total:%d", rx, tx, total)
	}
}
