// Package model は監視対象の正規化済み状態（API・DB・エンジン共通のJSON型）を定義する。
package model

import "time"

// SwitchConfig は登録スイッチ（プロファイル配下）。
type SwitchConfig struct {
	ID        int64  `json:"id"`
	ProfileID int64  `json:"profile_id"`
	Name      string `json:"name"`
	GroupTag  string `json:"group_tag"`
	IP        string `json:"ip"`
	SNMPPort  int    `json:"snmp_port"`
	Community string `json:"community,omitempty"`
	Enabled   bool   `json:"enabled"`
	WebURL    string `json:"web_url,omitempty"` // 手動上書き
}

// Severity 定数
const (
	SevCritical = "CRITICAL"
	SevWarning  = "WARNING"
	SevInfo     = "INFO"
)

// SwitchState はスイッチ1台の最新状態。
type SwitchState struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	IP          string     `json:"ip"`
	GroupTag    string     `json:"group_tag"`
	Reachable   bool       `json:"reachable"`
	LastError   string     `json:"last_error,omitempty"`
	LastPoll    time.Time  `json:"last_poll"`
	SysName     string     `json:"sys_name"`
	SysDescr    string     `json:"sys_descr"`
	SysObjectID string     `json:"sys_object_id"`
	Model       string     `json:"model"`
	FW          string     `json:"fw"`
	Serial      string     `json:"serial"`
	UptimeSec   int64      `json:"uptime_sec"`
	BaseMAC     string     `json:"base_mac"`
	Ports       []*Port    `json:"ports"`
	Health      Health     `json:"health"`
	IGMP        IGMP       `json:"igmp"`
	DHCP        DHCP       `json:"dhcp"`
	Config      SwitchConf `json:"config"`
	Neighbors   []Neighbor `json:"neighbors"`
	Caps        map[string]string `json:"caps"` // 機能グループ→SUPPORTED/UNSUPPORTED/FALLBACK_USED
	AlertCounts map[string]int    `json:"alert_counts"`
}

// Port はポート1つの状態と直近レート。
type Port struct {
	IfIndex   int     `json:"if_index"`
	Name      string  `json:"name"` // ifName（1/0/1, lag 1）
	Alias     string  `json:"alias"`
	Memo      string  `json:"memo,omitempty"`
	Type      string  `json:"type"` // rj45/sfp/lag/other
	Oper      string  `json:"oper"`
	SpeedMbps int     `json:"speed_mbps"`
	Duplex    string  `json:"duplex"`
	PVID      int     `json:"pvid"`
	RxBps     float64 `json:"rx_bps"`
	TxBps     float64 `json:"tx_bps"`
	UtilRx    float64 `json:"util_rx"`
	UtilTx    float64 `json:"util_tx"`
	RxMcPps   float64 `json:"rx_mc_pps"`
	TxMcPps   float64 `json:"tx_mc_pps"`
	RxBcPps   float64 `json:"rx_bc_pps"`

	// 直近ポーリング間隔の増分
	ErrDelta     uint64 `json:"err_delta"`
	DiscardDelta uint64 `json:"discard_delta"`
	FCSDelta     uint64 `json:"fcs_delta"`
	SymbolDelta  uint64 `json:"symbol_delta"`
	AlignDelta   uint64 `json:"align_delta"`
	TooLongDelta uint64 `json:"too_long_delta"`
	LateColDelta uint64 `json:"late_col_delta"`
	PauseDelta   uint64 `json:"pause_delta"`
	LPIDelta     uint64 `json:"lpi_delta"`
	CosDropDelta uint64 `json:"cos_drop_delta"`

	// ソフト・カウンタゼロリセット基準からの累積（UI観測モード用）
	ErrSinceReset     uint64 `json:"err_since_reset"`
	DiscardSinceReset uint64 `json:"discard_since_reset"`

	PoE      *PoE    `json:"poe,omitempty"`
	SFP      *SFP    `json:"sfp,omitempty"`
	EEE      string  `json:"eee"`
	Pause    string  `json:"pause"`
	MaxFrame int     `json:"max_frame"`
	UDLD     string  `json:"udld"`
	StpState string  `json:"stp_state"`
	Trust    string  `json:"trust"`
	McastCtl bool    `json:"mcast_storm_ctl"`
	BcastCtl bool    `json:"bcast_storm_ctl"`
	Excluded bool    `json:"excluded"`
	AVRole   string  `json:"av_role,omitempty"`
	ExpectedSpeed int `json:"expected_speed_mbps,omitempty"`
	Devices  []Device `json:"devices"`
	Cable    *Cable  `json:"cable,omitempty"`
	LastChange time.Time `json:"last_change"`
}

