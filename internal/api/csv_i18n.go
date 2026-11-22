package api

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
)

// 英語の先頭4列も既存の読込処理が受理する名称にそろえる。
var inventoryEnglishHeader = []string{
	"MAC", "name", "note", "planned_ip", "IP", "vendor", "lldp_name", "switch", "port",
	"VLAN", "PoE(W)", "link_speed", "last_seen", "source", "Ping",
}

// データ値の整形と数式対策はエンジンを唯一の正とし、見出しだけを置換する。
func writeLocalizedInventoryCSV(w io.Writer, rows []engine.InventoryRow, loc *time.Location, english bool) error {
	if !english {
		return engine.WriteInventoryCSV(w, rows, loc)
	}
	writer := &inventoryHeaderWriter{
		Writer:      w,
		expected:    []byte("\uFEFF" + strings.Join(engine.InventoryCSVHeader, ",") + "\r\n"),
		replacement: []byte("\uFEFF" + strings.Join(inventoryEnglishHeader, ",") + "\r\n"),
	}
	if err := engine.WriteInventoryCSV(writer, rows, loc); err != nil {
		return err
	}
	if len(writer.expected) > 0 {
		return fmt.Errorf("CSV header was incomplete")
	}
	return nil
}

// 見出し分だけを保持し、行数に比例した追加バッファを作らない。
type inventoryHeaderWriter struct {
	io.Writer
	expected, replacement, pending []byte
}

func (w *inventoryHeaderWriter) Write(data []byte) (int, error) {
	original := len(data)
	consumed := 0
	if len(w.expected) > 0 {
		n := min(len(data), len(w.expected)-len(w.pending))
		w.pending = append(w.pending, data[:n]...)
		data = data[n:]
		consumed = n
		if len(w.pending) < len(w.expected) {
			return original, nil
		}
		if !bytes.Equal(w.pending, w.expected) {
			return 0, fmt.Errorf("unexpected inventory CSV header")
		}
		if n, err := w.Writer.Write(w.replacement); err != nil {
			return 0, err
		} else if n != len(w.replacement) {
			return 0, io.ErrShortWrite
		}
		w.expected, w.pending = nil, nil
	}
	n, err := w.Writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return consumed + n, err
}
