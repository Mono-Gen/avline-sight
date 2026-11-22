package engine

import (
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

func productionBaselineName(now time.Time) string {
	jst := time.FixedZone("JST", 9*60*60)
	return "本番開始 " + now.In(jst).Format("2006-01-02 15:04")
}

func baselineSavedEvent(name string) model.Event {
	return model.Event{Category: "BASELINE", Severity: model.SevInfo, Code: "BASELINE_SAVED",
		Title: "ベースライン保存: " + name, Detail: map[string]any{"name": name}}
}
