package mib

// TrapDef は受信Trapの識別情報。OIDはMIBから機械解決したもの（仕様書4.3・9.1）。
type TrapDef struct {
	Name     string
	Severity string // CRITICAL / WARNING / INFO
	Title    string
}

// snmpTrapOID.0（SNMPv2-MIB）。v2c Trap の2番目のVarBindに入る。
const SnmpTrapOID = "1.3.6.1.6.3.1.1.4.1.0"

// Traps は通知OID→定義。未登録OIDは呼び出し側で生OIDとして保存する。
var Traps = map[string]TrapDef{
	"1.3.6.1.6.3.1.1.5.1": {"coldStart", "WARNING", "スイッチ起動（コールドスタート）"},
	"1.3.6.1.6.3.1.1.5.2": {"warmStart", "WARNING", "スイッチ再起動（ウォームスタート）"},
	"1.3.6.1.6.3.1.1.5.3": {"linkDown", "WARNING", "ポート リンクダウン"},
	"1.3.6.1.6.3.1.1.5.4": {"linkUp", "INFO", "ポート リンクアップ"},
	"1.3.6.1.6.3.1.1.5.5": {"authenticationFailure", "INFO", "SNMP認証失敗"}, // SNMPv2-MIB

	"1.0.8802.1.1.2.0.0.1": {"lldpRemTablesChange", "INFO", "LLDP隣接機器の変化"},

	"1.3.6.1.2.1.105.0.1": {"pethPsePortOnOffNotification", "INFO", "PoE給電状態の変化"},
	"1.3.6.1.2.1.105.0.2": {"pethMainPowerUsageOnNotification", "WARNING", "PoE電力使用量がしきい値超過"},
	"1.3.6.1.2.1.105.0.3": {"pethMainPowerUsageOffNotification", "INFO", "PoE電力使用量がしきい値未満に回復"},

	"1.3.6.1.4.1.4526.10.43.0.1": {"boxsFanStateChange", "CRITICAL", "ファン状態の変化"},
	"1.3.6.1.4.1.4526.10.43.0.2": {"boxsPowSupplyStateChange", "CRITICAL", "電源ユニット状態の変化"},
	"1.3.6.1.4.1.4526.10.43.0.3": {"boxsTemperatureChange", "WARNING", "温度の変化"},
	"1.3.6.1.4.1.4526.10.43.0.4": {"boxsThermalShutdown", "CRITICAL", "温度異常によるシャットダウン"},
	"1.3.6.1.4.1.4526.10.43.0.5": {"boxsTemperatureStateChange", "WARNING", "温度状態の変化"},
	"1.3.6.1.4.1.4526.10.43.0.6": {"boxsDyingGaspEventChange", "CRITICAL", "電源喪失（Dying Gasp）"},

	"1.3.6.1.4.1.4526.10.1.0.2":  {"broadcastStormStartTrap", "CRITICAL", "ブロードキャストストーム開始"},
	"1.3.6.1.4.1.4526.10.1.0.3":  {"broadcastStormEndTrap", "INFO", "ブロードキャストストーム終了"},
	"1.3.6.1.4.1.4526.10.1.0.4":  {"linkFailureTrap", "WARNING", "リンク障害"},
	"1.3.6.1.4.1.4526.10.1.0.9":  {"fanFailureTrap", "CRITICAL", "ファン障害"}, // FASTPATH-SWITCHING-MIB の旧通知
	"1.3.6.1.4.1.4526.10.1.0.10": {"stpInstanceNewRootTrap", "WARNING", "STPルートブリッジ変更"},
	"1.3.6.1.4.1.4526.10.1.0.11": {"stpInstanceTopologyChangeTrap", "WARNING", "STPトポロジー変更"},
	"1.3.6.1.4.1.4526.10.1.0.12": {"powerSupplyStatusChangeTrap", "CRITICAL", "電源ユニット状態の変化"}, // FASTPATH-SWITCHING-MIB の旧通知
	"1.3.6.1.4.1.4526.10.1.0.13": {"failedUserLoginTrap", "INFO", "スイッチへのログイン認証失敗"},
	"1.3.6.1.4.1.4526.10.1.0.15": {"daiIntfErrorDisabledTrap", "CRITICAL", "DAIによるポートErrDisable"},
	"1.3.6.1.4.1.4526.10.1.0.16": {"stpInstanceLoopInconsistentStartTrap", "CRITICAL", "STPループ不整合検知"},
	"1.3.6.1.4.1.4526.10.1.0.17": {"stpInstanceLoopInconsistentEndTrap", "INFO", "STPループ不整合解消"},
	"1.3.6.1.4.1.4526.10.1.0.18": {"dhcpSnoopingIntfErrorDisabledTrap", "CRITICAL", "DHCP SnoopingによるポートErrDisable"},
	"1.3.6.1.4.1.4526.10.1.0.20": {"agentSwitchIpAddressConflictTrap", "CRITICAL", "IPアドレス重複検知"},
	"1.3.6.1.4.1.4526.10.1.0.21": {"agentSwitchCpuRisingThresholdTrap", "WARNING", "CPU使用率上昇しきい値超過"},
	"1.3.6.1.4.1.4526.10.1.0.22": {"agentSwitchCpuFallingThresholdTrap", "INFO", "CPU使用率が回復"},
	"1.3.6.1.4.1.4526.10.1.0.23": {"agentSwitchCpuFreeMemBelowThresholdTrap", "WARNING", "空きメモリ低下"},
	"1.3.6.1.4.1.4526.10.1.0.27": {"agentSwitchMbufRisingThresholdTrap", "WARNING", "Mbuf使用率上昇"},
	"1.3.6.1.4.1.4526.10.1.0.28": {"agentSwitchMbufFallingThresholdTrap", "INFO", "Mbuf使用率回復"},
}

// TrapNeedsSlow はポートやトポロジーの変化を判定するためFDB・LLDPも再取得すべき通知を選ぶ。
func TrapNeedsSlow(code string) bool {
	switch code {
	case "coldStart", "warmStart", "linkDown", "linkUp", "linkFailureTrap", "lldpRemTablesChange",
		"pethPsePortOnOffNotification", "pethMainPowerUsageOnNotification", "pethMainPowerUsageOffNotification",
		"broadcastStormStartTrap", "broadcastStormEndTrap", "stpInstanceNewRootTrap", "stpInstanceTopologyChangeTrap",
		"stpInstanceLoopInconsistentStartTrap", "stpInstanceLoopInconsistentEndTrap", "daiIntfErrorDisabledTrap", "dhcpSnoopingIntfErrorDisabledTrap":
		return true
	}
	return false
}

// 名前→OIDの逆引き（シミュレータのTrap送信で使用）
func TrapOID(name string) string {
	for oid, d := range Traps {
		if d.Name == name {
			return oid
		}
	}
	return ""
}
