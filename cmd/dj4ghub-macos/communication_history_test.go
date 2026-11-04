package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 自动恢复留痕是系统事件（无 SIM 归属），任何卡片筛选下都必须可见，
// 否则 UI 默认「当前卡片」筛选会把恢复记录全部滤掉，留痕等于白做。
func TestAutoRecoveryRecordsVisibleRegardlessOfCard(t *testing.T) {
	now := time.Now()
	rec := newAutoRecoveryRecord("recovered", "ECM 网桥卡死自动恢复", now.Add(-28*time.Second), now)
	if rec.Kind != "auto_recovery" {
		t.Fatalf("Kind = %q, want auto_recovery", rec.Kind)
	}
	if !historyRecordVisible(rec, "89860123456789012345", false) {
		t.Fatal("auto recovery record hidden by card filter")
	}
	if !historyRecordVisible(rec, "89860123456789012345", true) {
		t.Fatal("auto recovery record hidden in all-cards view")
	}
	sms := historyRecord{Kind: "sms", ICCID: "2222222222222222222"}
	if historyRecordVisible(sms, "1111111111111111111", false) {
		t.Fatal("sms record from another card must stay hidden")
	}
}

func TestHistoryCardIsolationAndDurability(t *testing.T) {
	h := &communicationHistory{path: filepath.Join(t.TempDir(), "records.json"), active: map[int]string{}}
	a := &app{history: h}
	now := time.Now()
	msg := receivedSMS{Memory: "SM", Sender: "123", Content: "hello", Timestamp: now}
	for _, card := range []string{"1111111111111111111", "2222222222222222222", "1111111111111111111"} {
		if err := a.archiveReceivedSMS([]receivedSMS{msg}, card); err != nil {
			t.Fatal(err)
		}
	}
	msg.Memory = "ME"
	if err := a.archiveReceivedSMS([]receivedSMS{msg}, "1111111111111111111"); err != nil {
		t.Fatal(err)
	}
	h.closeDatabase()
	if err := h.openDatabase(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.closeDatabase)
	if err := h.loadDatabase(); err != nil {
		t.Fatal(err)
	}
	rows := h.records
	if len(rows) != 3 || rows[2].ICCID != "" {
		t.Fatalf("unexpected attribution: %+v", rows)
	}
	info, _ := os.Stat(h.path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("history must be private")
	}
	if confirmedHistoryIdentity("one", "two") != "" || confirmedHistoryIdentity("", "") != "" {
		t.Fatal("stale identity accepted")
	}
	req := httptest.NewRequest("GET", "/api/history?card=unknown", nil)
	w := httptest.NewRecorder()
	a.listHistory(w, req)
	var result struct {
		Records []historyRecord `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 {
		t.Fatal(w.Body.String())
	}
}

func TestHistoryCallLifecycleAndCardSwap(t *testing.T) {
	h := &communicationHistory{active: map[int]string{}}
	now := time.Now()
	check := func(calls []voiceCall, card string, at time.Time) {
		t.Helper()
		if err := h.observe(calls, card, at); err != nil {
			t.Fatal(err)
		}
	}
	check([]voiceCall{{ID: 1, Direction: 1, State: 4, Number: "123"}}, "card-a", now)
	check([]voiceCall{{ID: 1, Direction: 1, State: 0, Number: "123"}}, "card-a", now.Add(time.Second))
	check(nil, "card-a", now.Add(11*time.Second))
	if len(h.records) != 1 || h.records[0].Duration != 10 || h.records[0].State != "ended" {
		t.Fatalf("%+v", h.records)
	}
	check([]voiceCall{{ID: 1, Direction: 1, State: 4}}, "card-a", now.Add(12*time.Second))
	check([]voiceCall{{ID: 1, Direction: 1, State: 4}}, "card-b", now.Add(13*time.Second))
	if h.records[1].State != "interrupted" || h.records[2].ICCID != "card-b" {
		t.Fatalf("%+v", h.records)
	}
	check(nil, "card-b", now.Add(14*time.Second))
	if h.records[2].State != "missed" {
		t.Fatal("missing missed-call state")
	}
}

func TestHistoryObservationFailureDoesNotInventHangup(t *testing.T) {
	h := &communicationHistory{active: map[int]string{}}
	now := time.Now()
	if err := h.observe([]voiceCall{{ID: 1, State: 0}}, "card", now); err != nil {
		t.Fatal(err)
	}
	if err := h.interrupt(); err != nil {
		t.Fatal(err)
	}
	if err := h.observe(nil, "card", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if h.records[0].State != "interrupted" || h.records[0].Ended != nil || h.records[0].Duration != 0 {
		t.Fatalf("invented timing: %+v", h.records[0])
	}
}

func TestHistorySaveFailureCanRetryWithoutDuplicates(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(bad, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	h := &communicationHistory{path: filepath.Join(bad, "history"), active: map[int]string{}}
	a := &app{history: h}
	row := historyRecord{ID: "unique", Kind: "sms"}
	if err := a.appendHistory(row); err == nil {
		t.Fatal("expected persistence failure")
	}
	h.path = filepath.Join(dir, "history.json")
	if err := a.appendHistory(row); err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 1 || h.dirty {
		t.Fatal("retry failed")
	}
}

func TestHistoryPaginationAndNoCurrentCard(t *testing.T) {
	h := &communicationHistory{active: map[int]string{}}
	for i := 0; i < 105; i++ {
		h.records = append(h.records, historyRecord{Kind: "sms", ICCID: "1234567890123456789"})
	}
	a := &app{history: h}
	for _, tc := range []struct {
		query string
		want  int
	}{{"card=all", 100}, {"card=all&offset=100", 5}, {"card=current", 0}, {"card=all&offset=900", 0}} {
		w := httptest.NewRecorder()
		a.listHistory(w, httptest.NewRequest("GET", "/api/history?"+tc.query, nil))
		var result struct {
			Records []historyRecord `json:"records"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Records) != tc.want {
			t.Fatalf("%s: got %d", tc.query, len(result.Records))
		}
	}
}
