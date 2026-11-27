// Package mib は M4250 FW v13.0.7.6 のMIBから機械検証したOID・Enum・Trap定義を保持する。
// 値の出典: .docs/M4250L-v13.0.7.6-mibs/ を独自パーサで解決した結果（仕様書9章）。
// OIDはすべて先頭ドットなしの数値表記。テーブル列はインデックス部を含まない列OID。
package mib

// システム基本（SNMPv2-MIB / fastpathswitching.my agentInventoryGroup）
const (
	SysDescr    = "1.3.6.1.2.1.1.1.0"
	SysObjectID = "1.3.6.1.2.1.1.2.0"
	SysUpTime   = "1.3.6.1.2.1.1.3.0"
	SysName     = "1.3.6.1.2.1.1.5.0"

	AgentInventoryMachineType     = "1.3.6.1.4.1.4526.10.1.1.1.2.0"
	AgentInventoryMachineModel    = "1.3.6.1.4.1.4526.10.1.1.1.3.0"
	AgentInventorySerialNumber    = "1.3.6.1.4.1.4526.10.1.1.1.4.0"
	AgentInventorySoftwareVersion = "1.3.6.1.4.1.4526.10.1.1.1.13.0"

	// agentSupportedMibTable 列
	AgentSupportedMibName = "1.3.6.1.4.1.4526.10.1.1.3.1.2"

	NetgearEnterprise = "1.3.6.1.4.1.4526"
)

// スイッチ時刻（NETGEAR-TIMEZONE-PRIVATE-MIB）
const (
	AgentSystemTime               = NetgearEnterprise + ".10.42.1.1.0"
	AgentSystemDate               = NetgearEnterprise + ".10.42.1.2.0"
	AgentSystemTimeSource         = NetgearEnterprise + ".10.42.1.4.0"
	AgentSystemSummerTimeState    = NetgearEnterprise + ".10.42.1.5.0"
	AgentTimeZoneHoursOffset      = NetgearEnterprise + ".10.42.2.1.0"
	AgentTimeZoneMinutesOffset    = NetgearEnterprise + ".10.42.2.2.0"
	AgentSummerTimeMode           = NetgearEnterprise + ".10.42.3.1.0"
	AgentStRecurringZoneOffset    = NetgearEnterprise + ".10.42.3.2.10.0"
	AgentStNonRecurringZoneOffset = NetgearEnterprise + ".10.42.3.3.9.0"
)

// IF-MIB ifTable / ifXTable（INDEX=ifIndex）
const (
	IfDescr       = "1.3.6.1.2.1.2.2.1.2"
	IfType        = "1.3.6.1.2.1.2.2.1.3"
	IfMtu         = "1.3.6.1.2.1.2.2.1.4"
	IfPhysAddress = "1.3.6.1.2.1.2.2.1.6"
	IfAdminStatus = "1.3.6.1.2.1.2.2.1.7"
	IfOperStatus  = "1.3.6.1.2.1.2.2.1.8"
	IfLastChange  = "1.3.6.1.2.1.2.2.1.9" // IF-MIB
	IfInDiscards  = "1.3.6.1.2.1.2.2.1.13"
	IfInErrors    = "1.3.6.1.2.1.2.2.1.14"
	IfOutDiscards = "1.3.6.1.2.1.2.2.1.19"
	IfOutErrors   = "1.3.6.1.2.1.2.2.1.20"

	IfName               = "1.3.6.1.2.1.31.1.1.1.1"
	IfHCInOctets         = "1.3.6.1.2.1.31.1.1.1.6"
	IfHCInMulticastPkts  = "1.3.6.1.2.1.31.1.1.1.8"
	IfHCInBroadcastPkts  = "1.3.6.1.2.1.31.1.1.1.9"
	IfHCOutOctets        = "1.3.6.1.2.1.31.1.1.1.10"
	IfHCOutMulticastPkts = "1.3.6.1.2.1.31.1.1.1.12"
	IfHCOutBroadcastPkts = "1.3.6.1.2.1.31.1.1.1.13"
	IfHighSpeed          = "1.3.6.1.2.1.31.1.1.1.15"
	IfAlias              = "1.3.6.1.2.1.31.1.1.1.18"
)

