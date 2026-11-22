package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
)

type displayAlert struct {
	engine.Alert
	Detail  json.RawMessage `json:"detail,omitempty"`
	Hint    string          `json:"hint,omitempty"`
	AckedAt *time.Time      `json:"acked_at,omitempty"`
}

// 翻訳用の引数と対処文は元イベントを使い、WebSocketの発生状態とは独立して取得する。
func (s *Server) displayAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := s.Engine.Alerts()
	out := make([]displayAlert, len(alerts))
	ids := make([]int64, len(alerts))
	for i, alert := range alerts {
		out[i].Alert = alert
		ids[i] = alert.EventID
	}
	if len(ids) != 0 {
		if err := s.loadAlertDetails(r, ids, out); err != nil {
			slog.Warn("アラートの表示情報を取得できません", "err", err)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) loadAlertDetails(r *http.Request, ids []int64, out []displayAlert) error {
	params, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	index := make(map[int64]int, len(ids))
	for i, id := range ids {
		index[id] = i
	}
	rows, err := s.Store.DB.QueryContext(r.Context(), `
		SELECT id, detail_json, hint_text, acked_at FROM events
		WHERE id IN (SELECT value FROM json_each(?))`, string(params))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var detail, hint sql.NullString
		var ackAt sql.NullInt64
		if err := rows.Scan(&id, &detail, &hint, &ackAt); err != nil {
			return err
		}
		item := &out[index[id]]
		if detail.Valid && json.Valid([]byte(detail.String)) {
			item.Detail = json.RawMessage(detail.String)
		}
		item.Hint = hint.String
		if ackAt.Valid && ackAt.Int64 > 0 {
			at := time.UnixMilli(ackAt.Int64).UTC()
			item.AckedAt = &at
		}
	}
	return rows.Err()
}
