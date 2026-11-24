import type { EventCopy, eventJa } from './eventJa';

export const eventEn: Record<keyof typeof eventJa, EventCopy> = {
  "CONFIG_AUTO_IMPORTED": {
    "title": "Project file imported automatically",
    "hint": ""
  },
  "DEVICE_MISSING": {
    "title": "Registered device is missing: {name}",
    "hint": "Check that the device is powered on. Check and firmly re-seat the Ethernet cable.",
    "details": "Registered MAC ({mac}) previously observed, now missing across consecutive polls",
    "fallbackTitle": "Registered device missing"
  },
  "PING_FAIL": {
    "title": "No Ping response: {name} ({last_ip})",
    "hint": "Check the device power and Ethernet cable connection. If the device works normally, check its ICMP response setting and network path.",
    "details": "No Ping (ICMP) response from {last_ip}",
    "fallbackTitle": "No Ping response"
  },
  "IP_MISMATCH": {
    "title": "IP address does not match planned IP: {name} ({last_ip} ≠ planned {planned_ip})",
    "hint": "Check that the device's IP address matches the plan sheet. Verify it is plugged into the intended port and VLAN.",
    "details": "Observed IP {last_ip} differs from planned IP {planned_ip}",
    "fallbackTitle": "IP address does not match the planned IP"
  },
  "LINK_DOWN": {
    "title": "Port link is down",
    "hint": "Check both ends of the cable and re-seat firmly. Verify the connected device is powered on.",
    "details": "Port link down detected"
  },
  "LINK_UP": {
    "title": "Port link is back up",
    "hint": "Verify audio, video, or control on the connected device has recovered. Swap to a spare cable if drops repeat.",
    "details": "Port link up detected"
  },
  "LINK_FLAP": {
    "title": "Link flap detected ({count} times)",
    "hint": "Check for loose or broken latches and pinched cables. Swap to a spare cable and check device power.",
    "details": "Link flap detected from linkDown → linkUp SNMP Traps or a change in ifLastChange.",
    "fallbackTitle": "Link flap detected"
  },
  "SWITCH_UNREACHABLE": {
    "title": "Switch unreachable via SNMP",
    "hint": "Check the switch power LED and power cord. Check the uplink cable, management network connectivity, and SNMP settings.",
    "details": "Switch unreachable / no response to SNMP polling"
  },
  "SWITCH_RECOVERED": {
    "title": "Switch SNMP connectivity restored",
    "hint": "Verify connected devices on this switch are operating normally. Check for loose power or uplink cables and review event history.",
    "details": "Switch SNMP reachability recovered"
  },
  "SWITCH_REBOOT": {
    "title": "Switch rebooted",
    "hint": "Check for a loose power cord, brief power loss, or accidental reboot. Verify connected devices have recovered normally.",
    "details": "sysUpTime decreased. Possible causes include a power interruption, a crash, or a manual restart."
  },
  "HIGH_UTIL": {
    "title": "Port bandwidth utilization exceeded threshold",
    "hint": "Check for unintended audio or video streams. Check bandwidth allocation and QoS.",
    "details": "Port Rx/Tx bandwidth utilization exceeded threshold"
  },
  "DISCARDS": {
    "title": "Packet drops (Discards) are increasing (+{delta})",
    "hint": "Check for audio or video dropouts. If drops continue, check Storm Control and QoS.",
    "details": "Packet discards increased (+{delta}); possible congestion, Storm Control, or QoS queue overflow",
    "fallbackTitle": "Packet drops (Discards) are increasing"
  },
  "SPEED_BELOW_EXPECTED": {
    "title": "Link speed is below expected speed",
    "hint": "Re-seat the cable; if unchanged, swap to a spare cable. Check couplers, cable category, and auto-negotiation settings.",
    "details": "Link speed below expected speed; possible damaged pair (4-wire fallback), bad termination, or auto-negotiation mismatch"
  },
  "RELINK_SLOWER": {
    "title": "Link speed dropped after relink",
    "hint": "Inspect the connectors for damage or looseness and swap to a spare cable. Check whether the connected device port or auto-negotiation setting changed.",
    "details": "Link speed lower after relink; possible cable pair fault, connector issue, or auto-negotiation mismatch"
  },
  "SPEED_CHANGE": {
    "title": "Link speed changed",
    "hint": "Verify audio, video, or control on the connected device is unaffected. If unexpected, check expected speed and cabling, and replace the cable if needed.",
    "details": "Port link speed change detected"
  },
  "POE_PORT_FAULT": {
    "title": "PoE fault on this port",
    "hint": "Unplug the cable and inspect for damage, moisture, pinching, or shorts. Try a spare cable; if it persists, check the powered device and PoE budget.",
    "details": "PoE port fault detected; possible PoE power shortage, PD fault, or cable short"
  },
  "POE_FAULT": {
    "title": "PoE fault detected",
    "hint": "Check the PoE powered device and Ethernet cable. Check the PoE budget.",
    "details": "PoE fault detected; possible PoE budget shortage, PD fault, or cable short"
  },
  "POE_STATE": {
    "title": "PoE power state changed",
    "hint": "Check the power LED and operation of the connected device. If power was lost, re-seat the cable and check port PoE settings.",
    "details": "Port PoE power state changed"
  },
  "PHY_ERRORS": {
    "title": "Physical layer errors are increasing (FCS+{fcs} / Symbol+{symbol} / Align+{align})",
    "hint": "Re-seat the cable and swap to a spare if possible. For SFP fiber links, clean the connector endfaces and check for sharp bends.",
    "details": "Physical layer errors increased (FCS+{fcs} / Symbol+{symbol} / Align+{align}); healthy baseline is zero",
    "fallbackTitle": "Physical errors are increasing"
  },
  "MTU_MISMATCH": {
    "title": "Possible MTU mismatch (oversized frames detected)",
    "hint": "Compare MTU settings on the connected device and peer switch.",
    "details": "FrameTooLongs counter increased; possible MTU mismatch with connected device or peer switch"
  },
  "DUPLEX": {
    "title": "Half-Duplex or Late Collision detected",
    "hint": "Reseat the Ethernet cable. If the issue persists, check Duplex and auto-negotiation at both ends.",
    "details": "Half-Duplex operation or Late Collision increase; possible fixed vs. auto-negotiation mismatch"
  },
  "COS_DROP": {
    "title": "CoS queue drops are increasing",
    "hint": "Check for audio or video dropouts. Check bandwidth utilization and QoS priorities.",
    "details": "CoS queue drops increased; check bandwidth saturation, microbursts, and QoS priorities"
  },
  "PAUSE_FRAMES": {
    "title": "PAUSE frames are increasing",
    "hint": "Check for audio or video delays and dropouts. Check Flow Control on this port.",
    "details": "PAUSE frame counter increased; active Flow Control may introduce stream latency"
  },
  "SFP_LOS": {
    "title": "SFP Loss of Signal (LOS) or Transmit Fault (TxFault) detected",
    "hint": "Check the fiber cable for disconnection, sharp bends, or breaks. Clean the endface and re-seat; replace the SFP module if needed.",
    "details": "SFP Loss of Signal (LOS) or Transmit Fault (TxFault) detected"
  },
  "SFP_LOW_RX": {
    "title": "SFP optical RX power is low ({rx_dbm} dBm)",
    "hint": "Check the fiber cable for tight bends or pressure. Clean the endface and re-seat when safe; replace the SFP or fiber if unchanged.",
    "details": "SFP optical RX power fell below threshold ({rx_dbm} dBm)",
    "fallbackTitle": "SFP receive optical power is low"
  },
  "SFP_RX_DROP": {
    "title": "SFP optical RX power dropped from baseline",
    "hint": "Check if the fiber was newly bent, pulled, or stepped on. Inspect for loose or dirty connectors, and clean/re-seat when safe.",
    "details": "SFP optical RX power dropped from recorded baseline beyond threshold"
  },
  "MSRP_RESV_FAILED": {
    "title": "AVB / Milan MSRP bandwidth reservation failed",
    "hint": "Check AVB support along the path. Check available bandwidth and VLAN settings.",
    "details": "AVB / Milan MSRP bandwidth reservation failed; check bandwidth, AVB support, and VLAN match"
  },
  "MVR_RX_FAIL": {
    "title": "MVR IGMP receive failures are increasing",
    "hint": "Check for stream dropouts on receivers. Check the MVR ports and VLAN settings.",
    "details": "MVR IGMP receive failure counter increased; check source/receiver ports and MVR VLAN config"
  },
  "CPU_HIGH": {
    "title": "Switch CPU utilization exceeded threshold",
    "hint": "Check for physical loops or unusual traffic.",
    "details": "Switch 5-second CPU utilization exceeded threshold"
  },
  "MEM_LOW": {
    "title": "Switch free memory is low",
    "hint": "Check switch and device responsiveness. Monitor available memory.",
    "details": "Switch free memory percentage fell below threshold"
  },
  "TEMP_HIGH": {
    "title": "Switch temperature is abnormal",
    "hint": "Check that rack intake/exhaust vents are not blocked. Improve airflow (open rack doors, add fans) and lower ambient temperature.",
    "details": "Switch internal temperature exceeded threshold or thermal status is warning/critical/shutdown"
  },
  "FAN_FAIL": {
    "title": "Switch fan failure",
    "hint": "Check if the switch fan is spinning or obstructed. Watch switch temperature closely and prepare external airflow or a spare switch.",
    "details": "Switch fan fault state (failed / nopower / notpowering / incompatible)"
  },
  "PSU_FAIL": {
    "title": "Switch PSU (power supply unit) failure",
    "hint": "Check that the power cord is fully seated and the outlet has power. On dual-PSU switches, inspect the faulted unit and prepare a spare.",
    "details": "Switch PSU fault state (failed / nopower / notpowering / incompatible)"
  },
  "MBUF_ALLOC_FAIL": {
    "title": "Mbuf (internal packet buffer) allocation failures detected",
    "hint": "Check for audio or video dropouts. Check bandwidth utilization and QoS settings.",
    "details": "Mbuf allocation failures increased; check congestion, microbursts, and QoS"
  },
  "POE_BUDGET": {
    "title": "PoE budget usage is approaching limit",
    "hint": "Do not plug additional PoE devices into this switch. Move some devices to local power adapters or another switch if possible.",
    "details": "Switch total PoE budget utilization exceeded threshold"
  },
  "UDLD": {
    "title": "Port shut down by UDLD (unidirectional link fault)",
    "hint": "Check both strands of the fiber cable for loose connectors or sharp bends. Clean and re-seat, or swap to a spare fiber cable / SFP.",
    "details": "Port shut down by UDLD unidirectional link fault; possible single-strand break or Tx/Rx fault"
  },
  "STP_BLOCKING": {
    "title": "Port is blocked by STP Blocking",
    "hint": "Check for cabling loops. Verify whether the path is intentionally redundant before unplugging.",
    "details": "STP state is discarding (Blocking) while link is up; physical loop or redundant path blocked"
  },
  "STP_TCN_BURST": {
    "title": "STP Topology Change Notifications (TCN) are surging",
    "hint": "Check for link flaps and cabling loops.",
    "details": "STP Topology Change Notification (TCN) burst detected; possible physical loop or flapping link"
  },
  "DHCP_ROGUE": {
    "title": "Invalid / rogue DHCP server messages are increasing",
    "hint": "Check for unintended routers or DHCP servers.",
    "details": "Invalid/rogue DHCP server messages increased; check unauthorized routers or duplicate DHCP"
  },
  "IP_CONFLICT": {
    "title": "IP address conflict detected",
    "hint": "Check recently added or changed devices. Check for overlapping static IP and DHCP assignments.",
    "details": "IP address conflict detected between static IP and DHCP assignments"
  },
  "AUDIT_PENDING": {
    "title": "Loading initial configuration from all switches",
    "hint": "Wait for the first slow polling cycle. If this persists, check SNMP connectivity.",
    "details": "Cross-switch audit pending first slow polling cycle completion on all switches"
  },
  "DHCP_DOUBLE": {
    "title": "DHCP server is enabled on multiple switches",
    "hint": "Check DHCP server settings and address pools for each VLAN.",
    "details": "DHCP server enabled on multiple switches (per-VLAN separation is not evaluated)"
  },
  "SNOOP_MISMATCH": {
    "title": "IGMP Snooping ON/OFF does not match across switches in the same VLAN",
    "hint": "Compare IGMP Snooping settings across switches in the same VLAN.",
    "details": "IGMP Snooping enabled on some switches and disabled on others in the same VLAN"
  },
  "QUERIER_ABSENT": {
    "title": "No IGMP Querier active on IGMP Snooping-enabled VLAN",
    "hint": "Check IGMP Querier settings on the core switch promptly.",
    "details": "No IGMP Querier active on IGMP Snooping-enabled VLAN (risk of multicast drop after 1–5 min)"
  },
  "QUERIER_CONFLICT": {
    "title": "Multiple IGMP Querier IPs observed in the same VLAN",
    "hint": "Check IGMP query sources and IGMP Querier election.",
    "details": "Multiple IGMP Querier IPs observed in the same VLAN"
  },
  "IGMP_VERSION_MIX": {
    "title": "Multiple IGMP versions are mixed in the same VLAN",
    "hint": "Check multicast reception and the IGMP versions required by AV devices.",
    "details": "Multiple IGMP versions observed in the same VLAN"
  },
  "UPLINK_MTU_MISMATCH": {
    "title": "Uplink ends have different MTU settings.",
    "hint": "",
    "details": "Matching MTU at both LLDP-adjacent ends may be recommended for large frames."
  },
  "QUERIER_CHANGED": {
    "title": "IGMP Querier IP changed",
    "hint": "Check multicast streams and IGMP query sources.",
    "details": "IGMP Querier IP changed on VLAN; check election changes or external query sources"
  },
  "MCAST_FLOOD": {
    "title": "Possible multicast flooding to unjoined ports",
    "hint": "Check IGMP Snooping and IGMP Querier status.",
    "details": "Over 20,000 pps multicast sent to majority (>=2) of unjoined non-uplink ports; suspected flooding"
  },
  "DEVICE_NEW": {
    "title": "New device (MAC address) detected",
    "hint": "Verify the expected device is connected to the intended port. If unrecognized, inspect the physical port on site.",
    "details": "New MAC address / device detected in FDB"
  },
  "DEVICE_GONE": {
    "title": "Device disappeared from FDB (MAC address table)",
    "hint": "Check if the device was powered off or unplugged. No action needed if it was removed intentionally.",
    "details": "Device MAC no longer observed in FDB (power off, unplug, or FDB aging)"
  },
  "PHASE_CHANGED": {
    "title": "Operating phase changed: {phase}",
    "hint": "Confirm the phase matches the current stage (Setup / Rehearsal / Live). Switch it back in the header if changed by mistake.",
    "details": "Operating phase changed to {phase}; phase-based periodic Ping and alert suppression updated",
    "fallbackTitle": "Operating phase changed"
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
    "title": "Port link is down (SNMP Trap)",
    "hint": "Check both ends of the cable and re-seat firmly. Verify the connected device is powered on.",
    "details": "Port link down reported via SNMP Trap"
  },
  "linkUp": {
    "title": "Port link is back up (SNMP Trap)",
    "hint": "Verify audio, video, or control on the connected device has recovered. Swap to a spare cable if drops repeat.",
    "details": "Port link up reported via SNMP Trap"
  },
  "coldStart": {
    "title": "Switch rebooted from power cycle (SNMP Trap)",
    "hint": "Check for a loose power cord or brief power outage. Verify connected devices have recovered.",
    "details": "SNMP Trap (coldStart) received; power-cycle / cold boot"
  },
  "warmStart": {
    "title": "Switch rebooted (SNMP Trap)",
    "hint": "Check the restart and configuration history. Verify connected devices are operating normally.",
    "details": "SNMP Trap (warmStart) received; software / warm restart"
  },
  "CLOCK_SKEW": {
    "title": "Clock offset from monitoring PC: {offset_seconds} s",
    "hint": "Check time synchronization on the switch and monitoring PC.",
    "details": "The switch time sampled during slow polling was compared with the monitoring PC clock.",
    "fallbackTitle": "Clock offset from monitoring PC detected"
  },
  "PORT_CONFIG_CHANGED": {
    "title": "{port} {field} changed",
    "hint": "Verify that the configuration change was intended.",
    "details": "Compare the previous and current settings.",
    "fallbackTitle": "Port configuration changed"
  },
  "PRE_SHOW_CHECKLIST_REMAINDER": {
    "title": "Pre-show checks remained at show start ({count})",
    "hint": "Review the remaining pre-show checks.",
    "details": "This event records the checklist state when the show started.",
    "fallbackTitle": "Pre-show checks remained at show start"
  },
  "BASELINE_SAVED": {
    "title": "Baseline saved: {name}",
    "hint": "",
    "details": "The saved state can be used to compare configuration differences.",
    "fallbackTitle": "Baseline saved"
  },
  "LAB_FAULT": {
    "title": "Simulated fault triggered in the lab",
    "hint": "",
    "details": "Target {target} / Scenario {scenario}"
  },
  "TERMINAL_OPEN": {
    "title": "Switch terminal opened",
    "hint": "",
    "details": "Connection protocol: {protocol}"
  },
  "MARKER": {
    "title": "Marker: {note}",
    "hint": "",
    "details": "",
    "fallbackTitle": "Marker"
  },
  "SWITCH_LOG": {
    "title": "Switch log received",
    "hint": "",
    "details": ""
  },
  "UNKNOWN_TRAP": {
    "title": "Unrecognized SNMP Trap received",
    "hint": "Check the source of the Trap.",
    "details": "The notification OID is unregistered. OID: {trap_oid}"
  },
  "authenticationFailure": {
    "title": "SNMP authentication failed",
    "hint": "Check SNMP settings.",
    "details": "An SNMP Trap was received."
  },
  "lldpRemTablesChange": {
    "title": "LLDP neighbors changed",
    "hint": "Check the connected devices and cabling.",
    "details": "An SNMP Trap was received."
  },
  "pethPsePortOnOffNotification": {
    "title": "PoE power state changed",
    "hint": "Check power on the connected device.",
    "details": "An SNMP Trap was received."
  },
  "pethMainPowerUsageOnNotification": {
    "title": "PoE usage exceeded the threshold",
    "hint": "Check the PoE budget and connected devices.",
    "details": "An SNMP Trap was received."
  },
  "pethMainPowerUsageOffNotification": {
    "title": "PoE usage fell below the threshold",
    "hint": "Check the remaining PoE budget.",
    "details": "An SNMP Trap was received."
  },
  "boxsFanStateChange": {
    "title": "Fan status changed",
    "hint": "Check the fans.",
    "details": "An SNMP Trap was received."
  },
  "boxsPowSupplyStateChange": {
    "title": "PSU status changed",
    "hint": "Check the power cable and PSU.",
    "details": "An SNMP Trap was received."
  },
  "boxsTemperatureChange": {
    "title": "Switch temperature changed",
    "hint": "Check the temperature and airflow.",
    "details": "An SNMP Trap was received."
  },
  "boxsThermalShutdown": {
    "title": "Switch shut down due to high temperature",
    "hint": "Check the vents and ambient temperature.",
    "details": "An SNMP Trap was received."
  },
  "boxsTemperatureStateChange": {
    "title": "Switch temperature status changed",
    "hint": "Check the temperature and airflow.",
    "details": "An SNMP Trap was received."
  },
  "boxsDyingGaspEventChange": {
    "title": "Switch reported a loss of power",
    "hint": "Check the power cable and outlet.",
    "details": "An SNMP Trap was received."
  },
  "broadcastStormStartTrap": {
    "title": "Broadcast storm started",
    "hint": "Check for cabling loops and identify the source.",
    "details": "An SNMP Trap was received."
  },
  "broadcastStormEndTrap": {
    "title": "Broadcast storm ended",
    "hint": "Verify that the switch and connected devices are operating normally.",
    "details": "An SNMP Trap was received."
  },
  "linkFailureTrap": {
    "title": "Link fault detected",
    "hint": "Check the cable and the device at the other end.",
    "details": "An SNMP Trap was received."
  },
  "fanFailureTrap": {
    "title": "Fan fault detected",
    "hint": "Check the fans and switch temperature.",
    "details": "An SNMP Trap was received."
  },
  "stpInstanceNewRootTrap": {
    "title": "STP Root Bridge changed",
    "hint": "Verify that the expected switch is the STP Root Bridge.",
    "details": "An SNMP Trap was received."
  },
  "stpInstanceTopologyChangeTrap": {
    "title": "STP topology changed",
    "hint": "Check for link flaps or cabling loops.",
    "details": "An SNMP Trap was received."
  },
  "powerSupplyStatusChangeTrap": {
    "title": "PSU status changed",
    "hint": "Check the power cable and PSU.",
    "details": "An SNMP Trap was received."
  },
  "failedUserLoginTrap": {
    "title": "Switch login failed",
    "hint": "Check the login attempt and authentication settings.",
    "details": "An SNMP Trap was received."
  },
  "daiIntfErrorDisabledTrap": {
    "title": "DAI disabled a port",
    "hint": "Check the connected device and DAI settings.",
    "details": "An SNMP Trap was received."
  },
  "stpInstanceLoopInconsistentStartTrap": {
    "title": "STP detected a loop inconsistency",
    "hint": "Check the cabling and STP settings.",
    "details": "An SNMP Trap was received."
  },
  "stpInstanceLoopInconsistentEndTrap": {
    "title": "STP loop inconsistency cleared",
    "hint": "Verify connectivity on the affected port.",
    "details": "An SNMP Trap was received."
  },
  "dhcpSnoopingIntfErrorDisabledTrap": {
    "title": "DHCP Snooping disabled a port",
    "hint": "Check DHCP Snooping settings and the source of DHCP messages.",
    "details": "An SNMP Trap was received."
  },
  "agentSwitchIpAddressConflictTrap": {
    "title": "IP address conflict detected",
    "hint": "Check for overlapping static IP and DHCP assignments.",
    "details": "An SNMP Trap was received."
  },
  "agentSwitchCpuRisingThresholdTrap": {
    "title": "CPU usage exceeded the threshold",
    "hint": "Check for physical loops or unusual traffic.",
    "details": "An SNMP Trap was received."
  },
  "agentSwitchCpuFallingThresholdTrap": {
    "title": "CPU usage recovered",
    "hint": "Verify that the switch is responding.",
    "details": "An SNMP Trap was received."
  },
  "agentSwitchCpuFreeMemBelowThresholdTrap": {
    "title": "Free memory fell below the threshold",
    "hint": "Check switch responsiveness and available memory.",
    "details": "An SNMP Trap was received."
  },
  "agentSwitchMbufRisingThresholdTrap": {
    "title": "Mbuf usage exceeded the threshold",
    "hint": "Check bandwidth utilization and QoS settings.",
    "details": "An SNMP Trap was received."
  },
  "agentSwitchMbufFallingThresholdTrap": {
    "title": "Mbuf usage recovered",
    "hint": "Verify audio and video reception.",
    "details": "An SNMP Trap was received."
  },
  "EEE_ON_AV_PORT": {
    "title": "EEE (Energy Efficient Ethernet / 802.3az) is enabled on this port (LPI transitions: +{lpi}).",
    "hint": "",
    "details": "Disabling EEE is commonly recommended for sync-sensitive AV protocols such as Dante/PTP.",
    "fallbackTitle": "EEE (Energy Efficient Ethernet / 802.3az) is enabled on this port."
  },
  "FLOWCTRL_ON_AV_PORT": {
    "title": "Flow Control (PAUSE) is enabled on this port (setting: {pause}).",
    "hint": "",
    "details": "Disabling Flow Control is commonly recommended for AV ports other than NDI 4.",
    "fallbackTitle": "Flow Control (PAUSE) is enabled on this port."
  },
  "NDI4_NEEDS_FLOWCTRL": {
    "title": "Flow Control is disabled on a port assigned role ndi4.",
    "hint": "",
    "details": "Enabling Flow Control may be recommended/required for NDI 4 deployments."
  },
  "DSCP_TRUST": {
    "title": "Port QoS Trust is set to {trust} (not trustIpDscp).",
    "hint": "",
    "details": "Trust DSCP is commonly recommended when using DSCP prioritization for Dante/AES67/PTP.",
    "fallbackTitle": "Port QoS Trust is set to a value other than trustIpDscp."
  },
  "QSYS_MTU": {
    "title": "Max frame size is {max_frame} (>1522) on a port assigned role qsys.",
    "hint": "",
    "details": "Q-SYS deployments typically specify MTU 1500 / max frame 1522.",
    "fallbackTitle": "Max frame size exceeds 1522 on a port assigned role qsys."
  },
  "STORM_CTRL_UPLINK": {
    "title": "Storm Control (multicast/broadcast) is enabled on this uplink port.",
    "hint": "",
    "details": "Disabling it on uplinks may be recommended to avoid dropping high-bandwidth multicast or gigaACE/CobraNet traffic."
  },
  "STP_MSTP": {
    "title": "STP is running in MSTP mode on this switch.",
    "hint": "",
    "details": "RSTP is commonly recommended for ProAV deployments."
  },
  "FW_MISMATCH": {
    "title": "Monitored switches are running different firmware versions ({versions}).",
    "hint": "",
    "details": "Aligning firmware versions across switches is recommended.",
    "fallbackTitle": "Monitored switches are running different firmware versions."
  },
  "STP_PRIORITY_DEFAULT": {
    "title": "All monitored switches use the default STP priority 32768.",
    "hint": "",
    "details": "Setting explicit STP priorities (e.g., core=0, backup core=8192) is commonly recommended to control Root Bridge election."
  },
  "STP_EDGE_ROOT": {
    "title": "A switch with <=1 detected uplink is elected as the STP Root Bridge.",
    "hint": "",
    "details": "Designing STP priorities so the core switch becomes Root Bridge is commonly recommended."
  },
  "QUERIER_ON_EDGE": {
    "title": "A non-root switch with <=1 detected uplink is elected as the IGMP Querier on VLAN {vlan}.",
    "hint": "",
    "details": "Placing the IGMP Querier on the core switch is commonly recommended.",
    "fallbackTitle": "A non-root switch with at most one uplink is elected as the IGMP Querier."
  },
  "DAISY_CHAIN": {
    "title": "Switch chain depth from the STP Root Bridge is {depth} hops (>=3).",
    "hint": "",
    "details": "Star topology from the core is commonly recommended for SMPTE/AES67/PTPv2.",
    "fallbackTitle": "Switch chain depth from the STP Root Bridge is at least three hops."
  },
  "VLAN_CROWDED": {
    "title": "VLAN {vlan} has {count} non-uplink devices (>200).",
    "hint": "",
    "details": "Keeping a VLAN at or below 200 devices is recommended for systems such as Q-SYS.",
    "fallbackTitle": "A VLAN has more than 200 non-uplink devices."
  },
  "BCAST_STORM_CTRL_UPLINK": {
    "title": "Broadcast Storm Control is enabled on the uplink.",
    "hint": "",
    "details": "It may drop high-bandwidth broadcast traffic."
  },
  "MCAST_STORM_CTRL_UPLINK": {
    "title": "Multicast Storm Control is enabled on the uplink.",
    "hint": "",
    "details": "It may drop high-bandwidth multicast traffic."
  }
};
