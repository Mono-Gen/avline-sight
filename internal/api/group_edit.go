package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
)

func validGroupName(name string) bool {
	return model.ValidGroupName(name)
}

// グループ名は現場内でしか一意ではない。別画面の現場切替と操作を直列化する。
func (s *Server) checkGroupProfile(w http.ResponseWriter, r *http.Request) bool {
	value := r.Header.Get("X-AVLine-Profile")
	if value == "" {
		return true
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "プロファイルIDが不正です", "", "")
		return false
	}
	if id != s.Engine.ProfileID() {
		writeErr(w, 409, "表示中のプロファイルが変更されました", "", "画面を更新してから再試行してください")
		return false
	}
	return true
}

func orderGroups(groups []engine.Group, names []string) {
	rank := make(map[string]int, len(names))
	for i, name := range names {
		rank[name] = i + 1
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i].Name, groups[j].Name
		if a == "" || b == "" {
			return a != ""
		}
		x, y := rank[a], rank[b]
		if x == y {
			return a < b
		}
		return x != 0 && (y == 0 || x < y)
	})
}

func (s *Server) renameGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		NewName string `json:"new_name"`
	}
	err := readJSON(r, &in)
	if err != nil {
		writeErr(w, 400, "グループ名が不正です", "", "")
		return
	}
	oldName := r.PathValue("name")
	if legacy := r.URL.Query().Get("legacy_name"); oldName == "" && (legacy == "." || legacy == "..") {
		oldName = legacy
	}
	s.renameGroupTo(w, r, oldName, in.NewName)
}

func (s *Server) renameGroupTo(w http.ResponseWriter, r *http.Request, oldName, newName string) {
	newName = strings.TrimSpace(newName)
	if !validGroupName(newName) {
		writeErr(w, 400, "グループ名が不正です", "", "")
		return
	}
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	if !s.checkGroupProfile(w, r) {
		return
	}
	found, err := s.Store.RenameGroup(r.Context(), s.Engine.ProfileID(), oldName, newName)
	switch {
	case err != nil:
		writeErr(w, 500, "グループ名を保存できません", err.Error(), "")
		return
	case !found:
		writeErr(w, 404, "グループが見つかりません", "", "")
		return
	}
	if s.Poller == nil {
		s.Engine.Reload()
	} else {
		s.resync()
	}
	s.publishGroups(w, r)
}

func (s *Server) setGroupOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Names   []string `json:"names"`
		NewName *string  `json:"new_name"`
	}
	if err := readJSON(r, &in); err != nil || (in.Names == nil) == (in.NewName == nil) {
		writeErr(w, 400, "グループ順が不正です", "", "")
		return
	}
	if in.NewName != nil {
		s.renameGroupTo(w, r, "order", *in.NewName)
		return
	}
	seen := map[string]bool{}
	for _, name := range in.Names {
		if !validGroupName(name) || seen[name] {
			writeErr(w, 400, "グループ順が不正です", "", "")
			return
		}
		seen[name] = true
	}
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	if !s.checkGroupProfile(w, r) {
		return
	}
	if err := s.Store.SaveGroupOrder(r.Context(), s.Engine.ProfileID(), in.Names); err != nil {
		writeErr(w, 500, "グループ順を保存できません", err.Error(), "")
		return
	}
	s.publishGroups(w, r)
}

func (s *Server) publishGroups(w http.ResponseWriter, r *http.Request) {
	s.broadcastGroupsChanged()
	s.groups(w, r)
}

func (s *Server) broadcastGroupsChanged() {
	if s.Hub != nil {
		s.Hub.Broadcast("groups_changed", nil)
	}
}
