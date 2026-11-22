import type { MSRPFailure, MVRGroup } from './types';

export interface SwitchConfig {
  id: number;
  profile_id: number;
  name: string;
  group_tag: string;
  ip: string;
  snmp_port: number;
  community?: string;
  enabled: boolean;
  web_url?: string;
  last_trap_ts?: string | null;
}
export interface SwitchState {
  id: number;
  name: string;
  ip: string;
  group_tag: string;
  reachable: boolean;
  last_error?: string;
  last_poll: string;
  sys_name: string;
  sys_descr: string;
  sys_object_id: string;
  model: string;
  fw: string;
  serial: string;
  uptime_sec: number;
  clock_offset_seconds?: number;
  base_mac: string;
  ports: Port[];
  health: Health;
  igmp: IGMP;
  dhcp: DHCP;
  config: SwitchConf;
  neighbors: (Neighbor)[];
  caps: Record<string, string>;
  alert_counts: Record<string, number>;
}
export interface Port {
  if_index: number;
  name: string;
  alias: string;
  memo?: string;
  type: string;
  oper: string;
  speed_mbps: number;
  duplex: string;
  pvid: number;
  rx_bps: number;
  tx_bps: number;
  util_rx: number;
  util_tx: number;
  rx_mc_pps: number;
  tx_mc_pps: number;
  rx_bc_pps: number;
  err_delta: number;
  discard_delta: number;
  fcs_delta: number;
  symbol_delta: number;
  align_delta: number;
  too_long_delta: number;
  late_col_delta: number;
  pause_delta: number;
  lpi_delta: number;
  cos_drop_delta: number;
  err_since_reset: number;
  discard_since_reset: number;
  poe?: PoE | null;
  sfp?: SFP | null;
  eee: string;
  pause: string;
  max_frame: number;
  udld: string;
  stp_state: string;
  trust: string;
  mcast_storm_ctl: boolean;
  bcast_storm_ctl: boolean;
  excluded: boolean;
  av_role?: string;
  expected_speed_mbps?: number;
  devices: (Device)[];
  cable?: Cable | null;
  last_change: string;
}
export interface PoE {
  detect: string;
  class: number;
  power_w: number;
  current_ma: number;
  volts: number;
  fault: string;
  denied_cnt: number;
  overload_cnt: number;
  short_cnt: number;
}
export interface SFP {
  rx_dbm: number;
  tx_dbm: number;
  temp_c: number;
  volts: number;
  bias_ma: number;
  los: boolean;
  tx_fault: boolean;
  vendor: string;
  part_no: string;
  supported: string;
}
export interface Cable {
  status: string;
  min_len_m: number;
  max_len_m: number;
  fail_loc_m: number;
}
export interface Device {
  mac: string;
  vendor?: string;
  ip?: string;
  vlan: number;
  lldp_name?: string;
  lldp_desc?: string;
  lldp_port?: string;
  label?: string;
  source: string;
  link_local?: boolean;
}
export interface Neighbor {
  local_if_index: number;
  local_port: string;
  chassis_id: string;
  sys_name: string;
  sys_desc: string;
  port_id: string;
  mgmt_ip: string;
  is_netgear: boolean;
}
export interface Health {
  cpu_5s: number;
  cpu_60s: number;
  cpu_300s: number;
  mem_free_pct: number;
  mbuf_used_pct: number;
  mbuf_alloc_fails: number;
  mbuf_fail_delta: number;
  temp_max_c: number;
  temp_state: string;
  temps: (number)[];
  fans: (Item)[];
  psus: (Item)[];
  poe_budget_w: number;
  poe_used_w: number;
  processes: (Process)[];
}
export interface Item {
  index: number;
  state: string;
  speed?: string;
  duty?: string;
}
export interface Process {
  name: string;
  pct: number;
  raw: string;
}
export interface IGMP {
  snooping_on: boolean;
  vlans: (VlanIGMP)[];
  groups: (McastGroup)[];
  mrouters: (Mrouter)[];
  mvr: boolean;
  gptp?: GPTP | null;
  msrp_streams: number;
  msrp_reservations: number;
  msrp_failed: MSRPFailure[];
  mvr_groups: MVRGroup[];
  mvr_rx_failures: number;
  mvr_rx_fail_delta: number;
}
export interface VlanIGMP {
  vlan: number;
  name: string;
  snooping: boolean;
  fast_leave: boolean;
  querier_admin: boolean;
  querier_oper: string;
  querier_addr: string;
  last_querier: string;
  last_version: number;
  oper_version: number;
}
export interface McastGroup {
  vlan: number;
  group: string;
  ports: (string)[];
  reporter?: string;
  protocol: string;
  source: string;
}
export interface Mrouter {
  port: string;
  vlan: number;
}
export interface GPTP {
  clock_id: string;
  grandmaster: string;
  steps_removed: number;
}
export interface DHCP {
  server_on: boolean;
  pools: (string)[];
  leases: (Lease)[];
  conflicts: (string)[];
  snooping_on: boolean;
  invalid_server_msgs: number;
  invalid_delta: number;
  decline_rx: number;
}
export interface Lease {
  ip: string;
  mac: string;
  pool: string;
  port?: string;
}
export interface SwitchConf {
  stp_version: string;
  stp_priority: number;
  bridge_id: string;
  root_id: string;
  is_root: boolean;
  topo_changes: number;
  bonjour: boolean;
  bcast_storm_global: boolean;
  mcast_storm_global: boolean;
  https: boolean;
  https_port: number;
  ssh: boolean;
  ssh_port: number;
  telnet_port: number;
}
