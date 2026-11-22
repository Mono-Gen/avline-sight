package poller

import (
	"log/slog"
	"time"

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
	w.st.FDBPorts = nil
	if err != nil {
		return nil, err
	}
	w.st.FDBLastOK = time.Now().UTC()
	w.st.FDBPorts = map[int][]string{}
	entries := make([]fdbEntry, 0, len(rows))
	for _, r := range rows {
		if len(r.Index) < 7 || sx.Int(r.Cols[mib.Dot1qTpFdbStatus]) != 3 {
			continue
		} // learned のみ
		mac := sx.IndexMAC(r.Index[1:7])
		bp := int(sx.Int(r.Cols[mib.Dot1qTpFdbPort]))
		w.st.FDBMACs = append(w.st.FDBMACs, mac)
		// 別スイッチの在席証拠には、ブリッジポートとifIndexの対応が確認できた行だけを使う。
		if ix, ok := w.basePort[bp]; ok {
			w.st.FDBPorts[ix] = append(w.st.FDBPorts[ix], mac)
		}
		entries = append(entries, fdbEntry{mac: mac, port: bp, vlan: int(r.Index[0])})
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
