package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

var ErrGroupWorking = errors.New("group is in work mode")

func groupOrderKey(profileID int64) string {
	return "group_order:" + strconv.FormatInt(profileID, 10)
}

type metaReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readGroupMeta(ctx context.Context, db metaReader, key string, dest any) error {
	var value string
	err := db.QueryRowContext(ctx, "SELECT v FROM meta WHERE k=?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(value), dest)
}

func (s *Store) GroupOrder(ctx context.Context, profileID int64) ([]string, error) {
	var names []string
	err := readGroupMeta(ctx, s.DB, groupOrderKey(profileID), &names)
	return names, err
}

type metaWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func saveGroupOrder(ctx context.Context, db metaWriter, profileID int64, names []string) error {
	value, err := json.Marshal(names)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO meta(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v",
		groupOrderKey(profileID), string(value))
	return err
}

func (s *Store) SaveGroupOrder(ctx context.Context, profileID int64, names []string) error {
	return saveGroupOrder(ctx, s.DB, profileID, names)
}

func (s *Store) RenameGroup(ctx context.Context, profileID int64, oldName, newName string) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// 期限を保持する監視エンジンとDBの間で作業モードが失われる変更を防ぐ。
	var work map[string]time.Time
	if err := readGroupMeta(ctx, tx, "work_mode", &work); err != nil {
		return false, err
	}
	if oldName != "*" && oldName != newName && work[oldName].After(time.Now()) {
		return false, ErrGroupWorking
	}
	result, err := tx.ExecContext(ctx, "UPDATE switches SET group_tag=? WHERE profile_id=? AND group_tag=?", newName, profileID, oldName)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	var names []string
	if err := readGroupMeta(ctx, tx, groupOrderKey(profileID), &names); err != nil {
		return false, err
	}
	merged := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if name == oldName {
			name = newName
		}
		if !seen[name] {
			merged = append(merged, name)
			seen[name] = true
		}
	}
	if err := saveGroupOrder(ctx, tx, profileID, merged); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
