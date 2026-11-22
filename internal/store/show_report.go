package store

import (
	"context"
	"errors"
	"time"
)

type ShowReportRange struct {
	From, To       time.Time
	StartID, EndID int64
}

func (s *Store) LatestShowRange(ctx context.Context, now time.Time) (ShowReportRange, error) {
	const query = `
		WITH start AS (
			SELECT id, ts FROM events WHERE code='PHASE_CHANGED' AND json_extract(detail_json,'$.phase')='show'
			ORDER BY ts DESC, id DESC LIMIT 1
		), finish AS (
			SELECT e.id, e.ts FROM events e, start
			WHERE e.code='PHASE_CHANGED' AND (e.ts,e.id)>(start.ts,start.id)
			ORDER BY e.ts,e.id LIMIT 1
		)
		SELECT start.id,start.ts,COALESCE(finish.id,0),COALESCE(finish.ts,?)
		FROM start LEFT JOIN finish ON 1=1`
	var window ShowReportRange
	var from, to int64
	err := s.DB.QueryRowContext(ctx, query, ms(now)).Scan(&window.StartID, &from, &window.EndID, &to)
	window.From, window.To = fromMS(from), fromMS(to)
	return window, err
}

type ShowReportEntry struct {
	At                                              time.Time
	EventID, SwitchID                               int64
	IfIndex                                         int
	Kind, Code, Severity, SwitchName, Title, Detail string
}

const (
	showReportMaxRows      = 10_000
	showReportMaxTextBytes = 16 << 20
)

var ErrShowReportTooLarge = errors.New("show report exceeds the export size limit")

func (s *Store) ShowReport(ctx context.Context, window ShowReportRange) ([]ShowReportEntry, error) {
	const query = `
		WITH timeline(at, id, kind, position) AS (
			SELECT ts,id,CASE
				WHEN code='PHASE_CHANGED' THEN 'phase'
				WHEN code='PRE_SHOW_CHECKLIST_REMAINDER' THEN 'checklist'
				WHEN category='MARKER' THEN 'marker'
				ELSE 'alert'
			END,0 FROM events
			WHERE (code IN ('PHASE_CHANGED','PRE_SHOW_CHECKLIST_REMAINDER') OR category='MARKER'
				OR (severity IN ('WARNING','CRITICAL') AND category NOT IN ('TRAP','SYSLOG')
					AND COALESCE(json_extract(detail_json,'$._suppressed'),0)=0))
				AND ts BETWEEN ? AND ?
				AND (?=0 OR (ts,id)>=(?,?)) AND (?=0 OR (ts,id)<=(?,?))
			UNION ALL
			SELECT cleared_at,id,'recovery',1 FROM events
			WHERE cleared_at BETWEEN ? AND ? AND severity IN ('WARNING','CRITICAL')
				AND category NOT IN ('TRAP','SYSLOG') AND COALESCE(json_extract(detail_json,'$._suppressed'),0)=0
				AND cleared_at > ts AND code <> 'LINK_FLAP' AND COALESCE(json_extract(detail_json,'$._one_shot'),0)=0
			UNION ALL
			SELECT acked_at,id,'ack',2 FROM events
			WHERE acked=1 AND acked_at BETWEEN ? AND ? AND severity IN ('WARNING','CRITICAL')
				AND category NOT IN ('TRAP','SYSLOG') AND COALESCE(json_extract(detail_json,'$._suppressed'),0)=0
		)
		SELECT t.at,t.id,t.kind,e.code,e.severity,e.switch_id,e.switch_name,e.if_index,e.title,
			CASE WHEN t.kind IN ('checklist','phase') THEN e.detail_json ELSE '' END
		FROM timeline t JOIN events e ON e.id=t.id
		ORDER BY t.at,t.id,t.position LIMIT ?`
	from, to := ms(window.From), ms(window.To)
	rows, err := s.DB.QueryContext(ctx, query, from, to, window.StartID, from, window.StartID, window.EndID, to, window.EndID, from, to, from, to, showReportMaxRows+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []ShowReportEntry
	var textBytes int
	for rows.Next() {
		if len(entries) == showReportMaxRows {
			return nil, ErrShowReportTooLarge
		}
		var entry ShowReportEntry
		var at int64
		if err := rows.Scan(&at, &entry.EventID, &entry.Kind, &entry.Code, &entry.Severity, &entry.SwitchID, &entry.SwitchName,
			&entry.IfIndex, &entry.Title, &entry.Detail); err != nil {
			return nil, err
		}
		entry.At = fromMS(at)
		textBytes += len(entry.Kind) + len(entry.Code) + len(entry.Severity) + len(entry.SwitchName) + len(entry.Title) + len(entry.Detail)
		if textBytes > showReportMaxTextBytes {
			return nil, ErrShowReportTooLarge
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