// PoE はポートのPoE状態。
type PoE struct {
	Detect  string  `json:"detect"`
	Class   int     `json:"class"`
	PowerW  float64 `json:"power_w"`
	CurrentMA int   `json:"current_ma"`
	Volts   int     `json:"volts"`
	Fault   string  `json:"fault"`
	DeniedCnt   uint64 `json:"denied_cnt"`
	OverloadCnt uint64 `json:"overload_cnt"`
	ShortCnt    uint64 `json:"short_cnt"`
}

// SFP はDOM値。
type SFP struct {
	RxDBm   float64 `json:"rx_dbm"`
	TxDBm   float64 `json:"tx_dbm"`
	TempC   float64 `json:"temp_c"`
	Volts   float64 `json:"volts"`
	BiasMA  float64 `json:"bias_ma"`
	LOS     bool    `json:"los"`
	TxFault bool    `json:"tx_fault"`
	Vendor  string  `json:"vendor"`
	PartNo  string  `json:"part_no"`
	Supported string `json:"supported"`
}

// Cable はTDR結果。
type Cable struct {
	Status  string `json:"status"`
	MinLen  int    `json:"min_len_m"`
	MaxLen  int    `json:"max_len_m"`
	FailLoc int    `json:"fail_loc_m"`
}

// Device はポートで観測された接続機器。
type Device struct {
	MAC       string `json:"mac"`
	Vendor    string `json:"vendor,omitempty"`
	IP        string `json:"ip,omitempty"`
	VLAN      int    `json:"vlan"`
	LLDPName  string `json:"lldp_name,omitempty"`
	LLDPDesc  string `json:"lldp_desc,omitempty"`
	LLDPPort  string `json:"lldp_port,omitempty"`
	Label     string `json:"label,omitempty"`
	Source    string `json:"source"` // lldp/dhcp-snoop/arp/fdb
	LinkLocal bool   `json:"link_local,omitempty"`
}

// Neighbor はLLDP隣接（トポロジー用）。
type Neighbor struct {
	LocalIfIndex int    `json:"local_if_index"`
	LocalPort    string `json:"local_port"`
	ChassisID    string `json:"chassis_id"`
	SysName      string `json:"sys_name"`
	SysDesc      string `json:"sys_desc"`
	PortID       string `json:"port_id"`
	MgmtIP       string `json:"mgmt_ip"`
	IsNetgear    bool   `json:"is_netgear"`
}

// Health は筐体ヘルス。
type Health struct {
	CPU5s           float64     `json:"cpu_5s"`
	CPU60s          float64     `json:"cpu_60s"`
	CPU300s         float64     `json:"cpu_300s"`
	MemFreePct      float64     `json:"mem_free_pct"`
	MbufUsedPct     float64     `json:"mbuf_used_pct"`
	MbufAllocFails  uint64      `json:"mbuf_alloc_fails"`
	MbufFailDelta   uint64      `json:"mbuf_fail_delta"`
	TempMaxC        float64     `json:"temp_max_c"`
	TempState       string      `json:"temp_state"`
	Temps           []float64   `json:"temps"`
	Fans            []Item      `json:"fans"`
	PSUs            []Item      `json:"psus"`
	PoEBudgetW      float64     `json:"poe_budget_w"`
	PoEUsedW        float64     `json:"poe_used_w"`
	Processes       []Process   `json:"processes"`
}

// Item はファン・電源ユニット。
type Item struct {
	Index int    `json:"index"`
	State string `json:"state"`
	Speed string `json:"speed,omitempty"`
	Duty  string `json:"duty,omitempty"`
}

// Process はプロセス別CPU。
type Process struct {
	Name string  `json:"name"`
	Pct  float64 `json:"pct"`
	Raw  string  `json:"raw"`
}