// agentPortStatsRateTable（INDEX=ifIndex）
const (
	PortRateHCBpsRx   = "1.3.6.1.4.1.4526.10.1.1.8.1.1.9"
	PortRateHCBpsTx   = "1.3.6.1.4.1.4526.10.1.1.8.1.1.10"
	PortRateUtilRx    = "1.3.6.1.4.1.4526.10.1.1.8.1.1.13"
	PortRateUtilTx    = "1.3.6.1.4.1.4526.10.1.1.8.1.1.14"
	PortStatsRateRoot = "1.3.6.1.4.1.4526.10.1.1.8.1"
)

// EtherLike-MIB dot3StatsTable / dot3PauseTable（INDEX=ifIndex）
const (
	Dot3AlignmentErrors  = "1.3.6.1.2.1.10.7.2.1.2"
	Dot3FCSErrors        = "1.3.6.1.2.1.10.7.2.1.3"
	Dot3LateCollisions   = "1.3.6.1.2.1.10.7.2.1.8"
	Dot3FrameTooLongs    = "1.3.6.1.2.1.10.7.2.1.13"
	Dot3SymbolErrors     = "1.3.6.1.2.1.10.7.2.1.18"
	Dot3DuplexStatus     = "1.3.6.1.2.1.10.7.2.1.19" // 1=unknown 2=half 3=full
	Dot3TxLPITransitions = "1.3.6.1.2.1.10.7.2.1.24"
	Dot3RxLPITransitions = "1.3.6.1.2.1.10.7.2.1.25"
	Dot3PauseOperMode    = "1.3.6.1.2.1.10.7.10.1.2" // 1=disabled 2=xmit 3=rcv 4=both
	Dot3HCInPauseFrames  = "1.3.6.1.2.1.10.7.10.1.5"
	Dot3HCOutPauseFrames = "1.3.6.1.2.1.10.7.10.1.6"
)

// QoS CoS（fastpath_qos_cos.my）
const (
	CosQueueIntfHCTotalDrop = "1.3.6.1.4.1.4526.10.3.3.2.6.1.1" // INDEX=intf
	CosQueueStatsHCDrop     = "1.3.6.1.4.1.4526.10.3.3.2.7.1.1" // INDEX=intf,queue
	CosMapIntfTrustMode     = "1.3.6.1.4.1.4526.10.3.3.1.3.1.2" // 1=untrusted 2=dot1p 3=ipPrec 4=ipDscp
	CosMapIpDscpTrafficCls  = "1.3.6.1.4.1.4526.10.3.3.1.2.1.3" // INDEX=intf,dscp
	CpuCosQDrops            = "1.3.6.1.4.1.4526.10.1.1.6.1.1.2"
)

