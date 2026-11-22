package poller

import (
	"log/slog"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
)

type fdbEntry struct {
	mac        string
	port, vlan int
}

func (w *worker) collectFDB() ([]fdbEntry, error) {
	rows, err := w.c.WalkColumns([]string{mib.Dot1qTpFdbPort, mib.Dot1qTpFdbStatus})
	valid := err == nil
	w.st.FDBValid = &valid
	w.st.FDBMACs = nil
	if err != nil {
		return nil, err
	}
	entries := make([]fdbEntry, 0, len(rows))
	for _, r := range rows {
		if len(r.Index) < 7 || sx.Int(r.Cols[mib.Dot1qTpFdbStatus]) != 3 {
			continue
		} // learned のみ
		mac := sx.IndexMAC(r.Index[1:7])
		w.st.FDBMACs = append(w.st.FDBMACs, mac)
		entries = append(entries, fdbEntry{mac: mac, port: int(sx.Int(r.Cols[mib.Dot1qTpFdbPort])), vlan: int(r.Index[0])})
	}
	return entries, nil
}

func (w *worker) collectFDBOnly() error {
	_, err := w.collectFDB()
	if err != nil {
		slog.Debug("追加FDB取得に失敗", "switch", w.cfg.Name, "err", err)
	}
	return err
}
