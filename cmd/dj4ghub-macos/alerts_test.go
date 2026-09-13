package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCommunicationAlertsExcludeOldSMSAndAnsweredCalls(t *testing.T) {
	now := time.Now()
	a := &app{alertsStarted: now, sms: []receivedSMS{{Content: "old private text", Timestamp: now.Add(-time.Hour)}, {Content: "new private text", Timestamp: now}}, history: &communicationHistory{records: []historyRecord{{ID: "ring", Kind: "call", Direction: "incoming", State: "observed"}, {ID: "answered", Kind: "call", Direction: "incoming", State: "connected"}, {ID: "out", Kind: "call", Direction: "outgoing", State: "observed"}}}}
	w := httptest.NewRecorder()
	a.communicationAlerts(w, httptest.NewRequest("GET", "/api/alerts", nil))
	var result struct {
		SMS     []string `json:"sms"`
		Ringing []string `json:"ringing"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.SMS) != 1 || len(result.Ringing) != 1 || result.Ringing[0] != "ring" {
		t.Fatal(w.Body.String())
	}
}
