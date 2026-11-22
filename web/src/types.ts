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
export interface Event {
  id: number;
  ts: string;
  switch_id: number;
  if_index: number;
  category: string;
  severity: string;
  code: string;
  title: string;
  detail?: Record<string, unknown>;
  hint?: string;
  acked: boolean;
  switch_name?: string;
}
export interface SwitchSummary {
  id: number;
  name: string;
  ip: string;
  group_tag: string;
  reachable: boolean;
  last_error: string;
  model: string;
  fw: string;
  uptime_sec: number;
  cpu_5s: number;
  mem_free_pct: number;
  mbuf_used_pct: number;
  temp_max_c: number;
  temp_state: string;
  fans_ok: boolean;
  psu_ok: boolean;
  poe_budget_w: number;
  poe_used_w: number;
  ports_up: number;
  ports_total: number;
  rx_bps: number;
  tx_bps: number;
  alerts: {
    critical: number;
    warning: number;
  };
}
export interface Overview {
  switches: SwitchSummary[];
  alerts: {
    critical: number;
    warning: number;
    info: number;
  };
  recent_events: Event[];
}
export interface Profile {
  id: number;
  name: string;
  active: boolean;
}
export interface Settings {
  thresholds: Record<string, number>;
  alert_sound: boolean;
  views: ViewPreset[];
}
export interface ViewPreset {
  id: string;
  name: string;
  vlans: number[];
  tags: string[];
  categories: string[];
}
export interface Status {
  version: string;
  mode: string;
  lab: boolean;
  locked: boolean;
  pin_set: boolean;
  profile: {
    id: number;
    name: string;
  };
  trap: {
    addr: string;
    ok: boolean;
    error: string;
  };
  web_addr: string;
}
export interface Finding {
  severity: string;
  code: string;
  title: string;
  hint: string;
  switch_id: number;
  switch_name: string;
  port?: string;
}
export interface TopologyNode {
  id: string;
  label: string;
  kind: string;
  switch_id?: number;
  reachable?: boolean;
  is_root?: boolean;
  x?: number;
  y?: number;
}
export interface TopologyEdge {
  id: string;
  source: string;
  target: string;
  local_port: string;
  remote_port: string;
  blocking: boolean;
  speed_mbps: number;
  util_pct: number;
}
export interface Topology {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  background: string | null;
}
export interface WatchItem {
  switch_id: number;
  if_index: number;
  label: string;
}
export interface NamedSwitch {
  switch_id: number;
  switch_name: string;
}
export interface Multicast {
  vlans: (VlanIGMP & NamedSwitch)[];
  groups: (McastGroup & NamedSwitch)[];
  mrouters: (Mrouter & NamedSwitch)[];
  gptp: (GPTP & NamedSwitch)[];
  findings: Finding[];
}
export interface DhcpData {
  servers: (NamedSwitch & {
    server_on: boolean;
    pools: string[];
    lease_count: number;
    decline_rx: number;
    snooping_on: boolean;
    invalid_server_msgs: number;
  })[];
  leases: (Lease & NamedSwitch)[];
  conflicts: (NamedSwitch & {
    ip: string;
  })[];
  findings: Finding[];
}
export interface Audit {
  findings: Finding[];
  fw_versions: Record<string, string[]>;
}
export interface MetricResult {
  resolution: string;
  points: Record<string, number | string>[];
}
export interface Timeline {
  from: string;
  to: string;
  markers: {
    ts: string;
    severity: string;
    title: string;
    category: string;
  }[];
}

export interface MVRGroup {
  group: string;
  active: boolean;
  ports: string[];
}

export interface MSRPFailure {
  stream_id: string;
  port: string;
  declaration: string;
  failure_code: number;
}

export interface ModelDefinition {
  model: string;
  rows: string[][];
  note: string;
}

export type ModelDefinitions = Record<string, ModelDefinition>;
export interface ReplayRef {
  id: string;
  ts: string;
}

export interface ReplaySession {
  id: string;
  timeline: Timeline;
  states: SwitchState[];
  ts?: string;
  requestedTs?: string;
}

export interface ReplaySnapshot {
  ts: string;
  switches: SwitchState[];
}

export interface InventoryRow {
  mac: string;
  label: string;
  note: string;
  ip: string;
  ip_stale?: boolean;
  vendor: string;
  lldp_name: string;
  switch_id?: number;
  switch_name: string;
  group_tag: string;
  port: string;
  if_index?: number;
  vlan: number;
  poe_w?: number;
  speed_mbps: number;
  seen: boolean;
  last_seen?: string;
  source: string;
  via_uplink: boolean;
  ping?: PingMark;
}

export type PingStatus = 'ok' | 'timeout' | 'unreachable' | 'error';

export interface PingMark {
  status: PingStatus;
  rtt_ms: number;
  at: string;
}

export interface BulkPingResult {
  ip: string;
  status: PingStatus;
  rtt_ms: number;
  detail?: string;
  source: 'known' | 'manual';
}

export interface BulkPingResponse {
  results: BulkPingResult[];
  ok: number;
  total: number;
  elapsed_ms: number;
  at: string;
}

export type DeviceAliasImportMode = 'merge' | 'replace';

export interface DeviceAliasImportRow {
  line: number;
  mac: string;
  label: string;
  note: string;
  action: 'add' | 'update' | 'same';
  old_label?: string;
}

export interface DeviceAliasImportResponse {
  committed: boolean;
  result: {
    added: number;
    updated: number;
    unchanged: number;
    deleted: number;
  };
  rows: DeviceAliasImportRow[];
  skipped_empty: number;
  errors: { line: number; reason: string }[];
}
