package store

import (
	"context"
	"time"
)

type ChecklistActivity struct {
	SwitchID int64
	IfIndex  int
	Config   bool
}

func (s *Store) ChecklistActivity(ctx context.Context, profileID int64, errorsFrom, configFrom, to time.Time) ([]ChecklistActivity, error) {
	const query = `
		WITH activity(switch_id, if_index, config) AS (
			SELECT switch_id, if_index, category='CONFIG' FROM events
			WHERE ts<=? AND (
				(category='CONFIG' AND ts>=?) OR
				(ts>=? AND code IN ('PHY_ERRORS','DISCARDS','COS_DROP','MBUF_ALLOC_FAIL','LINK_FLAP','LINK_DOWN','MTU_MISMATCH'))
			)
			UNION
			SELECT switch_id, if_index, 0 FROM port_metrics_raw
			WHERE ts BETWEEN ? AND ? AND (err_delta>0 OR fcs_delta>0 OR discard_delta>0 OR cos_drop_delta>0)
			UNION
			SELECT switch_id, 0, 0 FROM switch_health_raw
			WHERE ts BETWEEN ? AND ? AND mbuf_alloc_fails_delta>0
		)
		SELECT DISTINCT a.switch_id, a.if_index, a.config FROM activity a
		JOIN switches sw ON sw.id=a.switch_id
		WHERE sw.profile_id=? AND sw.enabled=1
		ORDER BY a.switch_id, a.if_index, a.config`
	rows, err := s.DB.QueryContext(ctx, query, ms(to), ms(configFrom), ms(errorsFrom),
		ms(errorsFrom), ms(to), ms(errorsFrom), ms(to), profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChecklistActivity
	for rows.Next() {
		var activity ChecklistActivity
		if err := rows.Scan(&activity.SwitchID, &activity.IfIndex, &activity.Config); err != nil {
			return nil, err
		}
		out = append(out, activity)
	}
	return out, rows.Err()
}