// IGMP Snooping（fastpathswitching.my）
const (
	SnoopingAdminMode  = "1.3.6.1.4.1.4526.10.1.2.8.6.1.1.2" // INDEX=protocol 1=en 2=dis
	SnoopVlanAdminMode = "1.3.6.1.4.1.4526.10.1.2.8.8.1.1.1" // INDEX=vlan,protocol 0=dis 1=en
	SnoopVlanFastLeave = "1.3.6.1.4.1.4526.10.1.2.8.8.1.1.4"
	// agentSwitchSnoopingQuerierCfgTable（INDEX=protocol 1=IPv4）の全体アドレス。実機では未設定で 0.0.0.0
	QuerierGlobalAddress       = "1.3.6.1.4.1.4526.10.1.2.8.20.1.1.3"
	QuerierVlanAdminMode       = "1.3.6.1.4.1.4526.10.1.2.8.20.2.1.1"
	QuerierVlanOperMode        = "1.3.6.1.4.1.4526.10.1.2.8.20.2.1.2" // 0=disabled 1=querier 2=non-querier
	QuerierVlanAddress         = "1.3.6.1.4.1.4526.10.1.2.8.20.2.1.4"
	QuerierOperVersion         = "1.3.6.1.4.1.4526.10.1.2.8.20.2.1.5"
	QuerierLastQuerierAddress  = "1.3.6.1.4.1.4526.10.1.2.8.20.2.1.7"
	QuerierLastQuerierVersion  = "1.3.6.1.4.1.4526.10.1.2.8.20.2.1.8"
	SSMGroupLastReporter       = "1.3.6.1.4.1.4526.10.1.2.8.34.1.5" // INDEX=atype,ifIndex,vlan,group
	SSMFDBIncludePortList      = "1.3.6.1.4.1.4526.10.1.2.8.36.1.5"
	VlanStaticMrouterAdminMode = "1.3.6.1.4.1.4526.10.1.2.8.9.1.1.1" // INDEX=ifIndex,vlan,protocol
	Dot1qTpGroupEgressPorts    = "1.3.6.1.2.1.17.7.1.2.3.1.2"        // INDEX=vlan,mac
	MulticastStormAction       = "1.3.6.1.4.1.4526.10.1.2.8.12.14"
	BroadcastControlModeGlobal = "1.3.6.1.4.1.4526.10.1.2.8.12.4.0"
	MulticastControlModeGlobal = "1.3.6.1.4.1.4526.10.1.2.8.12.6.0"
)

// agentPortConfigTable（INDEX=agentPortDot1dBasePort）
const (
	PortCfgIfIndex           = "1.3.6.1.4.1.4526.10.1.2.13.1.2"
	PortCfgMaxFrameSize      = "1.3.6.1.4.1.4526.10.1.2.13.1.19"
	PortCfgBcastCtrlMode     = "1.3.6.1.4.1.4526.10.1.2.13.1.20" // 1=en 2=dis
	PortCfgMcastCtrlMode     = "1.3.6.1.4.1.4526.10.1.2.13.1.22"
	PortCfgMcastCtrlThresold = "1.3.6.1.4.1.4526.10.1.2.13.1.23"
)

// PoE（POWER-ETHERNET-MIB / fastpath_power_ethernet.my）
const (
	PethMainPower           = "1.3.6.1.2.1.105.1.3.1.1.2" // W
	PethMainOperStatus      = "1.3.6.1.2.1.105.1.3.1.1.3"
	PethMainConsumption     = "1.3.6.1.2.1.105.1.3.1.1.4" // W
	PethMainUsageThreshold  = "1.3.6.1.2.1.105.1.3.1.1.5" // %
	PethPortDetectionStatus = "1.3.6.1.2.1.105.1.1.1.6"   // INDEX=group,port
	PethPortClass           = "1.3.6.1.2.1.105.1.1.1.10"
	PethPortPowerDeniedCnt  = "1.3.6.1.2.1.105.1.1.1.12"
	PethPortOverLoadCnt     = "1.3.6.1.2.1.105.1.1.1.13"
	PethPortShortCnt        = "1.3.6.1.2.1.105.1.1.1.14"
	AgentPethOutputPower    = "1.3.6.1.4.1.4526.10.15.1.1.1.2" // mW, INDEX=group,port
	AgentPethOutputCurrent  = "1.3.6.1.4.1.4526.10.15.1.1.1.3" // mA
	AgentPethOutputVolts    = "1.3.6.1.4.1.4526.10.15.1.1.1.4" // V
	AgentPethFaultStatus    = "1.3.6.1.4.1.4526.10.15.1.1.1.9"
	// agentPethTemperature (.5) は obsolete のため使用禁止
)

