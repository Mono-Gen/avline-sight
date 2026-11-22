package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

// ---- プロファイル ----

func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Store.Profiles()
	if err != nil {
		writeErr(w, 500, "プロファイル一覧を取得できません", err.Error(), "")
		return
	}
	if ps == nil {
		ps = []store.Profile{}
	}
	writeJSON(w, 200, ps)
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if err := readJSON(r, &in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeErr(w, 400, "プロファイルを作成できません", "名前が空です", "現場名を入力してください")
		return
	}
	id, err := s.Store.CreateProfile(strings.TrimSpace(in.Name))
	if err != nil {
		writeErr(w, 500, "プロファイルを作成できません", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "name": in.Name})
}

func (s *Server) activateProfile(w http.ResponseWriter, r *http.Request) {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	id, err := pathID(r, "id")
	if err == nil {
		err = s.Store.ActivateProfile(id)
	}
	if err != nil {
		writeErr(w, 400, "プロファイルを切替できません", err.Error(), "")
		return
	}
	s.Engine.SetProfile(id)
	if s.Poller != nil {
		s.resync()
	}
	if s.Hub != nil {
		s.Hub.Broadcast("groups_changed", nil)
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) exportProfile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "不正なID", "", "")
		return
	}
	c, err := s.Store.ProfileConfig(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "プロファイルが見つかりません", err.Error(), "")
		return
	}
	if err == nil {
		c, err = engine.ExportProfileConfig(c)
	}
	if err != nil {
		writeErr(w, 500, "設定を書き出せません", err.Error(), "")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="avline-sight.config.json"`)
	writeJSON(w, 200, c)
}

type configDiff struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

func (s *Server) previewProfile(w http.ResponseWriter, r *http.Request) {
	next, err := engine.ReadProfileConfig(r.Body)
	if err != nil {
		writeConfigInputError(w, err)
		return
	}
	pid := s.Engine.ProfileID()
	current, err := s.Store.ProfileConfig(r.Context(), pid)
	if err != nil {
		writeErr(w, 500, "現在の設定を読み込めません", err.Error(), "")
		return
	}
	revision, err := store.ConfigRevision(pid, current, next)
	if err == nil {
		current, err = engine.ExportProfileConfig(current)
	}
	if err != nil {
		writeErr(w, 500, "現在の設定を読み込めません", err.Error(), "")
		return
	}
	if next.TrapAddr == "" {
		next.TrapAddr = current.TrapAddr
	}
	groups := map[string]bool{}
	for _, sw := range next.Switches {
		if sw.GroupTag != "" {
			groups[sw.GroupTag] = true
		}
	}
	changes, err := compareProfileConfigs(current, next)
	if err != nil {
		writeErr(w, 500, "現在の設定を読み込めません", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{
		"profile_id": pid, "revision": revision, "switch_count": len(next.Switches), "group_count": len(groups),
		"changes": changes,
	})
}

func compareProfileConfigs(current, next store.ProfileConfig) ([]configDiff, error) {
	changes := []configDiff{}
	for _, diff := range []configDiff{
		{"name", current.Name, next.Name},
		{"switches", current.Switches, next.Switches},
		{"group_order", current.GroupOrder, next.GroupOrder},
		{"trap_addr", current.TrapAddr, next.TrapAddr},
		{"ports", current.Ports, next.Ports},
		{"aliases", current.Aliases, next.Aliases},
		{"watchlist", current.Watchlist, next.Watchlist},
		{"topology_layout", current.Layout, next.Layout},
	} {
		if !reflect.DeepEqual(diff.Before, diff.After) {
			changes = append(changes, diff)
		}
	}
	var before, after map[string]json.RawMessage
	// 両方とも Settings 型から生成済み。項目順を固定して変更箇所を比較する。
	if err := json.Unmarshal(current.Settings, &before); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(next.Settings, &after); err != nil {
		return nil, err
	}
	for _, field := range []string{"setup_ping_interval_sec", "rehearsal_ping_interval_sec", "thresholds", "expert_display_default", "views"} {
		if string(before[field]) != string(after[field]) {
			changes = append(changes, configDiff{"settings." + field, before[field], after[field]})
		}
	}
	return changes, nil
}

func writeConfigInputError(w http.ResponseWriter, err error) {
	writeErr(w, 400, "設定ファイルを読み込めません", err.Error(), "該当項目を修正して再試行してください")
}

func (s *Server) importProfile(w http.ResponseWriter, r *http.Request) {
	c, err := engine.ReadProfileConfig(r.Body)
	if err != nil {
		writeConfigInputError(w, err)
		return
	}
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	if !s.checkGroupProfile(w, r) {
		return
	}
	revision := r.Header.Get("X-AVLine-Config-Revision")
	opts := store.ConfigApply{Activate: revision != "", PreviewProfile: s.Engine.ProfileID(), Revision: revision}
	if !opts.Activate {
		c.Name += "（読込）"
	}
	s.mu.Lock()
	if s.locked {
		s.mu.Unlock()
		writeErr(w, http.StatusLocked, "閲覧専用ロック中のため変更できません", "", "ヘッダーのロックを解除してから操作してください")
		return
	}
	id, err := s.Store.ApplyProfileConfig(r.Context(), c, opts)
	s.mu.Unlock()
	if errors.Is(err, store.ErrConfigChanged) {
		writeErr(w, 409, "確認後に設定が変更されました", err.Error(), "ファイルを選び直して差分を確認してください")
		return
	}
	if err != nil {
		writeErr(w, 500, "設定を保存できません", err.Error(), "")
		return
	}
	if opts.Activate {
		s.Engine.SetProfile(id)
		if s.Poller != nil {
			s.resync()
		}
		if s.Trap != nil && c.TrapAddr != "" && s.Trap.Status().Addr != c.TrapAddr {
			s.Trap.Start(c.TrapAddr)
		}
		s.broadcastGroupsChanged()
	}
	writeJSON(w, 200, map[string]any{"id": id})
}
