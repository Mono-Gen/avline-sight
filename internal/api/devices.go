package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

// 接続機器一覧と機器名CSVのAPI。一覧の組み立てとCSV形式は engine/inventory.go が持つ。

// maxAliasCSVBytes は読込CSVの上限（約5000行に十分な大きさ）。
const maxAliasCSVBytes = 2 << 20

func inventoryOptions(r *http.Request) engine.InventoryOptions {
	q := r.URL.Query()
	opt := engine.InventoryOptions{Scope: q.Get("scope"), Group: q.Get("group"), Query: q.Get("q")}
	for _, v := range strings.Split(q.Get("vlans"), ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			opt.VLANs = append(opt.VLANs, n)
		}
	}
	return opt
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Engine.Inventory(inventoryOptions(r))
	if err != nil {
		writeErr(w, 500, "接続機器一覧を作れません", err.Error(), "")
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) devicesCSV(w http.ResponseWriter, r *http.Request) {
	opt := inventoryOptions(r)
	rows, err := s.Engine.Inventory(opt)
	if err != nil {
		writeErr(w, 500, "接続機器一覧を作れません", err.Error(), "")
		return
	}
	writeInventoryCSV(w, rows, opt, time.Now())
}

// replayInventory は再生データの指定時刻スナップショットから一覧を作る。
func (s *Server) replayInventory(w http.ResponseWriter, r *http.Request) ([]engine.InventoryRow, time.Time, bool) {
	rp := s.getReplay(w, r)
	if rp == nil {
		return nil, time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, r.URL.Query().Get("ts"))
	if err != nil {
		writeErr(w, 400, "時刻の指定が不正です", "ts はRFC3339形式で指定します", "")
		return nil, time.Time{}, false
	}
	at, states, err := rp.st.SnapshotAt(ts)
	if err != nil {
		writeErr(w, 404, "スナップショットがありません", err.Error(), "")
		return nil, time.Time{}, false
	}
	// 再生データに同梱された別名のうち、記録したスイッチの属するプロファイルのものを使う
	var aliases []store.Alias
	if len(states) > 0 {
		if pid, err := rp.st.ProfileOfSwitch(states[0].ID); err == nil {
			aliases, _ = rp.st.Aliases(pid)
		}
	}
	// 再生時刻以前の観測だけから最終確認を求める（記録期間内の履歴に限られる）
	var last map[string]store.LastSeen
	if len(states) > 0 {
		if pid, err := rp.st.ProfileOfSwitch(states[0].ID); err == nil {
			last, _ = rp.st.LastSeenByMAC(pid, at)
		}
	}
	opt := inventoryOptions(r)
	opt.SnapshotLabels = true
	return engine.BuildInventory(states, aliases, last, opt, at), at, true
}

func (s *Server) replayDevices(w http.ResponseWriter, r *http.Request) {
	if rows, _, ok := s.replayInventory(w, r); ok {
		writeJSON(w, 200, rows)
	}
}

func (s *Server) replayDevicesCSV(w http.ResponseWriter, r *http.Request) {
	if rows, at, ok := s.replayInventory(w, r); ok {
		writeInventoryCSV(w, rows, inventoryOptions(r), at)
	}
}

func writeInventoryCSV(w http.ResponseWriter, rows []engine.InventoryRow, opt engine.InventoryOptions, at time.Time) {
	kind := "devices"
	if opt.Scope == "unnamed" {
		kind = "devices-unnamed"
	}
	name := fmt.Sprintf("avline-sight-%s-%s.csv", kind, at.Local().Format("20060102-1504"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	if err := engine.WriteInventoryCSV(w, rows, time.Local); err != nil {
		// ヘッダ送信後なのでステータスは変えられない。途中切れのファイルになるためログに残す
		slog.Warn("接続機器CSVの書出が途中で失敗", "err", err)
	}
}

// importAliases は機器名CSVを試行（commit=false）または反映（commit=true）する。
func (s *Server) importAliases(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CSV    string `json:"csv"`
		Mode   string `json:"mode"` // merge（既定）/ replace
		Commit bool   `json:"commit"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "機器名CSVを読み込めません", err.Error(), "")
		return
	}
	if len(in.CSV) > maxAliasCSVBytes {
		writeErr(w, 413, "機器名CSVが大きすぎます", "上限は2MBです", "ファイルを分けて読み込んでください")
		return
	}
	if in.Mode != "" && in.Mode != "merge" && in.Mode != "replace" {
		writeErr(w, 400, "読込方法の指定が不正です", "mode は merge か replace です", "")
		return
	}
	rows, skipped, errs := engine.ParseAliasCSV(in.CSV)
	list := make([]store.Alias, len(rows))
	for i, x := range rows {
		list[i] = store.Alias{MatchType: "mac", MatchKey: x.MAC, Label: x.Label, Note: x.Note}
	}
	commit := in.Commit
	if commit && len(errs) > 0 {
		// 誤りを含むファイルで（特に置換モードの削除を）実行しない
		writeErr(w, 422, "エラーのある行があるため反映していません", fmt.Sprintf("%d行にエラーがあります", len(errs)), "CSVを修正してから読み込み直してください")
		return
	}
	if commit && len(rows) == 0 {
		writeErr(w, 422, "反映できる行がありません", "名称が入力された行がありません", "名称の列を記入してから読み込んでください")
		return
	}
	res, err := s.Store.ApplyMACAliases(s.Engine.ProfileID(), list, in.Mode == "replace", commit)
	if err != nil {
		writeErr(w, 500, "機器名を保存できません", err.Error(), "")
		return
	}
	if commit {
		s.Engine.Reload()
	}
	if errs == nil {
		errs = []engine.AliasCSVError{}
	}
	// 行ごとの扱い（追加/更新/変更なし）をプレビューに出すため、反映前の登録名を添える
	type previewRow struct {
		engine.AliasCSVRow
		Action   string `json:"action"` // add / update / same
		OldLabel string `json:"old_label,omitempty"`
	}
	prev := map[string]store.Alias{}
	if !commit {
		if cur, err := s.Store.Aliases(s.Engine.ProfileID()); err == nil {
			for _, a := range cur {
				if a.MatchType == "mac" {
					prev[a.MatchKey] = a
				}
			}
		}
	}
	out := make([]previewRow, 0, len(rows))
	for _, x := range rows {
		pr := previewRow{AliasCSVRow: x, Action: "add"}
		if old, ok := prev[x.MAC]; ok {
			pr.OldLabel = old.Label
			pr.Action = "update"
			if old.Label == x.Label && old.Note == x.Note {
				pr.Action = "same"
			}
		}
		out = append(out, pr)
	}
	writeJSON(w, 200, map[string]any{"committed": commit, "result": res, "rows": out, "skipped_empty": skipped, "errors": errs})
}
