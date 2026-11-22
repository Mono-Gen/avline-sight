package engine

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

const (
	checkPass    = "pass"
	checkReview  = "review"
	checkPending = "pending"

	checklistTimeout      = 5 * time.Second
	recentErrorWindow     = 5 * time.Minute
	recentConfigWindow    = 15 * time.Minute
	clockToleranceSeconds = 60
	sfpMarginDB           = 3
	poeMarginPoints       = 10
	temperatureMarginC    = 5
	invalidOpticalReading = -40
)

type ChecklistTarget struct {
	SwitchID    int64  `json:"switch_id,omitempty"`
	IfIndex     int    `json:"if_index,omitempty"`
	MAC         string `json:"mac,omitempty"`
	Label       string `json:"label,omitempty"`
	Destination string `json:"destination,omitempty"`
}

type ChecklistFinding struct {
	State  string          `json:"state"`
	Reason string          `json:"reason"`
	Target ChecklistTarget `json:"target"`
}

type ChecklistItem struct {
	ID       string             `json:"id"`
	State    string             `json:"state"`
	Reason   string             `json:"reason"`
	Action   string             `json:"action"`
	Optional bool               `json:"optional"`
	Findings []ChecklistFinding `json:"findings,omitempty"`
}

type ShowChecklist struct {
	Items []ChecklistItem `json:"items"`
}

func (c ShowChecklist) remaining() []ChecklistItem {
	var out []ChecklistItem
	for _, item := range c.Items {
		if item.State != checkPass {
			out = append(out, item)
		}
	}
	return out
}

func checklistText(english bool, ja, en string) string {
	if english {
		return en
	}
	return ja
}

func (item *ChecklistItem) add(state, reason string, target ChecklistTarget) {
	if item.State == checkPass || state == checkReview && item.State == checkPending {
		item.State, item.Reason = state, reason
	}
	item.Findings = append(item.Findings, ChecklistFinding{State: state, Reason: reason, Target: target})
}

func switchTarget(s *model.SwitchState) ChecklistTarget {
	return ChecklistTarget{SwitchID: s.ID, Label: s.Name}
}

func portTarget(s *model.SwitchState, index int, name string) ChecklistTarget {
	if name == "" && index > 0 {
		name = "#" + strconv.Itoa(index)
	}
	target := switchTarget(s)
	target.IfIndex = index
	if name != "" {
		target.Label = strings.TrimSpace(s.Name + " " + name)
	}
	return target
}

func inventoryTarget(row InventoryRow) ChecklistTarget {
	label := row.MAC
	if row.Label != "" {
		label = row.Label + " (" + row.MAC + ")"
	}
	return ChecklistTarget{SwitchID: row.SwitchID, IfIndex: row.IfIndex, MAC: row.MAC, Label: label}
}

