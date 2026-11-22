package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

const ProfileConfigVersion = 1

// ConfigSwitch のIDはファイル内参照用。資格情報や受信履歴を型に持たせない。
type ConfigSwitch struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	GroupTag string `json:"group_tag"`
	IP       string `json:"ip"`
	SNMPPort int    `json:"snmp_port"`
	Enabled  bool   `json:"enabled"`
	WebURL   string `json:"web_url,omitempty"`
}

type ConfigPosition struct{ X, Y float64 }

type ConfigWatch struct {
	SwitchID int64  `json:"switch_id"`
	IfIndex  int    `json:"if_index"`
	Label    string `json:"label"`
}

type ProfileConfig struct {
	Version    int                            `json:"version"`
	Name       string                         `json:"name"`
	Settings   json.RawMessage                `json:"settings"`
	GroupOrder []string                       `json:"group_order"`
	TrapAddr   string                         `json:"trap_addr,omitempty"`
	Layout     map[string]ConfigPosition      `json:"topology_layout"`
	Watchlist  []ConfigWatch                  `json:"watchlist"`
	Switches   []ConfigSwitch                 `json:"switches"`
	Ports      map[string]map[int]PortSetting `json:"ports"`
	Aliases    []Alias                        `json:"aliases"`
}

var ErrConfigChanged = errors.New("current settings changed after preview")

func (s *Store) ProfileConfig(ctx context.Context, id int64) (ProfileConfig, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ProfileConfig{}, err
	}
	defer tx.Rollback()
	return readProfileConfig(ctx, tx, id)
}

func readProfileConfig(ctx context.Context, tx *sql.Tx, id int64) (ProfileConfig, error) {
	c := ProfileConfig{Version: ProfileConfigVersion}
	var raw, layout, watch string
	if err := tx.QueryRowContext(ctx, `SELECT name, settings_json, topology_layout_json, watchlist_json FROM profiles WHERE id=?`, id).
		Scan(&c.Name, &raw, &layout, &watch); err != nil {
		return c, err
	}
	c.Settings = json.RawMessage(raw)
	if err := json.Unmarshal([]byte(layout), &c.Layout); err != nil {
		return c, fmt.Errorf("topology_layout: %w", err)
	}
	if err := json.Unmarshal([]byte(watch), &c.Watchlist); err != nil {
		return c, fmt.Errorf("watchlist: %w", err)
	}
	if err := readGroupMeta(ctx, tx, groupOrderKey(id), &c.GroupOrder); err != nil {
		return c, fmt.Errorf("group_order: %w", err)
	}
	err := tx.QueryRowContext(ctx, `SELECT v FROM meta WHERE k='trap_addr'`).Scan(&c.TrapAddr)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c, err
	}
	ids, err := readConfigSwitches(ctx, tx, id, &c)
	if err != nil {
		return c, err
	}
	if err := readConfigAliases(ctx, tx, id, &c); err != nil {
		return c, err
	}
	// 削除済みスイッチへの参照は、新しい現場では別の機器を指さないよう除く。
	c.RemapReferences(ids)
	return c, nil
}

