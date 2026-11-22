package api

import (
	"encoding/json"
	"fmt"
	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
	"net/http"
	"strings"
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
	id, err := pathID(r, "id")
	if err == nil {
		err = s.Store.ActivateProfile(id)
	}
	if err != nil {
		writeErr(w, 400, "プロファイルを切替できません", err.Error(), "")
		return
	}
	s.Engine.SetProfile(id)
	s.resync()
	writeJSON(w, 200, map[string]any{"id": id})
}

// profileExport はプロファイル一式（スイッチ・ポート設定・ラベル・設定・レイアウト）のJSON。
type profileExport struct {
	Name      string                               `json:"name"`
	Settings  json.RawMessage                      `json:"settings"`
	Layout    json.RawMessage                      `json:"topology_layout"`
	Watchlist json.RawMessage                      `json:"watchlist"`
	Switches  []model.SwitchConfig                 `json:"switches"`
	Ports     map[string]map[int]store.PortSetting `json:"ports"`
	Aliases   []store.Alias                        `json:"aliases"`
}

func (s *Server) exportProfile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "不正なID", "", "")
		return
	}
	var ex profileExport
	var set, lay, wl string
	if err := s.Store.DB.QueryRow(`SELECT name,settings_json,topology_layout_json,watchlist_json FROM profiles WHERE id=?`, id).Scan(&ex.Name, &set, &lay, &wl); err != nil {
		writeErr(w, 404, "プロファイルが見つかりません", err.Error(), "")
		return
	}
	ex.Settings, ex.Layout, ex.Watchlist = json.RawMessage(set), json.RawMessage(lay), json.RawMessage(wl)
	ex.Switches, _ = s.Store.Switches(id)
	ex.Ports = map[string]map[int]store.PortSetting{}
	for i := range ex.Switches {
		ps, _ := s.Store.PortSettings(ex.Switches[i].ID)
		ex.Ports[ex.Switches[i].Name] = ps
		ex.Switches[i].ID = 0
	}
	ex.Aliases, _ = s.Store.Aliases(id)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="profile-%d.json"`, id))
	writeJSON(w, 200, ex)
}

func (s *Server) importProfile(w http.ResponseWriter, r *http.Request) {
	var ex profileExport
	if err := readJSON(r, &ex); err != nil || ex.Name == "" {
		writeErr(w, 400, "プロファイルを読み込めません", "JSON形式が不正です", "エクスポートしたファイルを指定してください")
		return
	}
	id, err := s.Store.CreateProfile(ex.Name + "（読込）")
	if err != nil {
		writeErr(w, 500, "プロファイルを作成できません", err.Error(), "")
		return
	}
	var set engine.Settings
	if json.Unmarshal(ex.Settings, &set) == nil {
		s.Store.SetProfileJSON(id, "settings_json", set)
	}
	var lay any
	if json.Unmarshal(ex.Layout, &lay) == nil && lay != nil {
		s.Store.SetProfileJSON(id, "topology_layout_json", lay)
	}
	for _, sw := range ex.Switches {
		sw.ID, sw.ProfileID = 0, id
		if err := s.Store.SaveSwitch(&sw); err == nil {
			for ix, ps := range ex.Ports[sw.Name] {
				s.Store.SavePortSetting(sw.ID, ix, ps)
			}
		}
	}
	for _, a := range ex.Aliases {
		s.Store.SaveAlias(id, a)
	}
	writeJSON(w, 200, map[string]any{"id": id})
}
