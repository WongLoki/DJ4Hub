package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gorm.io/gorm"
)

// ICCID identifies a subscription/profile, not the USB module or phone number.
type historyRecord struct {
	ID        string     `json:"id"`
	ICCID     string     `json:"iccid"`
	Kind      string     `json:"kind"`
	Direction string     `json:"direction"`
	Number    string     `json:"number"`
	Content   string     `json:"content,omitempty"`
	State     string     `json:"state"`
	Started   time.Time  `json:"started"`
	Connected *time.Time `json:"connected,omitempty"`
	Ended     *time.Time `json:"ended,omitempty"`
	Duration  int64      `json:"duration_seconds"`
}

type communicationHistory struct {
	mu          sync.Mutex
	path        string
	records     []historyRecord
	active      map[int]string
	current     string
	fileLock    *os.File
	dirty       bool
	db          *gorm.DB
	persisted   map[string]string
	backup      historyBackupConfig
	backupError string
}

func (a *app) initHistory() error {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	if a.history != nil {
		return nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	h := &communicationHistory{active: map[int]string{}, records: []historyRecord{}}
	if !a.demo {
		legacy := filepath.Join(dir, "DJ4Hub", "communication-history.json")
		h.path = filepath.Join(dir, "DJ4Hub", "communication-history.sqlite")
		if err := os.MkdirAll(filepath.Dir(h.path), 0700); err != nil {
			return err
		}
		// Retain the old lock name to exclude a running JSON-era backend too.
		lock, err := os.OpenFile(legacy+".lock", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = lock.Close()
			return fmt.Errorf("another service owns communication history: %w", err)
		}
		h.fileLock = lock
		defer func() {
			if a.history == nil {
				_ = lock.Close()
			}
		}()
		if err := h.openDatabase(legacy); err != nil {
			return err
		}
		if err := h.loadDatabase(); err != nil {
			h.closeDatabase()
			return err
		}
		{
			for i := range h.records {
				if h.records[i].Kind == "call" && h.records[i].Ended == nil {
					h.records[i].State = "interrupted"
					h.dirty = true
				}
			}
		}
	}
	if h.dirty {
		if err := h.save(); err != nil {
			return err
		}
	}
	a.history = h
	h.loadBackupConfig()
	return nil
}

func (a *app) historyStore() *communicationHistory {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	return a.history
}

func (h *communicationHistory) save() error {
	if h.path == "" {
		return nil
	}
	if h.db == nil {
		if err := h.openDatabase(""); err != nil {
			return err
		}
	}
	if err := h.persistDatabase(); err != nil {
		return err
	}
	h.dirty = false
	return nil
}

func confirmedHistoryIdentity(before string, after string) string {
	if before != "" && before == after {
		return before
	}
	return ""
}

func (a *app) historyIdentity() string {
	if a.historyStore() == nil {
		return ""
	}
	if a.demo {
		return "89860123456789012345"
	}
	raw, err := a.runATCommand("AT+QCCID", 3*time.Second)
	if err != nil {
		return ""
	}
	return parseHistoryICCID(raw)
}

// Some modules expose the final BCD filler nibble as F. It is not part of
// the subscription identity. Do not strip arbitrary non-digit characters.
func parseHistoryICCID(raw string) string {
	value := strings.TrimSpace(parseUSBATPrefixed(raw, "+QCCID:"))
	if strings.HasSuffix(strings.ToUpper(value), "F") {
		value = value[:len(value)-1]
	}
	if !regexp.MustCompile(`^[0-9]{18,22}$`).MatchString(value) {
		return ""
	}
	return value
}

func (a *app) appendHistory(record historyRecord) error {
	h := a.historyStore()
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if record.ID == "" {
		record.ID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), len(h.records))
	}
	for _, old := range h.records {
		if old.ID == record.ID {
			if h.dirty {
				return h.save()
			}
			return nil
		}
	}
	h.records = append(h.records, record)
	h.dirty = true
	return h.save()
}

func (a *app) archiveReceivedSMS(messages []receivedSMS, identity string) error {
	for _, msg := range messages {
		card := ""
		// ME and serial callbacks have no reliable original subscription identity.
		if msg.Memory == "SM" {
			card = identity
		}
		digest := sha256.Sum256([]byte(card + "\x00" + smsCacheKey(msg)))
		if err := a.appendHistory(historyRecord{ID: "sms-" + hex.EncodeToString(digest[:]), ICCID: card, Kind: "sms", Direction: "incoming", Number: msg.Sender, Content: msg.Content, Started: msg.Timestamp, State: "received"}); err != nil {
			return err
		}
	}
	return nil
}