// 筐体ヘルス（fastpathswitching.my / fastpath_boxservices.my）
const (
	CpuMemFreePct       = "1.3.6.1.4.1.4526.10.1.1.4.10.0"
	CpuTotal5s          = "1.3.6.1.4.1.4526.10.1.1.4.11.0"
	CpuTotal60s         = "1.3.6.1.4.1.4526.10.1.1.4.12.0"
	CpuTotal300s        = "1.3.6.1.4.1.4526.10.1.1.4.13.0"
	CpuProcessName      = "1.3.6.1.4.1.4526.10.1.1.4.8.1.2"
	CpuProcessPct       = "1.3.6.1.4.1.4526.10.1.1.4.8.1.3" // DisplayString
	MbufsFree           = "1.3.6.1.4.1.4526.10.1.1.7.1.0"
	MbufsTotal          = "1.3.6.1.4.1.4526.10.1.1.7.3.0"
	MbufsUsed           = "1.3.6.1.4.1.4526.10.1.1.7.4.0"
	MbufAllocFails      = "1.3.6.1.4.1.4526.10.1.1.7.2.1.4" // 優先度別テーブル。全行合算
	TempSensorTemp      = "1.3.6.1.4.1.4526.10.43.1.8.1.5"  // INDEX=unit,sensor
	TempUnitState       = "1.3.6.1.4.1.4526.10.43.1.15.1.2" // BoxsTemperatureStatus
	FanItemState        = "1.3.6.1.4.1.4526.10.43.1.6.1.3"  // INDEX=unit,fan
	FanSpeed            = "1.3.6.1.4.1.4526.10.43.1.6.1.4"  // OCTET STRING
	FanDutyLevel        = "1.3.6.1.4.1.4526.10.43.1.6.1.5"  // OCTET STRING
	PowSupplyItemState  = "1.3.6.1.4.1.4526.10.43.1.7.1.3"
	OpticsTemp          = "1.3.6.1.4.1.4526.10.43.1.18.1.2" // ÷10 ℃, INDEX=ifIndex
	OpticsVoltage       = "1.3.6.1.4.1.4526.10.43.1.18.1.3" // ÷1000 V
	OpticsCurrent       = "1.3.6.1.4.1.4526.10.43.1.18.1.4" // ÷10 mA
	OpticsPowerOut      = "1.3.6.1.4.1.4526.10.43.1.18.1.5" // ÷100 dBm
	OpticsPowerIn       = "1.3.6.1.4.1.4526.10.43.1.18.1.6" // ÷100 dBm
	OpticsTxFault       = "1.3.6.1.4.1.4526.10.43.1.18.1.7" // TruthValue
	OpticsLos           = "1.3.6.1.4.1.4526.10.43.1.18.1.8"
	OpticsInfoVendor    = "1.3.6.1.4.1.4526.10.43.1.19.1.2"
	OpticsInfoPartNo    = "1.3.6.1.4.1.4526.10.43.1.19.1.6"
	OpticsInfoSupported = "1.3.6.1.4.1.4526.10.43.1.19.1.10"
)

