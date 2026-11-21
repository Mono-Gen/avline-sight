package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// PanelDef はフロントパネル定義（仕様3.1-3）。Rows は上段・下段のポート名（ifName）並び。
// 空文字 "" は筐体上の隙間（銅線群とSFP群の間など）を表す。
// data/models/*.json に同形式のファイルを置くと内蔵定義より優先される（機種追加・実機配置の補正用）。
type PanelDef struct {
	Model string     `json:"model"`
	Rows  [][]string `json:"rows"`
	Note  string     `json:"note,omitempty"`
}

// builtinModels は代表的なM4250の銅線/SFPポート数（製品カタログ準拠の概数）。
// TODO(要確認): 実機の筐体ポート配置（上下段の順序）は製品写真と照合して data/models で補正すること。
var builtinModels = map[string][2]int{
	"M4250-26G4F-PoE++":  {26, 4},
	"M4250-10G2XF-PoE++": {10, 2},
	"M4250-12M2XF":       {12, 2},
	"M4250-40G8XF-PoE++": {40, 8},
	"M4250-26G4XF-PoE+":  {26, 4},
	"M4250-8G2XF-PoE+":   {8, 2},
	"M4250-10G2F-PoE+":   {10, 2},
	"M4250-16XF":         {0, 16},
	"M4250-12M2XF-PoE++": {12, 2},
}

// rackLayout は「奇数番上段・偶数番下段」の一般的な2段配置を生成する。
func rackLayout(model string, copper, sfp int) PanelDef {
	top, bottom := []string{}, []string{}
	add := func(from, to int) {
		for p := from; p <= to; p += 2 {
			top = append(top, fmt.Sprintf("1/0/%d", p))
			if p+1 <= to {
				bottom = append(bottom, fmt.Sprintf("1/0/%d", p+1))
			} else {
				bottom = append(bottom, "")
			}
		}
	}
	add(1, copper)
	if sfp > 0 && copper > 0 {
		top, bottom = append(top, ""), append(bottom, "")
	}
	add(copper+1, copper+sfp)
	return PanelDef{Model: model, Rows: [][]string{top, bottom}, Note: "内蔵定義"}
}

// loadPanelDefs は内蔵定義と data/models/*.json を統合する。
func (s *Server) loadPanelDefs() map[string]PanelDef {
	out := map[string]PanelDef{}
	for m, c := range builtinModels {
		out[m] = rackLayout(m, c[0], c[1])
	}
	dir := filepath.Join(s.DataDir, "models")
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil || len(b) > 1<<20 {
			continue
		}
		var d PanelDef
		if err := json.Unmarshal(b, &d); err != nil || strings.TrimSpace(d.Model) == "" || len(d.Rows) == 0 {
			slog.Warn("フロントパネル定義を読み込めません（形式不正）", "file", filepath.Base(f), "err", err)
			continue
		}
		d.Note = "data/models/" + filepath.Base(f)
		out[d.Model] = d
	}
	return out
}

// panelModels は GET /api/models。未知モデルはUI側でifTable（銅線/SFP種別）から自動生成する。
func (s *Server) panelModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.loadPanelDefs())
}
