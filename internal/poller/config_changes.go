package poller

import (
	"strconv"
	"strings"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

func (w *worker) recordSetting(p *model.Port, field, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if w.configRead == nil {
		w.configRead = make(map[int]map[string]string)
	}
	settings := w.configRead[p.IfIndex]
	if settings == nil {
		settings = make(map[string]string)
		w.configRead[p.IfIndex] = settings
	}
	settings[field] = value
}

func (w *worker) recordPositiveSetting(p *model.Port, field string, value int) {
	if value > 0 {
		w.recordSetting(p, field, strconv.Itoa(value))
	}
}

func (w *worker) recordStormSetting(p *model.Port, field string, value int64) {
	switch value {
	case 1:
		w.recordSetting(p, field, "enabled")
	case 2:
		w.recordSetting(p, field, "disabled")
	}
}

func (w *worker) finishConfigCollection(tier string) {
	var fields []string
	switch tier {
	case "fast":
		fields = []string{"alias"}
	case "slow":
		fields = []string{"pvid", "eee", "trust", "max_frame", "bcast_storm_ctl", "mcast_storm_ctl"}
	default:
		return
	}
	for _, p := range w.st.Ports {
		if p.ConfigSettings == nil {
			p.ConfigSettings = make(map[string]string)
		}
		for _, field := range fields {
			before, after := p.ConfigSettings[field], w.configRead[p.IfIndex][field]
			if after == "" {
				delete(p.ConfigSettings, field)
				continue
			}
			if before != "" && before != after {
				w.emit(model.PortConfigChange(p, field, before, after))
			}
			p.ConfigSettings[field] = after
		}
	}
}
