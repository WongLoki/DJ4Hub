package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func backupTestHistory(t *testing.T) *communicationHistory {
	t.Helper()
	h := &communicationHistory{path: filepath.Join(t.TempDir(), "history.sqlite"), active: map[int]string{}}
	if err := h.openDatabase(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.closeDatabase)
	return h
}

func TestBackupWALSnapshotAndMergeRestore(t *testing.T) {
	h := backupTestHistory(t)
	h.records = []historyRecord{{ID: "sms-one", ICCID: "8986012345678901234", Kind: "sms", Direction: "incoming", Content: "test", State: "received", Started: time.Now()}}
	if err := h.save(); err != nil {
		t.Fatal(err)
	}
	h.backup.Directory = t.TempDir()
	path, err := h.backupNow()
	if err != nil {
		t.Fatal(err)
	}
	records, err := readHistoryBackup(path)
	if err != nil || len(records) != 1 || records[0].Content != "test" {
		t.Fatalf("snapshot missing committed data: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("snapshot must be private")
	}
	other := backupTestHistory(t)
	other.records = []historyRecord{{ID: "sms-newer", Kind: "sms", Content: "newer", Started: time.Now()}}
	if err := other.save(); err != nil {
		t.Fatal(err)
	}
	count, safety, err := other.restoreBackup(path)
	if err != nil || count != 1 || len(other.records) != 2 {
		t.Fatalf("merge failed: %d %v", count, err)
	}
	old, err := readHistoryBackup(safety)
	if err != nil || len(old) != 1 || old[0].Content != "newer" {
		t.Fatal("pre-restore snapshot missing")
	}
	count, _, err = other.restoreBackup(path)
	if err != nil || count != 0 {
		t.Fatal("restore must deduplicate")
	}
	other.active[1] = "active"
	if _, _, err := other.restoreBackup(path); err == nil {
		t.Fatal("restore during call must fail")
	}
	other.active = map[int]string{}
	invalid := filepath.Join(t.TempDir(), "invalid.sqlite")
	if err := os.WriteFile(invalid, []byte("not a database"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := other.restoreBackup(invalid); err == nil || len(other.records) != 2 {
		t.Fatal("invalid restore changed records")
	}
}

func TestBackupRotationAndConfig(t *testing.T) {
	h := backupTestHistory(t)
	h.backup.Directory = t.TempDir()
	unrelated := filepath.Join(h.backup.Directory, "someone-else.sqlite")
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if _, err := h.backupNow(); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(h.backup.Directory)
	if err != nil || len(files) != 11 {
		t.Fatalf("expected 10 snapshots and unrelated file: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("unrelated file removed")
	}
	loaded := &communicationHistory{path: h.path}
	loaded.loadBackupConfig()
	if loaded.backup.LastFile != h.backup.LastFile || loaded.backup.Owner == "" {
		t.Fatal("config did not persist")
	}
	h.backup.Directory = filepath.Join(t.TempDir(), "offline")
	if _, err := h.backupNow(); err == nil {
		t.Fatal("unavailable directory should fail")
	}
}

func TestAutomaticBackupOnlyWhenEnabledDueAndChanged(t *testing.T) {
	h := backupTestHistory(t)
	h.backup.Directory = t.TempDir()
	h.backupIfDue(time.Now())
	if h.backup.LastFile != "" {
		t.Fatal("must be opt in")
	}
	h.backup.Enabled = true
	h.backupIfDue(time.Now())
	first := h.backup.LastFile
	if first == "" {
		t.Fatalf("initial snapshot failed: %s", h.backupError)
	}
	h.backupIfDue(time.Now().Add(time.Hour))
	if h.backup.LastFile != first {
		t.Fatal("unchanged data should not create duplicate snapshots")
	}
	h.records = []historyRecord{{ID: "new", Kind: "sms", Started: time.Now()}}
	if err := h.save(); err != nil {
		t.Fatal(err)
	}
	h.backupIfDue(time.Now())
	if h.backup.LastFile != first {
		t.Fatal("should wait for interval")
	}
	h.backupIfDue(time.Now().Add(time.Hour))
	if h.backup.LastFile == first {
		t.Fatal("changed records should be backed up")
	}
}

func TestBackupAPIRequiresNativeHeaderAndRestoreConfirmation(t *testing.T) {
	h := backupTestHistory(t)
	a := &app{history: h}
	for _, origin := range []string{"", "https://example.com"} {
		r := httptest.NewRequest("POST", "/api/history/backup", strings.NewReader(`{"action":"backup"}`))
		if origin != "" {
			r.Header.Set("X-DJ4Hub-Audio", "1")
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		a.historyBackupAction(w, r)
		if w.Code != 403 {
			t.Fatal("unexpected cross-origin permission")
		}
	}
	data, err := json.Marshal(map[string]any{"action": "configure", "path": t.TempDir(), "enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/history/backup", strings.NewReader(string(data)))
	r.Header.Set("X-DJ4Hub-Audio", "1")
	w := httptest.NewRecorder()
	a.historyBackupAction(w, r)
	if w.Code != 200 || !h.backup.Enabled {
		t.Fatalf("configure failed: %s", w.Body)
	}
	r = httptest.NewRequest("POST", "/api/history/backup", strings.NewReader(`{"action":"restore","path":"/tmp/example.sqlite"}`))
	r.Header.Set("X-DJ4Hub-Audio", "1")
	w = httptest.NewRecorder()
	a.historyBackupAction(w, r)
	if w.Code != 400 {
		t.Fatal("restore requires explicit confirmation")
	}
}
