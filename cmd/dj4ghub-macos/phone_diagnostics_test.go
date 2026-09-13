package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestVoiceDiagnosticPrivacy(t *testing.T) {
	raw := "ATD000000000;\r\n+CLCC: 1,0,0,0,0,\"000000000\"\r\n+CMT: secret\r\nSMS body\r\n+CME ERROR: 30\r\nERROR"
	if got := voiceDiagnosticResult(raw); got != "+CME ERROR: 30 | ERROR" {
		t.Fatalf("unsafe or incomplete result: %q", got)
	}
	if got := voiceDiagnosticResult("unknown private response"); got != "unavailable" {
		t.Fatal(got)
	}
}

func TestVoiceFailureDiagnosticsReadOnlyAndBounded(t *testing.T) {
	var commands []string
	got := voiceFailureDiagnostics(func(command string, timeout time.Duration) (string, error) {
		commands = append(commands, command)
		if timeout != 2*time.Second {
			t.Fatalf("unbounded query: %v", timeout)
		}
		switch command {
		case "AT+CEER":
			return "+CEER: 0,-1\r\nOK", nil
		case `AT+QCFG="ims"`:
			return "", errors.New("transport disconnected")
		default:
			return `+QCFG: "usbcfg",0x2CA3,0x4006,1,1,1,1,1,1,0`, nil
		}
	})
	if strings.Join(commands, ";") != `AT+CEER;AT+QCFG="ims";AT+QCFG="usbcfg"` {
		t.Fatalf("unexpected sequence: %v", commands)
	}
	if !strings.Contains(got, "ims=query_failed") || !strings.Contains(got, "ceer=+CEER: 0,-1") || !strings.Contains(got, "usb_voice=+QCFG:") {
		t.Fatal(got)
	}
}