// 接続機器識別
const (
	LldpLocChassisID        = "1.0.8802.1.1.2.1.3.2.0"
	LldpRemChassisIDSubtype = "1.0.8802.1.1.2.1.4.1.1.4"   // 4=macAddress, 7=local(ホスト名など)
	LldpRemChassisID        = "1.0.8802.1.1.2.1.4.1.1.5"   // INDEX=timeMark,localPort,remIndex
	LldpRemPortIDSubtype    = "1.0.8802.1.1.2.1.4.1.1.6"   // 3=macAddress, 5=interfaceName, 7=local
	Dot1qVlanStaticEgress   = "1.3.6.1.2.1.17.7.1.4.3.1.2" // PortList, INDEX=vlan
	Dot1qVlanStaticUntagged = "1.3.6.1.2.1.17.7.1.4.3.1.4"
	// 実際に流れているVLANメンバー（Auto-Trunk等で動的に加わった分も含む）。INDEX=timeMark,vlan
	Dot1qVlanCurrentEgress   = "1.3.6.1.2.1.17.7.1.4.2.1.4"
	Dot1qVlanCurrentUntagged = "1.3.6.1.2.1.17.7.1.4.2.1.5"
	// agentSwitchportConfigTable（INDEX=ifIndex）。モード access(1) trunk(2) general(3)
	SwitchportMode       = "1.3.6.1.4.1.4526.10.1.2.8.37.1.2"
	SwitchportNativeVlan = "1.3.6.1.4.1.4526.10.1.2.8.37.1.4"
	LldpRemPortID        = "1.0.8802.1.1.2.1.4.1.1.7"
	LldpRemPortDesc      = "1.0.8802.1.1.2.1.4.1.1.8"
	LldpRemSysName       = "1.0.8802.1.1.2.1.4.1.1.9"
	LldpRemSysDesc       = "1.0.8802.1.1.2.1.4.1.1.10"
	LldpRemManAddrIfID   = "1.0.8802.1.1.2.1.4.2.1.4"   // INDEX=...,subtype,len,addr
	Dot1qTpFdbPort       = "1.3.6.1.2.1.17.7.1.2.2.1.2" // INDEX=fdbId,mac(6)
	Dot1qTpFdbStatus     = "1.3.6.1.2.1.17.7.1.2.2.1.3"
	Dot1dBasePortIfIdx   = "1.3.6.1.2.1.17.1.4.1.2"
	IpNetToPhysPhys      = "1.3.6.1.2.1.4.35.1.4"                // INDEX=ifIndex,atype,len,addr
	IpNetToMediaPhys     = "1.3.6.1.2.1.4.22.1.2"                // INDEX=ifIndex,ip(4)
	DsBindingIfIndex     = "1.3.6.1.4.1.4526.10.1.2.8.23.11.1.1" // INDEX=mac
	DsBindingVlan        = "1.3.6.1.4.1.4526.10.1.2.8.23.11.1.2"
	DsBindingIP          = "1.3.6.1.4.1.4526.10.1.2.8.23.11.1.4"
	Dot1qPvid            = "1.3.6.1.2.1.17.7.1.4.5.1.1"
	Dot1qVlanStaticName  = "1.3.6.1.2.1.17.7.1.4.3.1.1"
)

// DHCP（fastpath_dhcp.my / fastpathswitching.my）
const (
	DhcpServerAdminMode    = "1.3.6.1.4.1.4526.10.12.1.1.0" // 1=en 2=dis
	DhcpServerDeclineRx    = "1.3.6.1.4.1.4526.10.12.1.8.0"
	DhcpServerNakTx        = "1.3.6.1.4.1.4526.10.12.1.13.0"
	DhcpServerDiscoverDrop = "1.3.6.1.4.1.4526.10.12.1.16.0"
	DhcpPoolName           = "1.3.6.1.4.1.4526.10.12.2.2.1.2"
	DhcpLeaseIP            = "1.3.6.1.4.1.4526.10.12.3.2.1.1" // INDEX=leaseIndex
	DhcpLeaseHW            = "1.3.6.1.4.1.4526.10.12.3.2.1.3"
	DhcpLeasePool          = "1.3.6.1.4.1.4526.10.12.3.2.1.7"
	DhcpConflictType       = "1.3.6.1.4.1.4526.10.12.4.3.1.2"   // INDEX=ip
	DhcpSnoopingAdminMode  = "1.3.6.1.4.1.4526.10.1.2.8.23.1.0" // TruthValue
	DhcpSnoopVlanEnable    = "1.3.6.1.4.1.4526.10.1.2.8.23.3.1.2"
	DhcpSnoopIfTrust       = "1.3.6.1.4.1.4526.10.1.2.8.23.4.1.1"
	DhcpSnoopInvalidServer = "1.3.6.1.4.1.4526.10.1.2.8.23.7.1.3" // INDEX=ifIndex
)

