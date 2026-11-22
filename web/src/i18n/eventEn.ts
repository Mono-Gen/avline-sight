import { eventJa } from './eventJa';
export const eventEn: { [K in keyof typeof eventJa]: { title: string; hint: string } } = {
  "CLOCK_SKEW": {
    "title": "Clock offset from monitoring PC: {offset_seconds} s",
    "hint": "Check time synchronization on the switch and monitoring PC."
  },
  "DEVICE_MISSING": {
    "title": "Registered device missing: {name} ({mac})",
    "hint": "Check device power, cabling and connection location."
  },
  "PING_FAIL": {
    "title": "No Ping response: {name} ({last_ip})",
    "hint": "Check power, routing and ICMP response settings."
  },
  "IP_MISMATCH": {
    "title": "Planned IP mismatch: {name} ({last_ip} → {planned_ip})",
    "hint": "Check the device IP configuration and planned IP."
  },
  "LINK_DOWN": {
    "title": "Link down",
    "hint": "Check cabling, connectors and connected device power."
  },
  "LINK_UP": {
    "title": "Link up",
    "hint": "Check the connected device status."
  },
  "LINK_FLAP": {
    "title": "Transient link interruption ({count})",
    "hint": "Check cabling, connectors and device power."
  },
  "SWITCH_UNREACHABLE": {
    "title": "Switch unreachable",
    "hint": "Check switch power, connectivity and SNMP settings."
  },
  "SWITCH_RECOVERED": {
    "title": "Switch recovered",
    "hint": "Check switch status and incident history."
  },
  "SWITCH_REBOOT": {
    "title": "Switch rebooted",
    "hint": "Check power and the cause of the reboot."
  },
  "HIGH_UTIL": {
    "title": "High bandwidth utilization",
    "hint": "Check congestion, Storm Control and QoS."
  },
  "DISCARDS": {
    "title": "Packet drops +{delta}",
    "hint": "Check congestion, Storm Control and QoS."
  },
  "SPEED_BELOW_EXPECTED": {
    "title": "Link below expected speed",
    "hint": "Check cable pairs, connectors and auto-negotiation."
  },
  "RELINK_SLOWER": {
    "title": "Link speed dropped after reconnect",
    "hint": "Check cable pairs, connectors and auto-negotiation."
  },
  "SPEED_CHANGE": {
    "title": "Link speed changed",
    "hint": "Check expected speed and cabling."
  },
  "POE_PORT_FAULT": {
    "title": "PoE port fault",
    "hint": "Check the PoE budget, powered device and cable shorts."
  },
  "POE_FAULT": {
    "title": "PoE fault",
    "hint": "Check the PoE budget, powered device and cable shorts."
  },
  "POE_STATE": {
    "title": "PoE state changed",
    "hint": "Check the powered device and power status."
  },
  "PHY_ERRORS": {
    "title": "Physical errors FCS+{fcs} Symbol+{symbol} Align+{align}",
    "hint": "Check cables, connectors and SFP contamination or degradation."
  },
  "MTU_MISMATCH": {
    "title": "Possible MTU mismatch",
    "hint": "Match the MTU of the connected device and peer switch."
  },
  "DUPLEX": {
    "title": "Half-duplex / late collision",
    "hint": "Check duplex and auto-negotiation settings at both ends."
  },
  "COS_DROP": {
    "title": "CoS queue drops",
    "hint": "Check bandwidth saturation, microbursts and QoS priorities."
  },
  "PAUSE_FRAMES": {
    "title": "PAUSE frames increased",
    "hint": "Check Flow Control; disabling it is recommended for AV except NDI4."
  },
  "EEE_ON_AV_PORT": {
    "title": "EEE enabled on AV port",
    "hint": "Disable Energy Efficient Ethernet and check synchronization."
  },
  "SFP_LOS": {
    "title": "SFP loss of signal / transmit fault",
    "hint": "Clean optical connectors and check bends, breaks and modules."
  },
  "SFP_LOW_RX": {
    "title": "Low SFP optical RX {rx_dbm} dBm",
    "hint": "Clean optical connectors and check bends, breaks and modules."
  },
  "SFP_RX_DROP": {
    "title": "SFP RX dropped from baseline",
    "hint": "Clean optical connectors and check bends, breaks and modules."
  },
  "MSRP_RESV_FAILED": {
    "title": "AVB / Milan reservation failed",
    "hint": "Check bandwidth, AVB support and VLAN settings."
  },
  "MVR_RX_FAIL": {
    "title": "MVR IGMP receive failures",
    "hint": "Check source and receiver ports and the MVR VLAN."
  },
  "CPU_HIGH": {
    "title": "High CPU usage",
    "hint": "Check switch load, cooling and power."
  },
  "MEM_LOW": {
    "title": "Low free memory",
    "hint": "Check switch load and operating state."
  },
  "TEMP_HIGH": {
    "title": "High temperature",
    "hint": "Check airflow, cooling and ambient temperature."
  },
  "FAN_FAIL": {
    "title": "Fan failure",
    "hint": "Check cooling fans and power."
  },
  "PSU_FAIL": {
    "title": "Power supply failure",
    "hint": "Check power, cables and power supply units."
  },
  "MBUF_ALLOC_FAIL": {
    "title": "Mbuf allocation failure",
    "hint": "Check congestion, microbursts and QoS."
  },
  "POE_BUDGET": {
    "title": "High PoE budget usage",
    "hint": "Check power capacity and device consumption."
  },
  "UDLD": {
    "title": "Unidirectional link fault",
    "hint": "Check both fiber strands and Tx/Rx."
  },
  "STP_BLOCKING": {
    "title": "STP blocking",
    "hint": "Check for physical loops and inspect STP state."
  },
  "STP_MSTP": {
    "title": "MSTP is active",
    "hint": "Check the recommended ProAV RSTP configuration."
  },
  "STP_TCN_BURST": {
    "title": "STP topology change burst",
    "hint": "Check for physical loops and link interruptions."
  },
  "DHCP_ROGUE": {
    "title": "Possible rogue DHCP server",
    "hint": "Check unauthorized routers, duplicate DHCP servers and static IP conflicts."
  },
  "IP_CONFLICT": {
    "title": "IP address conflict",
    "hint": "Resolve conflicts between static IPs and DHCP assignments."
  },
  "AUDIT_PENDING": {
    "title": "Waiting for initial switch polling",
    "hint": "Audit results appear after the first slow polling cycle."
  },
  "FW_MISMATCH": {
    "title": "Firmware mismatch",
    "hint": "Align firmware versions across switches."
  },
  "STP_PRIORITY_DEFAULT": {
    "title": "All switches use default STP priority",
    "hint": "Check priority configuration: core 0, backup core 8192."
  },
  "STP_EDGE_ROOT": {
    "title": "Edge switch is STP root bridge",
    "hint": "Check STP priorities on the core and backup core."
  },
  "DHCP_DOUBLE": {
    "title": "Multiple DHCP servers",
    "hint": "Disable unintended DHCP servers."
  },
  "QUERIER_ON_EDGE": {
    "title": "Edge switch is IGMP querier",
    "hint": "Check querier placement on the core and IP election priorities."
  },
  "SNOOP_MISMATCH": {
    "title": "IGMP snooping mismatch",
    "hint": "Align snooping settings per VLAN."
  },
  "QUERIER_ABSENT": {
    "title": "No IGMP querier",
    "hint": "Enable an IGMP querier on the core switch."
  },
  "QUERIER_CONFLICT": {
    "title": "Multiple querier IPs observed",
    "hint": "Check external query sources and querier election."
  },
  "IGMP_VERSION_MIX": {
    "title": "Mixed IGMP versions",
    "hint": "Check IGMP versions and AV device requirements."
  },
  "FLOWCTRL_ON_AV_PORT": {
    "title": "Flow Control enabled on AV port",
    "hint": "Consider disabling Flow Control on AV ports except NDI4."
  },
  "NDI4_NEEDS_FLOWCTRL": {
    "title": "Flow Control disabled on NDI4",
    "hint": "Enable Flow Control on NDI4 ports."
  },
  "DSCP_TRUST": {
    "title": "AV port QoS does not trust DSCP",
    "hint": "Check Trust DSCP for Dante/AES67/PTP."
  },
  "STORM_CTRL_UPLINK": {
    "title": "Storm Control enabled on uplink",
    "hint": "Check whether Storm Control drops high-bandwidth multicast."
  },
  "QSYS_MTU": {
    "title": "Q-SYS port MTU setting",
    "hint": "Check the Q-SYS MTU 1500 requirement."
  },
  "UPLINK_MTU_MISMATCH": {
    "title": "Uplink MTU mismatch",
    "hint": "Match MTU settings at both ends of the uplink."
  },
  "DAISY_CHAIN": {
    "title": "Multi-stage daisy chain",
    "hint": "Check star connections from the core for PTP synchronization."
  },
  "VLAN_CROWDED": {
    "title": "More than 200 devices in VLAN",
    "hint": "Consider splitting the VLAN."
  },
  "QUERIER_CHANGED": {
    "title": "IGMP querier IP changed",
    "hint": "Check querier election and external query sources."
  },
  "MCAST_FLOOD": {
    "title": "Possible multicast flooding",
    "hint": "Check IGMP snooping and the querier."
  },
  "DEVICE_NEW": {
    "title": "Device detected",
    "hint": "Check connection location and device name."
  },
  "DEVICE_GONE": {
    "title": "Device no longer observed",
    "hint": "Check connection location, power and FDB aging."
  },
  "PHASE_CHANGED": {
    "title": "Operating phase changed: {phase}",
    "hint": "Check Ping and suppression settings for this phase."
  },
  "WORK_STARTED": {
    "title": "Work mode started: {group}",
    "hint": "Alerts for this group are suppressed during work mode."
  },
  "WORK_ENDED": {
    "title": "Work mode ended: {group}",
    "hint": "Monitoring resumed for this group."
  },
  "linkDown": {
    "title": "Link down",
    "hint": "Check cabling, connectors and connected device power."
  },
  "linkUp": {
    "title": "Link up",
    "hint": "Check the connected device status."
  },
  "coldStart": {
    "title": "Switch rebooted",
    "hint": "Check power and the cause of the reboot."
  },
  "warmStart": {
    "title": "Switch rebooted",
    "hint": "Check power and the cause of the reboot."
  }
};