func readConfigSwitches(ctx context.Context, tx *sql.Tx, id int64, c *ProfileConfig) (map[int64]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,name,group_tag,ip,snmp_port,enabled,web_url FROM switches WHERE profile_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[int64]int64{}
	c.Switches = []ConfigSwitch{}
	for rows.Next() {
		var sw ConfigSwitch
		var source int64
		if err := rows.Scan(&source, &sw.Name, &sw.GroupTag, &sw.IP, &sw.SNMPPort, &sw.Enabled, &sw.WebURL); err != nil {
			return nil, err
		}
		sw.ID = int64(len(c.Switches) + 1)
		ids[source] = sw.ID
		c.Switches = append(c.Switches, sw)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ports, err := tx.QueryContext(ctx, `SELECT p.switch_id,p.if_index,p.monitor_excluded,p.expected_speed_mbps,p.expected_av_role,p.manual_memo
		FROM ports p JOIN switches sw ON sw.id=p.switch_id WHERE sw.profile_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer ports.Close()
	c.Ports = map[string]map[int]PortSetting{}
	for ports.Next() {
		var source int64
		var index int
		var p PortSetting
		if err := ports.Scan(&source, &index, &p.Excluded, &p.ExpectedSpeed, &p.AVRole, &p.Memo); err != nil {
			return nil, err
		}
		key := strconv.FormatInt(ids[source], 10)
		if c.Ports[key] == nil {
			c.Ports[key] = map[int]PortSetting{}
		}
		c.Ports[key][index] = p
	}
	return ids, ports.Err()
}

func readConfigAliases(ctx context.Context, tx *sql.Tx, id int64, c *ProfileConfig) error {
	rows, err := tx.QueryContext(ctx, `SELECT match_type,match_key,label,note,planned_ip FROM device_aliases
		WHERE profile_id=? ORDER BY match_type,match_key`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.Aliases = []Alias{}
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.MatchType, &a.MatchKey, &a.Label, &a.Note, &a.PlannedIP); err != nil {
			return err
		}
		c.Aliases = append(c.Aliases, a)
	}
	return rows.Err()
}

func (c *ProfileConfig) RemapReferences(ids map[int64]int64) {
	layout := map[string]ConfigPosition{}
	for key, point := range c.Layout {
		if suffix, ok := strings.CutPrefix(key, "sw-"); ok {
			id, _ := strconv.ParseInt(suffix, 10, 64)
			if ids[id] != 0 {
				layout[fmt.Sprintf("sw-%d", ids[id])] = point
			}
		} else {
			layout[key] = point
		}
	}
	watch := []ConfigWatch{}
	for _, item := range c.Watchlist {
		if id := ids[item.SwitchID]; id != 0 {
			item.SwitchID = id
			watch = append(watch, item)
		}
	}
	aliases := []Alias{}
	for _, a := range c.Aliases {
		if a.MatchType == "port" {
			source, index, _ := strings.Cut(a.MatchKey, ":")
			id, _ := strconv.ParseInt(source, 10, 64)
			if ids[id] == 0 {
				continue
			}
			a.MatchKey = fmt.Sprintf("%d:%s", ids[id], index)
		}
		aliases = append(aliases, a)
	}
	slices.SortFunc(aliases, func(a, b Alias) int {
		if order := strings.Compare(a.MatchType, b.MatchType); order != 0 {
			return order
		}
		return strings.Compare(a.MatchKey, b.MatchKey)
	})
	c.Layout, c.Watchlist, c.Aliases = layout, watch, aliases
}

// ConfigRevision は確認画面に使った設定とファイルを適用時に照合する。
func ConfigRevision(id int64, current, next ProfileConfig) (string, error) {
	data, err := json.Marshal([]any{id, current, next})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func (s *Store) HasProfiles(ctx context.Context) (bool, error) {
	var found bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM profiles)`).Scan(&found)
	return found, err
}

func (s *Store) CredentialPendingCount(ctx context.Context, id int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM switches WHERE profile_id=? AND community_ro=''`, id).Scan(&n)
	return n, err
}

type ConfigApply struct {
	Activate       bool
	OnlyEmpty      bool
	ReuseActive    bool
	PreviewProfile int64
	Revision       string
	StartupEvent   *model.Event
}

func (s *Store) ApplyProfileConfig(ctx context.Context, c ProfileConfig, opts ConfigApply) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if opts.OnlyEmpty {
		var found bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM profiles)`).Scan(&found); err != nil || found {
			return 0, err
		}
	}
	if opts.ReuseActive {
		var active int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM profiles WHERE active=1 LIMIT 1`).Scan(&active)
		if err == nil {
			current, err := readProfileConfig(ctx, tx, active)
			if err != nil {
				return 0, err
			}
			next := c
			if next.TrapAddr == "" {
				next.TrapAddr = current.TrapAddr
			}
			// 同じ --config で起動し直しても、入力済みの資格情報と監視履歴を持つ現場を再利用する。
			prevJSON, err := json.Marshal(current)
			if err != nil {
				return 0, err
			}
			nextJSON, err := json.Marshal(next)
			if err != nil {
				return 0, err
			}
			if string(prevJSON) == string(nextJSON) {
				return active, nil
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	if opts.Revision != "" {
		current, err := readProfileConfig(ctx, tx, opts.PreviewProfile)
		if err != nil {
			return 0, err
		}
		revision, err := ConfigRevision(opts.PreviewProfile, current, c)
		if err != nil {
			return 0, err
		}
		if revision != opts.Revision {
			return 0, ErrConfigChanged
		}
	}
	if opts.Activate {
		if _, err := tx.ExecContext(ctx, `UPDATE profiles SET active=0 WHERE active=1`); err != nil {
			return 0, err
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO profiles(name,active,settings_json,created_at) VALUES(?,?,?,?)`,
		c.Name, opts.Activate, string(c.Settings), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := writeProfileConfig(ctx, tx, id, c, opts.Activate); err != nil {
		return 0, err
	}
	if opts.StartupEvent != nil {
		event := *opts.StartupEvent
		event.Detail = map[string]any{"_profile_id": id}
		if _, err := insertEvent(tx, &event); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

func writeProfileConfig(ctx context.Context, tx *sql.Tx, id int64, c ProfileConfig, activate bool) error {
	ids := make(map[int64]int64, len(c.Switches))
	for _, sw := range c.Switches {
		cfg := model.SwitchConfig{ProfileID: id, Name: sw.Name, GroupTag: sw.GroupTag,
			IP: sw.IP, SNMPPort: sw.SNMPPort, Enabled: sw.Enabled, WebURL: sw.WebURL}
		if err := insertSwitch(tx, &cfg); err != nil {
			return fmt.Errorf("switches[%d]: %w", sw.ID, err)
		}
		ids[sw.ID] = cfg.ID
		for index, port := range c.Ports[strconv.FormatInt(sw.ID, 10)] {
			if err := savePortSetting(tx, cfg.ID, index, port); err != nil {
				return fmt.Errorf("ports[%d][%d]: %w", sw.ID, index, err)
			}
		}
	}
	c.RemapReferences(ids)
	for _, a := range c.Aliases {
		if err := saveAlias(tx, id, a); err != nil {
			return fmt.Errorf("aliases: %w", err)
		}
	}
	for column, value := range map[string]any{"topology_layout_json": c.Layout, "watchlist_json": c.Watchlist} {
		if err := setProfileJSON(tx, id, column, value); err != nil {
			return err
		}
	}
	if err := saveGroupOrder(ctx, tx, id, c.GroupOrder); err != nil {
		return err
	}
	if activate && c.TrapAddr != "" {
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta(k,v) VALUES('trap_addr',?)`, c.TrapAddr)
		return err
	}
	return nil
}