// L2冗長・診断・AV地雷・ログ・管理アクセス
const (
	StpForceVersion        = "1.3.6.1.4.1.4526.10.1.2.15.5.0" // 1=stp 2=rstp 3=mstp
	StpAdminMode           = "1.3.6.1.4.1.4526.10.1.2.15.6.0"
	StpCstBridgePriority   = "1.3.6.1.4.1.4526.10.1.2.15.8.11.0"
	StpMstBridgeIdentifier = "1.3.6.1.4.1.4526.10.1.2.15.10.1.3" // INDEX=mstId (0=CST)
	StpMstDesignatedRootID = "1.3.6.1.4.1.4526.10.1.2.15.10.1.4"
	StpMstTopoChangeCount  = "1.3.6.1.4.1.4526.10.1.2.15.10.1.8"
	StpCstPortFwdState     = "1.3.6.1.4.1.4526.10.1.2.15.9.1.5" // 1=discarding 3=forwarding
	LagSummaryName         = "1.3.6.1.4.1.4526.10.1.2.2.2.1.2"
	Dot3adAggPortAttached  = "1.2.840.10006.300.43.1.2.1.1.13"
	GreenEeeMode           = "1.3.6.1.4.1.4526.10.55.1.1.1.9" // 0=dis 1=en 2=n/a
	GreenEnergyDetectMode  = "1.3.6.1.4.1.4526.10.55.1.1.1.2"
	UdldIntfOperStatus     = "1.3.6.1.4.1.4526.10.54.1.2.1.1.1" // 1=shutdown 2=undetermined 3=bidir
	CableTesterIfIndex     = "1.3.6.1.4.1.4526.10.1.4.2.0"
	CableTesterCableStatus = "1.3.6.1.4.1.4526.10.1.4.3.0" // 1=normal 2=open 3=short 4=unknown
	CableTesterMinLen      = "1.3.6.1.4.1.4526.10.1.4.4.0"
	CableTesterMaxLen      = "1.3.6.1.4.1.4526.10.1.4.5.0"
	CableTesterFailLoc     = "1.3.6.1.4.1.4526.10.1.4.6.0"
	LogInMemoryMsgText     = "1.3.6.1.4.1.4526.10.14.3.2.1.2" // INDEX=msgIndex
	BonjourAdminMode       = "1.3.6.1.4.1.4526.10.71.1.1.1.0"
	SSLAdminMode           = "1.3.6.1.4.1.4526.10.11.1.1.0"
	SSLSecurePort          = "1.3.6.1.4.1.4526.10.11.1.2.0"
	SSHAdminMode           = "1.3.6.1.4.1.4526.10.11.2.1.0"
	SSHMgmtPortNum         = "1.3.6.1.4.1.4526.10.11.2.10.0"
	TelnetMgmtPortNum      = "1.3.6.1.4.1.4526.10.1.2.1.2.4.0"
	MvrAdminMode           = "1.3.6.1.4.1.4526.10.50.1.1.0"
	MvrGroupStatus         = "1.3.6.1.4.1.4526.10.50.3.1.2"
	MvrIGMPRxFailures      = "1.3.6.1.4.1.4526.10.50.5.9.0"
	AsDefaultClockIdentity = "1.3.111.2.802.1.1.20.1.1.1.0"
	AsStepsRemoved         = "1.3.111.2.802.1.1.20.1.2.1.0"
	AsGrandmasterIdentity  = "1.3.111.2.802.1.1.20.1.3.4.0"
	MsrpStreamVlanID       = "1.3.6.1.4.1.4526.10.60.2.3.1.7.1.4"
	MsrpStreamID           = "1.3.6.1.4.1.4526.10.60.2.3.1.7.1.2"
	MsrpResvDeclType       = "1.3.6.1.4.1.4526.10.60.2.3.1.8.1.3" // 0=talkerAdvertise 1=talkerFailed 2=listenerAskingFailed 3=listenerReady 4=listenerReadyFailed
	MsrpResvFailureCode    = "1.3.6.1.4.1.4526.10.60.2.3.1.8.1.6" // 0=noFailure
	MvrPortMemberRow       = "1.3.6.1.4.1.4526.10.50.4.1.3"       // INDEX=group(4),ifIndex
)