func (e *Engine) ShowChecklist(ctx context.Context, trapReady, english bool) (ShowChecklist, error) {
	ctx, cancel := context.WithTimeout(ctx, checklistTimeout)
	defer cancel()
	now := time.Now()
	e.mu.RLock()
	pid, started, sleep := e.profileID, e.startedAt, e.sleepInhibited
	states, alerts := e.statesLocked(), e.alertsLocked(now)
	traps, pings, settings := maps.Clone(e.lastTraps), maps.Clone(e.pingResults), e.settings
	missing := maps.Clone(e.missing)
	phase, portCfg := e.phase, maps.Clone(e.portCfg)
	e.mu.RUnlock()

	aliases, err := e.st.AliasesContext(ctx, pid)
	if err != nil {
		return ShowChecklist{}, fmt.Errorf("read registered devices: %w", err)
	}
	last, err := e.st.LastSeenByMACContext(ctx, pid, now)
	if err != nil {
		return ShowChecklist{}, fmt.Errorf("read device locations: %w", err)
	}
	activity, err := e.st.ChecklistActivity(ctx, pid, now.Add(-recentErrorWindow), now.Add(-recentConfigWindow), now)
	if err != nil {
		return ShowChecklist{}, fmt.Errorf("read recent monitoring activity: %w", err)
	}

	l := func(ja, en string) string { return checklistText(english, ja, en) }
	items := make([]ChecklistItem, 0, 15)
	item := func(id, reason, action string, optional bool) *ChecklistItem {
		items = append(items, ChecklistItem{ID: id, State: checkPass, Reason: reason, Action: action, Optional: optional})
		return &items[len(items)-1]
	}
	reach := item("reach", l("全スイッチと通信できています。", "All monitored switches are reachable."), l("電源と管理ネットワークを確認してください。", "Check switch power and the management network."), false)
	trap := item("trap", l("起動後に全スイッチから受信しました。", "Every monitored switch sent a trap since startup."), l("スイッチのTrap送信先と待受ポートを確認してください。", "Check the trap destination and listening port."), false)
	clock := item("clock", l("全スイッチの時刻差は60秒以内です。", "Every switch clock is within 60 seconds."), l("監視PCとスイッチの時刻・時刻同期設定を確認してください。", "Check the PC and switch clock and time synchronization settings."), false)
	device := item("devices", l("すべての登録機器を検出しています。", "All registered devices are visible."), l("機器の電源と接続を確認してください。", "Check device power and cabling."), false)
	unacked := item("unacked", l("未確認のアラートはありません。", "There are no unacknowledged alerts."), l("該当アラートを確認してください。", "Review the affected alerts."), false)
	active := item("active", l("発生中のアラートはありません。", "There are no active alerts."), l("該当アラートの原因を確認してください。", "Check the cause of the affected alerts."), false)
	initial := item("initial", l("全スイッチの初回取得が完了しました。", "Initial collection completed on all monitored switches."), l("通信を確認し、Slow周期の取得を待ってください。", "Check connectivity and wait for the slow collection cycle."), false)
	host := item("host", l("Trap待受とスリープ抑止が有効です。", "Trap listening and sleep prevention are active."), l("Trap待受設定と監視PCの電源設定を確認してください。", "Check trap listener and PC power settings."), false)
	excluded := item("excluded", l("監視除外ポートはありません。", "No ports are excluded from monitoring."), l("意図した監視除外か確認してください。", "Verify these exclusions are intentional."), true)
	ping := item("ping", l("登録機器の直近Pingと設定済みの予定IPに問題はありません。", "Recent pings and configured planned IPs are OK."), l("機器の疎通と予定IPを確認してください。", "Check device connectivity and the planned IP."), true)
	recent := item("recent", l("直近5分にエラー・ドロップ・リンク断続の記録はありません。", "No errors, drops or link interruptions were recorded in the last five minutes."), l("該当箇所の配線とログを確認してください。", "Check cabling and logs for the affected target."), true)
	optics := item("optics", l("計測できる光リンクに受光余裕があります。本番開始時に基準値を保存します。", "Measurable optical links have receive power headroom. Starting the show saves the baseline."), l("光端面、曲げ、モジュールを確認してください。", "Check fiber ends, bends and modules."), true)
	uplink := item("uplink", l("検出した上位回線のリンク・速度・STPに問題はありません。", "Detected uplink link, speed and STP checks passed."), l("上位回線の両端のリンク速度とSTPを確認してください。", "Check the link speed and STP at both ends of the uplink."), true)
	power := item("power", l("計測できるPoE・温度と冗長電源に問題はありません。", "Measured PoE, temperature and redundant power checks passed."), l("PoE負荷・冷却・電源ユニットを確認してください。", "Check PoE load, cooling and power supplies."), true)
	config := item("config", l("直近15分にポート設定変更の記録はありません。", "No port configuration changes were recorded in the last 15 minutes."), l("ログで意図した設定変更か確認してください。", "Review the logs to verify the change was intentional."), true)

	if len(states) == 0 {
		reach.add(checkReview, l("監視対象のスイッチがありません。", "No switches are being monitored."), ChecklistTarget{Destination: "settings"})
		noSwitch := l("判定するスイッチがありません。", "There are no monitored switches to check.")
		for _, check := range []*ChecklistItem{trap, clock, initial} {
			check.add(checkReview, noSwitch, ChecklistTarget{Destination: "settings"})
		}
	}
	if !trapReady {
		host.add(checkReview, l("Trap待受が有効ではありません。", "The trap listener is inactive."), ChecklistTarget{Destination: "trap"})
	}
	if !sleep {
		host.add(checkReview, l("スリープ抑止の有効化を確認できません。", "Active sleep prevention could not be verified."), ChecklistTarget{Destination: "settings"})
	}
	byID := make(map[int64]*model.SwitchState, len(states))
	for _, s := range states {
		byID[s.ID] = s
		target := switchTarget(s)
		switch {
		case s.LastPoll.IsZero():
			reach.add(checkPending, l("初回の応答を待っています。", "Waiting for the first polling response."), target)
		case !s.Reachable:
			reach.add(checkReview, l("SNMPポーリングに応答していません。", "The switch did not answer SNMP polling."), target)
		}
		if receipt := traps[s.ID]; receipt.IsZero() || receipt.Before(started) {
			trap.add(checkReview, l("起動後のTrap受信を確認できません。", "No trap has been observed from this switch since startup."), target)
		}
		if !s.Reachable || s.ClockOffsetSeconds == nil || math.IsNaN(*s.ClockOffsetSeconds) {
			clock.add(checkReview, l("監視PCとの時刻差を取得できません。", "The clock offset from the monitoring PC is unavailable."), target)
		} else if offset := *s.ClockOffsetSeconds; math.Abs(offset) > clockToleranceSeconds {
			clock.add(checkReview, fmt.Sprintf(l("監視PCとの時刻差は%+.1f秒です。", "The clock offset from the monitoring PC is %+.1f seconds."), offset), target)
		}
		if s.LastPoll.IsZero() || s.Config.StpVersion == "" {
			state := checkPending
			if !s.LastPoll.IsZero() && !s.Reachable {
				state = checkReview
			}
			initial.add(state, l("初回の設定取得が完了していません。", "Initial switch configuration collection is incomplete."), target)
		}
		for _, index := range slices.Sorted(maps.Keys(portCfg[s.ID])) {
			setting := portCfg[s.ID][index]
			if setting.Excluded {
				name := ""
				for _, p := range s.Ports {
					if p.IfIndex == index {
						name = p.Name
						break
					}
				}
				excluded.add(checkReview, l("監視除外が設定されています。", "Monitoring exclusion is configured."), portTarget(s, index, name))
			}
		}
		for _, p := range s.Ports {
			target := portTarget(s, p.IfIndex, p.Name)
			if p.Excluded && !portCfg[s.ID][p.IfIndex].Excluded {
				excluded.add(checkReview, l("監視除外が設定されています。", "Monitoring exclusion is configured."), target)
			}
			if !s.Reachable {
				continue
			}
			if p.Oper == "up" && p.SFP != nil && p.SFP.LOS {
				optics.add(checkReview, l("光信号の消失を検出しました。", "Loss of optical signal was detected."), target)
			} else if p.Oper == "up" && p.SFP != nil && p.SFP.RxDBm > invalidOpticalReading && settings.Thresholds.SFPRxDBm != 0 &&
				p.SFP.RxDBm < settings.Thresholds.SFPRxDBm+sfpMarginDB {
				optics.add(checkReview, fmt.Sprintf(l("受光 %.1f dBm（余裕目安 %.1f dBm 以上）。", "Receive power %.1f dBm (headroom target %.1f dBm or higher)."),
					p.SFP.RxDBm, settings.Thresholds.SFPRxDBm+sfpMarginDB), target)
			}
			if isUplink(s, p) && (p.Oper != "up" || p.SpeedMbps <= 0 || p.ExpectedSpeed <= 0 ||
				p.SpeedMbps < p.ExpectedSpeed || p.StpState == "discarding") {
				expected := l("期待速度未設定", "no expected speed configured")
				if p.ExpectedSpeed > 0 {
					expected = fmt.Sprintf(l("期待 %d Mbps", "expected %d Mbps"), p.ExpectedSpeed)
				}
				uplink.add(checkReview, fmt.Sprintf(l("リンク %s / %d Mbps / STP %s（%s）。", "Link %s / %d Mbps / STP %s (%s)."),
					p.Oper, p.SpeedMbps, p.StpState, expected), target)
			}
		}
		if s.Reachable {
			h := s.Health
			th := settings.Thresholds
			if h.PoEBudgetW > 0 && th.PoEPct > 0 {
				if used := 100 * h.PoEUsedW / h.PoEBudgetW; used >= math.Max(0, th.PoEPct-poeMarginPoints) && used > 0 {
					power.add(checkReview, fmt.Sprintf(l("PoE使用率 %.1f%%。", "PoE usage is %.1f%%."), used), target)
				}
			}
			if h.TempState == "warning" || h.TempState == "critical" || h.TempState == "shutdown" {
				power.add(checkReview, fmt.Sprintf(l("温度状態 %s。", "Temperature state is %s."), h.TempState), target)
			} else if th.TempC > 0 && (len(h.Temps) > 0 || h.TempMaxC != 0) && h.TempMaxC >= th.TempC-temperatureMarginC {
				power.add(checkReview, fmt.Sprintf(l("最高温度 %.1f °C。", "Peak temperature is %.1f °C."), h.TempMaxC), target)
			}
			if len(h.PSUs) > 1 {
				operational := 0
				for _, psu := range h.PSUs {
					if psu.State == "operational" {
						operational++
					}
				}
				if operational < len(h.PSUs) {
					power.add(checkReview, fmt.Sprintf(l("稼働中の電源は%d/%d台です。", "%d of %d power supplies are operational."), operational, len(h.PSUs)), target)
				}
			}
		}
	}
	registered := make(map[string]bool, len(aliases))
	for _, alias := range aliases {
		if alias.MatchType == "mac" {
			registered[strings.ToUpper(alias.MatchKey)] = true
		}
	}
	pingInterval := settings.SetupPingIntervalSec
	if phase == "rehearsal" {
		pingInterval = settings.RehearsalPingIntervalSec
	}
	pingCutoff := now.Add(-2 * time.Duration(pingInterval) * time.Second)
	for _, row := range BuildInventory(states, aliases, last, InventoryOptions{}, now) {
		if !registered[row.MAC] {
			continue
		}
		target := inventoryTarget(row)
		if missing[row.MAC].confirmed {
			row.Seen = false
		}
		if !row.Seen {
			device.add(checkReview, l("現在は機器を検出できません。", "The registered device is not currently visible."), target)
		}
		if row.PlannedIP != "" && (!row.Seen || row.IPStale || row.IP != row.PlannedIP) {
			ping.add(checkReview, fmt.Sprintf(l("観測IP %s / 予定IP %s を照合してください。", "Verify observed IP %s against planned IP %s."), row.IP, row.PlannedIP), target)
		}
		result, known := pings[row.MAC]
		switch {
		case !row.Seen || row.IP == "" || row.IPStale || !known || result.IP != row.IP || result.TS.Before(pingCutoff):
			ping.add(checkReview, l("現在のIPへの新しいPing結果を確認できません。", "No fresh ping result is available for the current IP."), target)
		case !result.OK:
			ping.add(checkReview, l("直近のPingに応答がありません。", "The most recent ping did not receive a reply."), target)
		}
	}
	for _, a := range alerts {
		if a.Severity != model.SevWarning && a.Severity != model.SevCritical {
			continue
		}
		target := ChecklistTarget{SwitchID: a.SwitchID, IfIndex: a.IfIndex, MAC: a.MAC, Label: strings.TrimSpace(a.SwitchName + " " + a.Port)}
		if a.SwitchID == 0 && a.MAC == "" {
			target.Destination = "logs"
		}
		if target.Label == "" {
			target.Label = a.MAC
		}
		reason := fmt.Sprintf(l("アラート %s が残っています。", "Alert %s remains."), a.Code)
		if !a.Acked {
			unacked.add(checkReview, reason, target)
		}
		if a.Active {
			active.add(checkReview, reason, target)
		}
	}
	for _, entry := range activity {
		s := byID[entry.SwitchID]
		if s == nil {
			continue
		}
		name := ""
		for _, p := range s.Ports {
			if p.IfIndex == entry.IfIndex {
				name = p.Name
				break
			}
		}
		target := portTarget(s, entry.IfIndex, name)
		if entry.Config {
			config.add(checkReview, l("直近15分に設定変更が記録されました。", "A configuration change was recorded in the last 15 minutes."), target)
		} else {
			recent.add(checkReview, l("直近5分にエラー・ドロップ・リンク断続の記録があります。", "Error, drop or link interruption activity was recorded in the last five minutes."), target)
		}
	}
	return ShowChecklist{Items: items}, nil
}
