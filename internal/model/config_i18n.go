package model

import "fmt"

const CategoryConfig = "CONFIG"
const CodePortConfigChanged = "PORT_CONFIG_CHANGED"

var configFieldJa = map[string]string{
	"alias":           "ポートの説明",
	"pvid":            "PVID",
	"eee":             "EEE",
	"trust":           "Trust",
	"max_frame":       "最大フレーム",
	"bcast_storm_ctl": "ブロードキャストストーム制御",
	"mcast_storm_ctl": "マルチキャストストーム制御",
}

func PortConfigChange(p *Port, field, before, after string) Event {
	return Event{
		Category: CategoryConfig, Severity: SevInfo, Code: CodePortConfigChanged, IfIndex: p.IfIndex, Hint: EventHintJa(CodePortConfigChanged),
		Title:  fmt.Sprintf("%s %s %s → %s", p.Name, configFieldJa[field], before, after),
		Detail: map[string]any{"port": p.Name, "field": field, "before": before, "after": after},
	}
}
