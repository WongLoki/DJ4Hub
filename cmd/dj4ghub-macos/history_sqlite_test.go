package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLitePaginationAndBatchRollback(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "old.json")
	h := &communicationHistory{path: filepath.Join(dir, "history.sqlite")}
	if err := h.openDatabase(""); err != nil {
		t.Fatal(err)
	}
	h.records = []historyRecord{{ID: "existing", Kind: "sms", ICCID: "card"}}
	if err := h.persistDatabase(); err != nil {
		t.Fatal(err)
	}
	h.closeDatabase()
	items := []historyRecord{}
	for i := 0; i < 120; i++ {
		items = append(items, historyRecord{ID: fmt.Sprint(i), Kind: "sms", ICCID: "card"})
	}
	items = append(items, historyRecord{ID: "existing", Kind: "sms"})
	data, _ := json.Marshal(items)
	if err := os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.openDatabase(legacy); err == nil {
		t.Fatal("expected conflicting existing record")
	}
	if err := h.openDatabase(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.closeDatabase)
	if err := h.loadDatabase(); err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 1 {
		t.Fatal("partial import was committed")
	}
	h.records = append(h.records, items[:120]...)
	if err := h.persistDatabase(); err != nil {
		t.Fatal(err)
	}
	result, err := h.queryDatabase(context.Background(), "card", false, "sms", "100")
	if err != nil {
		t.Fatal(err)
	}
	if result["total"].(int64) != 121 || len(result["records"].([]historyRecord)) != 21 {
		t.Fatalf("bad page: %+v", result)
	}
}

func TestSQLiteMigrationIsAtomicAndRepeatable(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "communication-history.json")
	original := []historyRecord{{ID: "sms-one", ICCID: "123", Kind: "sms", Content: "保留原文", Started: time.Now()}, {ID: "call-one", Kind: "call", Direction: "incoming", State: "missed"}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	h := &communicationHistory{path: filepath.Join(dir, "history.sqlite")}
	if err := h.openDatabase(legacy); err != nil {
		t.Fatal(err)
	}
	if err := h.loadDatabase(); err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 2 || h.records[0].Content != "保留原文" {
		t.Fatal("lost records")
	}
	h.records[0].Content = "new state"
	if err := h.persistDatabase(); err != nil {
		t.Fatal(err)
	}
	h.closeDatabase()
	if err := h.openDatabase(legacy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.closeDatabase)
	if err := h.loadDatabase(); err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 2 || h.records[0].Content != "new state" {
		t.Fatal("import replayed")
	}
	backup, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, backup) {
		t.Fatal("legacy backup changed")
	}
	var migrations int64
	if err := h.db.Model(&historyMigration{}).Count(&migrations).Error; err != nil {
		t.Fatal(err)
	}
	if migrations != 1 {
		t.Fatal("missing migration marker")
	}
}

func TestSQLiteMalformedMigrationCanRetry(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "old.json")
	if err := os.WriteFile(legacy, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	h := &communicationHistory{path: filepath.Join(dir, "history.sqlite")}
	if err := h.openDatabase(legacy); err == nil {
		t.Fatal("bad JSON accepted")
	}
	if h.db != nil {
		t.Fatal("failed connection retained")
	}
	if err := os.WriteFile(legacy, []byte(`[{"id":"one","kind":"sms"},{"id":"one","kind":"call"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.openDatabase(legacy); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
	if err := os.WriteFile(legacy, []byte(`[{"id":"one","kind":"sms"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.openDatabase(legacy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.closeDatabase)
	if err := h.loadDatabase(); err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 1 {
		t.Fatal("retry duplicated or lost rows")
	}
}
