package mib

import "strconv"

// Enum は MIB の INTEGER 列挙を表す。未知値は生値を文字列化して返す（仕様: 未知値は生値表示）。
type Enum map[int]string

// Name は値に対応する名前を返す。未定義の値は "unknown(<値>)" とする。
func (e Enum) Name(v int) string {
	if s, ok := e[v]; ok {
		return s
	}
	return "unknown(" + strconv.Itoa(v) + ")"
}

// 出典: fastpath_boxservices.my（v13.0.7.6）
var (
	// boxServicesFanItemState / boxServicesPowSupplyItemState
	BoxItemState = Enum{1: "notpresent", 2: "operational", 3: "failed", 4: "powering", 5: "nopower", 6: "notpowering", 7: "incompatible"}
	// TC BoxsTemperatureStatus
	BoxTempStatus = Enum{0: "low", 1: "normal", 2: "warning", 3: "critical", 4: "shutdown", 5: "notpresent", 6: "notoperational"}
)

// 出典: if.my / etherlike.my / power_ethernet.my / fastpath_power_ethernet.my / fastpathswitching.my
var (
	IfOperStatusEnum = Enum{1: "up", 2: "down", 3: "testing", 4: "unknown", 5: "dormant", 6: "notPresent", 7: "lowerLayerDown"}
	DuplexEnum       = Enum{1: "unknown", 2: "halfDuplex", 3: "fullDuplex"}
	PauseModeEnum    = Enum{1: "disabled", 2: "enabledXmit", 3: "enabledRcv", 4: "enabledXmitAndRcv"}
	PethDetectEnum   = Enum{1: "disabled", 2: "searching", 3: "deliveringPower", 4: "fault", 5: "test", 6: "otherFault"}
	PethFaultEnum    = Enum{0: "none", 1: "mpsAbsent", 2: "short", 3: "overload", 4: "powerDenied", 5: "thermalShutdown", 6: "startupFailure"}
	QuerierOperEnum  = Enum{0: "disabled", 1: "querier", 2: "non-querier"}
	StpVersionEnum   = Enum{1: "stp", 2: "rstp", 3: "mstp"}
	StpFwdStateEnum  = Enum{1: "discarding", 2: "learning", 3: "forwarding", 4: "disabled", 5: "manualFwd", 6: "notParticipate"}
	UdldOperEnum     = Enum{1: "shutdown", 2: "undetermined", 3: "biDirectional", 4: "notApplicable", 5: "undetermined-LinkDown"}
	CableStatusEnum  = Enum{1: "normal", 2: "open", 3: "short", 4: "unknown"}
	TrustModeEnum    = Enum{1: "untrusted", 2: "trustDot1p", 3: "trustIpPrecedence", 4: "trustIpDscp", 5: "trustMplsExp"}
	GreenModeEnum    = Enum{0: "disable", 1: "enable", 2: "not-applicable"}
)

// TruthValue: 1=true, 2=false
func Truth(v int) bool { return v == 1 }