// IGMP はSnooping/クエリア/グループ。
type IGMP struct {
	SnoopingOn bool            `json:"snooping_on"`
	Vlans      []VlanIGMP      `json:"vlans"`
	Groups     []McastGroup    `json:"groups"`
	Mrouters   []Mrouter       `json:"mrouters"`
	MVR        bool            `json:"mvr"`
	GPTP       *GPTP           `json:"gptp,omitempty"`
	MSRPStreams int            `json:"msrp_streams"`
}

// VlanIGMP はVLANのSnooping・クエリア状態。
type VlanIGMP struct {
	VLAN         int    `json:"vlan"`
	Name         string `json:"name"`
	Snooping     bool   `json:"snooping"`
	FastLeave    bool   `json:"fast_leave"`
	QuerierAdmin bool   `json:"querier_admin"`
	QuerierOper  string `json:"querier_oper"`
	QuerierAddr  string `json:"querier_addr"`
	LastQuerier  string `json:"last_querier"`
	LastVersion  int    `json:"last_version"`
	OperVersion  int    `json:"oper_version"`
}

// McastGroup はマルチキャストグループ×ポート。
type McastGroup struct {
	VLAN     int      `json:"vlan"`
	Group    string   `json:"group"`
	Ports    []string `json:"ports"`
	Reporter string   `json:"reporter,omitempty"`
	Protocol string   `json:"protocol"` // Dante/AES67/NDI/sACN等の推定
	Source   string   `json:"source"`   // ssm/l2
}

// Mrouter はMRouterポート。
type Mrouter struct {
	Port string `json:"port"`
	VLAN int    `json:"vlan"`
}

// GPTP は802.1AS状態。
type GPTP struct {
	ClockID       string `json:"clock_id"`
	Grandmaster   string `json:"grandmaster"`
	StepsRemoved  int    `json:"steps_removed"`
}

// DHCP はサーバー・Snooping状態。
type DHCP struct {
	ServerOn     bool        `json:"server_on"`
	Pools        []string    `json:"pools"`
	Leases       []Lease     `json:"leases"`
	Conflicts    []string    `json:"conflicts"`
	SnoopingOn   bool        `json:"snooping_on"`
	InvalidServerMsgs uint64 `json:"invalid_server_msgs"`
	InvalidDelta uint64      `json:"invalid_delta"`
	DeclineRx    uint64      `json:"decline_rx"`
}

// Lease はDHCPリース。
type Lease struct {
	IP   string `json:"ip"`
	MAC  string `json:"mac"`
	Pool string `json:"pool"`
	Port string `json:"port,omitempty"`
}

// SwitchConf はSlow周期の設定値（監査用）。
type SwitchConf struct {
	StpVersion   string `json:"stp_version"`
	StpPriority  int    `json:"stp_priority"`
	BridgeID     string `json:"bridge_id"`
	RootID       string `json:"root_id"`
	IsRoot       bool   `json:"is_root"`
	TopoChanges  uint64 `json:"topo_changes"`
	Bonjour      bool   `json:"bonjour"`
	BcastStormGlobal bool `json:"bcast_storm_global"`
	McastStormGlobal bool `json:"mcast_storm_global"`
	HTTPS        bool   `json:"https"`
	HTTPSPort    int    `json:"https_port"`
	SSH          bool   `json:"ssh"`
	SSHPort      int    `json:"ssh_port"`
	TelnetPort   int    `json:"telnet_port"`
}

// Event はTrap・異常・接続変化・内部ログ・マーカーの統合イベント。
type Event struct {
	ID       int64          `json:"id"`
	TS       time.Time      `json:"ts"`
	SwitchID int64          `json:"switch_id"`
	IfIndex  int            `json:"if_index"`
	Category string         `json:"category"` // TRAP/ANOMALY/CONNECT/SYSLOG/BASELINE/MARKER/SYSTEM
	Severity string         `json:"severity"`
	Code     string         `json:"code"`
	Title    string         `json:"title"`
	Detail   map[string]any `json:"detail,omitempty"`
	Hint     string         `json:"hint,omitempty"`
	Acked    bool           `json:"acked"`
	SwitchName string       `json:"switch_name,omitempty"`
	Dedupe   string         `json:"-"` // 重複排除キー（内部ログ等）
}
