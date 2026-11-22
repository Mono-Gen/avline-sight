package engine

import (
	"fmt"
	"time"
)

// 既定は従来の60秒。30秒の下限で登録機器への定期送信を毎分2回以下に抑える。
const (
	DefaultPingIntervalSec = 60
	MinPingIntervalSec     = 30
	MaxPingIntervalSec     = 3600
)

func (s Settings) Validate() error {
	if err := s.validatePingIntervals(); err != nil {
		return err
	}
	return s.validateMonitoringSettings()
}

func (s Settings) validatePingIntervals() error {
	for _, interval := range []struct {
		field string
		value int
	}{{"setup_ping_interval_sec", s.SetupPingIntervalSec}, {"rehearsal_ping_interval_sec", s.RehearsalPingIntervalSec}} {
		if interval.value < MinPingIntervalSec || interval.value > MaxPingIntervalSec {
			return fmt.Errorf("%s: must be between %d and %d seconds", interval.field, MinPingIntervalSec, MaxPingIntervalSec)
		}
	}
	return nil
}

func (s *Settings) normalizePingIntervals() {
	for _, value := range []*int{&s.SetupPingIntervalSec, &s.RehearsalPingIntervalSec} {
		if *value < MinPingIntervalSec || *value > MaxPingIntervalSec {
			*value = DefaultPingIntervalSec
		}
	}
}

func (e *Engine) pingInterval() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	seconds := e.settings.SetupPingIntervalSec
	if e.phase == "rehearsal" {
		seconds = e.settings.RehearsalPingIntervalSec
	}
	return time.Duration(seconds) * time.Second
}
