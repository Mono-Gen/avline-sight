package api

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/store"
)

const showReportTimeout = 30 * time.Second

func (s *Server) showReport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), showReportTimeout)
	defer cancel()
	q := r.URL.Query()
	var window store.ShowReportRange
	if q.Get("from") == "" && q.Get("to") == "" {
		var err error
		window, err = s.Store.LatestShowRange(ctx, time.Now())
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "本番の開始記録がありません", "", "出力期間を指定してください")
			return
		}
		if err != nil {
			writeErr(w, 500, "本番期間を取得できません", err.Error(), "期間を指定して再試行してください")
			return
		}
	} else {
		var startErr, endErr error
		window.From, startErr = time.Parse(time.RFC3339, q.Get("from"))
		window.To, endErr = time.Parse(time.RFC3339, q.Get("to"))
		if startErr != nil || endErr != nil || !window.From.Before(window.To) {
			writeErr(w, 400, "出力期間が不正です", "", "開始と終了を指定し、開始を終了より前にしてください")
			return
		}
	}
	started := false
	err := writeShowReport(ctx, w, s.Store, window, func() {
		started = true
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="avline-sight-show-report.csv"`)
		w.Header().Set("Cache-Control", "no-store")
	})
	if err != nil {
		if errors.Is(err, store.ErrShowReportTooLarge) {
			writeErr(w, 422, "本番後レポートが出力上限を超えています", "", "出力期間を短くしてください")
		} else if !started {
			writeErr(w, 500, "本番後レポートを出力できません", err.Error(), "時間をおいて再試行してください")
		} else {
			slog.Warn("本番後レポートの出力が中断されました", "err", err)
		}
	}
}

func writeShowReport(ctx context.Context, w io.Writer, st *store.Store, window store.ShowReportRange, start func()) error {
	entries, err := st.ShowReport(ctx, window)
	if err != nil {
		return err
	}
	start()
	if _, err := io.WriteString(w, "\uFEFF"); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	jst := time.FixedZone("JST", 9*60*60)
	if err := cw.Write([]string{"time (JST)", "type", "event_id", "code", "severity", "switch_id", "switch_name", "if_index", "title", "detail"}); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := []string{entry.At.In(jst).Format("2006-01-02 15:04:05.000"), entry.Kind, strconv.FormatInt(entry.EventID, 10),
			entry.Code, entry.Severity, strconv.FormatInt(entry.SwitchID, 10), entry.SwitchName, strconv.Itoa(entry.IfIndex), entry.Title, entry.Detail}
		for i, value := range row {
			row[i] = csvSafe(value)
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
