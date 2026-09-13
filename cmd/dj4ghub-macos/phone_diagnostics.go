package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Keep only protocol result lines, never AT echoes, numbers or unsolicited SMS.
var (
	voiceResultPattern = regexp.MustCompile(`^(OK|ERROR|NO CARRIER|BUSY|NO ANSWER|NO DIALTONE|\+CME ERROR: [0-9]+|\+CEER: [0-9, -]+|\+QCFG: "ims",[012],[01]|\+QCFG: "usbcfg",0x[0-9A-Fa-f]{4},0x[0-9A-Fa-f]{4}(,[01]){7})$`)
	voicePrivateDigits = regexp.MustCompile(`[0-9]{5,}`)
)

func voiceDiagnosticResult(raw string) string {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if voiceResultPattern.MatchString(line) && len(lines) < 4 {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "unavailable"
	}
	return strings.Join(lines, " | ")
}

func voiceFailureDiagnostics(run func(string, time.Duration) (string, error)) string {
	// CEER must precede other diagnostic queries, which may overwrite the cause.
	var results []string
	for _, query := range []struct{ name, command string }{
		{"ceer", "AT+CEER"}, {"ims", `AT+QCFG="ims"`}, {"usb_voice", `AT+QCFG="usbcfg"`},
	} {
		raw, err := run(query.command, 2*time.Second)
		result := voiceDiagnosticResult(raw)
		if err != nil {
			result = "query_failed"
		}
		results = append(results, fmt.Sprintf("%s=%s", query.name, result))
	}
	return strings.Join(results, "; ")
}
