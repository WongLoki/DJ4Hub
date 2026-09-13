package main

import (
	"testing"
)

func TestVoiceCallsExcludeData(t *testing.T) {
	calls := parseVoiceCalls("+CLCC: 1,1,0,1,0,\"\",128\r\n+CLCC: 3,0,0,0,0,\"123\",129,\"Service\"\r\n+CLCC: bad\r\n")
	if len(calls) != 1 || calls[0].ID != 3 || calls[0].Number != "123" {
		t.Fatalf("unexpected voice calls: %+v", calls)
	}
}
