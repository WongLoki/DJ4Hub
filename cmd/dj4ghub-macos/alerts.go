package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"
)

// Returns event identities only. Polling this endpoint never touches USB.
func (a *app) communicationAlerts(w http.ResponseWriter, r *http.Request) {
	sms := []string{}
	a.smsMu.RLock()
	for _, message := range a.sms {
		if !a.alertsStarted.IsZero() && message.Timestamp.Before(a.alertsStarted.Add(-30*time.Second)) {
			continue
		}
		sms = append(sms, fmt.Sprintf("%x", sha256.Sum256([]byte(smsCacheKey(message)))))
	}
	a.smsMu.RUnlock()
	ringing := []string{}
	if h := a.historyStore(); h != nil {
		h.mu.Lock()
		for _, record := range h.records {
			if record.Kind == "call" && record.Direction == "incoming" && record.State == "observed" && record.Ended == nil {
				ringing = append(ringing, record.ID)
			}
		}
		h.mu.Unlock()
	}
	writeJSON(w, 200, map[string]any{"sms": sms, "ringing": ringing})
}