func (h *communicationHistory) observe(calls []voiceCall, identity string, now time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current = identity
	seen := map[int]bool{}
	for _, call := range calls {
		seen[call.ID] = true
		id := h.active[call.ID]
		index := -1
		for i := range h.records {
			if h.records[i].ID == id && h.records[i].ICCID == identity {
				index = i
				break
			}
		}
		if index == -1 {
			h.dirty = true
			id = fmt.Sprintf("call-%d-%d", now.UnixNano(), call.ID)
			direction := "outgoing"
			if call.Direction == 1 {
				direction = "incoming"
			}
			h.records = append(h.records, historyRecord{ID: id, ICCID: identity, Kind: "call", Direction: direction, Number: call.Number, Started: now, State: "observed"})
			index = len(h.records) - 1
			h.active[call.ID] = id
		}
		r := &h.records[index]
		if call.Number != "" && call.Number != r.Number {
			h.dirty = true
			r.Number = call.Number
		}
		if (call.State == 0 || call.State == 1) && r.Connected == nil {
			h.dirty = true
			t := now
			r.Connected = &t
		}
		if r.Connected != nil {
			r.State = "connected"
		}
	}
	for i := range h.records {
		r := &h.records[i]
		if r.Kind != "call" || r.Ended != nil || r.State == "interrupted" {
			continue
		}
		present := false
		for key, id := range h.active {
			if id == r.ID && seen[key] {
				present = true
			}
		}
		if present {
			continue
		}
		t := now
		h.dirty = true
		r.Ended = &t
		r.State = "unanswered"
		if r.Direction == "incoming" {
			r.State = "missed"
		}
		if r.Connected != nil {
			r.State = "ended"
			r.Duration = int64(now.Sub(*r.Connected).Seconds())
		}
		if r.ICCID != identity {
			r.State = "interrupted"
		}
	}
	for key := range h.active {
		if !seen[key] {
			delete(h.active, key)
		}
	}
	if h.dirty {
		return h.save()
	}
	return nil
}

func (a *app) monitorCallHistory(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.demo || !a.audioMu.TryLock() {
				continue
			}
			identity := a.historyIdentity()
			raw, err := a.phoneCommand("AT+CLCC")
			after := a.historyIdentity()
			h := a.historyStore()
			if h == nil {
				a.audioMu.Unlock()
				continue
			}
			if err != nil {
				if saveErr := h.interrupt(); saveErr != nil {
					log.Printf("communication history save failed: %v", saveErr)
				}
				a.audioMu.Unlock()
				continue // A read failure is not evidence of hangup.
			}
			if err := h.observe(parseVoiceCalls(raw), confirmedHistoryIdentity(identity, after), time.Now()); err != nil {
				log.Printf("communication history save failed: %v", err)
			}
			a.audioMu.Unlock()
		}
	}
}

func (a *app) listHistory(w http.ResponseWriter, r *http.Request) {
	h := a.historyStore()
	if h == nil {
		writeError(w, 503, "记录存储未初始化")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	card := r.URL.Query().Get("card")
	if card == "current" {
		card = h.current
		if card == "" {
			card = "no-current-card"
		}
	}
	if card == "unknown" {
		card = ""
	}
	all := r.URL.Query().Get("card") == "all"
	if h.db != nil {
		result, err := h.queryDatabase(r.Context(), card, all, r.URL.Query().Get("kind"), r.URL.Query().Get("offset"))
		if err != nil {
			writeError(w, 500, "读取通信记录失败")
			return
		}
		writeJSON(w, 200, result)
		return
	}
	rows := []historyRecord{}
	cards := map[string]string{}
	for _, record := range h.records {
		if record.ICCID != "" {
			cards[record.ICCID] = "SIM · " + record.ICCID[max(0, len(record.ICCID)-4):]
		}
		if (all || record.ICCID == card) && (r.URL.Query().Get("kind") == "" || record.Kind == r.URL.Query().Get("kind")) {
			rows = append(rows, record)
		}
	}
	if h.current != "" {
		cards[h.current] = "SIM · " + h.current[max(0, len(h.current)-4):]
	}
	sort.SliceStable(rows, func(i int, j int) bool { return rows[i].Started.After(rows[j].Started) })
	total := len(rows)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	offset = max(0, min(offset, total))
	end := min(offset+100, total)
	writeJSON(w, 200, map[string]any{"records": rows[offset:end], "cards": cards, "current_iccid": h.current, "total": total, "next_offset": end})
}

// End timing is deliberately left unknown after a gap in observation.
func (h *communicationHistory) interrupt() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current = ""
	for i := range h.records {
		for _, id := range h.active {
			if h.records[i].ID == id {
				h.records[i].State = "interrupted"
				h.dirty = true
			}
		}
	}
	h.active = map[int]string{}
	if h.dirty {
		return h.save()
	}
	return nil
}
