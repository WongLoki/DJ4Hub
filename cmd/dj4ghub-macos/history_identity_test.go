package main

import (
	"testing"
)

func TestHistoryICCIDFiller(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"digits", "+QCCID: 8986012345678901234\r\nOK", "8986012345678901234"},
		{"filler", "AT+QCCID\r\n+QCCID: 8986012345678901234F\r\nOK", "8986012345678901234"},
		{"lowercase", "+QCCID: 8986012345678901234f", "8986012345678901234"},
		{"double filler", "+QCCID: 8986012345678901234fF", ""},
		{"embedded filler", "+QCCID: 898601234F678901234", ""},
		{"short", "+QCCID: 1234F", ""},
		{"error", "+CME ERROR: SIM not inserted", ""},
		{"other identifier", "+CIMI: 8986012345678901234", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseHistoryICCID(tc.raw); got != tc.want {
				t.Fatalf("unexpected normalized identity: %q", got)
			}
		})
	}
	before := parseHistoryICCID("+QCCID: 8986012345678901234F")
	after := parseHistoryICCID("+QCCID: 8986012345678901234")
	if confirmedHistoryIdentity(before, after) != after {
		t.Fatal("padding must not split the same SIM identity")
	}
	if confirmedHistoryIdentity(before, "8986012345678909999") != "" || confirmedHistoryIdentity(before, "") != "" {
		t.Fatal("changed or unreadable SIM must remain unassigned")
	}
}
