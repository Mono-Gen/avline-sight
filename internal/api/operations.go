package api

import (
	"net/http"
	"time"
)

func (s *Server) getPhase(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"phase": s.Engine.Phase()})
}

func (s *Server) putPhase(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phase string `json:"phase"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "運用フェーズを変更できません", err.Error(), "")
		return
	}
	if in.Phase != "setup" && in.Phase != "rehearsal" && in.Phase != "show" {
		writeErr(w, 400, "運用フェーズが不正です", "setup/rehearsal/show から指定してください", "")
		return
	}
	if err := s.Engine.SetPhase(in.Phase); err != nil {
		writeErr(w, 500, "運用フェーズを保存できません", err.Error(), "")
		return
	}
	s.getPhase(w, r)
}

func (s *Server) groups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.Engine.Groups()
	if err != nil {
		writeErr(w, 500, "グループ一覧を取得できません", err.Error(), "")
		return
	}
	names, err := s.Store.GroupOrder(r.Context(), s.Engine.ProfileID())
	if err != nil {
		writeErr(w, 500, "グループ一覧を取得できません", err.Error(), "")
		return
	}
	orderGroups(groups, names)
	writeJSON(w, 200, groups)
}

func (s *Server) knownGroup(group string) (bool, error) {
	if group == "*" {
		return true, nil
	}
	groups, err := s.Engine.Groups()
	if err != nil {
		return false, err
	}
	for _, g := range groups {
		if g.Name == group {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) resetGroupCounters(w http.ResponseWriter, r *http.Request) {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	if !s.checkGroupProfile(w, r) {
		return
	}
	var in struct {
		Group *string `json:"group"`
	}
	if err := readJSON(r, &in); err != nil || in.Group == nil {
		writeErr(w, 400, "カウンタをリセットできません", "group が必要です", "")
		return
	}
	ok, err := s.knownGroup(*in.Group)
	if err != nil {
		writeErr(w, 500, "グループを確認できません", err.Error(), "")
		return
	}
	if !ok {
		writeErr(w, 404, "グループが見つかりません", "", "")
		return
	}
	sws, err := s.Store.Switches(s.Engine.ProfileID())
	if err != nil {
		writeErr(w, 500, "スイッチ一覧を取得できません", err.Error(), "")
		return
	}
	results := []map[string]any{}
	for _, sw := range sws {
		if *in.Group != "*" && sw.GroupTag != *in.Group {
			continue
		}
		row := map[string]any{"switch_id": sw.ID, "ok": false}
		if !sw.Enabled || s.Poller == nil || !s.Poller.ResetCounters(sw.ID, 0) {
			row["error"] = "監視中のスイッチではありません"
		} else {
			row["ok"] = true
			row["reset_at"] = time.Now().UTC()
		}
		results = append(results, row)
	}
	writeJSON(w, 200, map[string]any{"results": results})
}
