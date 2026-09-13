package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type communicationRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	RecordID  string    `gorm:"column:record_id;not null;uniqueIndex:uk_communication_record_record_id"`
	ICCID     string    `gorm:"column:iccid;not null;index:idx_communication_record_iccid_kind_started,priority:1"`
	Kind      string    `gorm:"column:kind;not null;index:idx_communication_record_iccid_kind_started,priority:2;index:idx_communication_record_kind_started,priority:1"`
	Started   time.Time `gorm:"column:started;not null;index:idx_communication_record_iccid_kind_started,priority:3;index:idx_communication_record_kind_started,priority:2"`
	Payload   string    `gorm:"column:payload;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (communicationRecord) TableName() string { return "communication_record" }

type historyMigration struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name      string    `gorm:"column:name;not null;uniqueIndex:uk_history_migration_name"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (historyMigration) TableName() string { return "history_migration" }

func encodeHistory(records []historyRecord) ([]communicationRecord, error) {
	rows := make([]communicationRecord, 0, len(records))
	seen := map[string]string{}
	for _, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		if record.ID == "" {
			record.ID = fmt.Sprintf("legacy-%x", sha256.Sum256(payload))
			payload, err = json.Marshal(record)
			if err != nil {
				return nil, err
			}
		}
		if previous, ok := seen[record.ID]; ok {
			if previous != string(payload) {
				return nil, fmt.Errorf("conflicting duplicate history ID")
			}
			continue
		}
		seen[record.ID] = string(payload)
		rows = append(rows, communicationRecord{RecordID: record.ID, ICCID: record.ICCID, Kind: record.Kind, Started: record.Started, Payload: string(payload)})
	}
	return rows, nil
}

func (h *communicationHistory) openDatabase(legacy string) error {
	if err := os.MkdirAll(filepath.Dir(h.path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(h.path, 0600); err != nil {
		return err
	}
	db, err := gorm.Open(sqlite.Open(h.path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)
	ok := false
	defer func() {
		if !ok {
			_ = sqlDB.Close()
		}
	}()
	// One connection makes connection-local SQLite pragmas deterministic.
	for _, statement := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL"} {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	if err := db.AutoMigrate(&communicationRecord{}, &historyMigration{}); err != nil {
		return err
	}
	if legacy != "" {
		err = db.Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&historyMigration{}).Where(&historyMigration{Name: "json-v1"}).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return nil
			}
			data, err := os.ReadFile(legacy)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err == nil {
				var records []historyRecord
				if err := json.Unmarshal(data, &records); err != nil {
					return fmt.Errorf("invalid legacy JSON; original preserved: %w", err)
				}
				rows, err := encodeHistory(records)
				if err != nil {
					return err
				}
				if len(rows) > 0 {
					if err := tx.CreateInBatches(rows, 100).Error; err != nil {
						return err
					}
				}
			}
			return tx.Create(&historyMigration{Name: "json-v1"}).Error
		})
		if err != nil {
			return err
		}
	}
	h.db = db
	ok = true
	return nil
}

func (h *communicationHistory) closeDatabase() {
	if h.db != nil {
		if db, err := h.db.DB(); err == nil {
			_ = db.Close()
		}
		h.db = nil
	}
}

func (h *communicationHistory) loadDatabase() error {
	var rows []communicationRecord
	if err := h.db.Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}
	h.records = make([]historyRecord, 0, len(rows))
	h.persisted = make(map[string]string, len(rows))
	for _, row := range rows {
		var record historyRecord
		if err := json.Unmarshal([]byte(row.Payload), &record); err != nil {
			return err
		}
		h.records = append(h.records, record)
		h.persisted[row.RecordID] = row.Payload
	}
	return nil
}

func (h *communicationHistory) persistDatabase() error {
	rows, err := encodeHistory(h.records)
	if err != nil {
		return err
	}
	changed := rows[:0]
	for _, row := range rows {
		if h.persisted[row.RecordID] != row.Payload {
			changed = append(changed, row)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "record_id"}}, DoUpdates: clause.AssignmentColumns([]string{"iccid", "kind", "started", "payload", "updated_at"})}).CreateInBatches(changed, 100).Error
	})
	if err != nil {
		return err
	}
	if h.persisted == nil {
		h.persisted = map[string]string{}
	}
	for _, row := range changed {
		h.persisted[row.RecordID] = row.Payload
	}
	return nil
}

func (h *communicationHistory) queryDatabase(ctx context.Context, card string, all bool, kind string, rawOffset string) (map[string]any, error) {
	query := h.db.WithContext(ctx).Model(&communicationRecord{})
	if !all {
		query = query.Where("iccid = ?", card)
	}
	if kind != "" {
		query = query.Where(&communicationRecord{Kind: kind})
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	offset, _ := strconv.Atoi(rawOffset)
	offset = max(0, min(offset, int(total)))
	var rows []communicationRecord
	if err := query.Order("started DESC, id DESC").Offset(offset).Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	records := []historyRecord{}
	for _, row := range rows {
		var record historyRecord
		if err := json.Unmarshal([]byte(row.Payload), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	var ids []string
	if err := h.db.WithContext(ctx).Model(&communicationRecord{}).Distinct().Pluck("iccid", &ids).Error; err != nil {
		return nil, err
	}
	cards := map[string]string{}
	ids = append(ids, h.current)
	for _, id := range ids {
		if id != "" {
			cards[id] = "SIM · " + id[max(0, len(id)-4):]
		}
	}
	return map[string]any{"records": records, "cards": cards, "current_iccid": h.current, "total": total, "next_offset": offset + len(records)}, nil
}
