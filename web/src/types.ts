import type { GPTP, Lease, McastGroup, Mrouter, SwitchState, VlanIGMP } from './switchTypes';
export * from './switchTypes';
export * from './operationTypes';
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
  acked_at?: string | null;
  switch_name?: string;
}
export interface SwitchSummary {
  id: number;
  name: string;
  ip: string;
  group_tag: string;
  reachable: boolean;
  last_error: string;
  last_trap_ts?: string | null;
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
  setup_ping_interval_sec: number;
  rehearsal_ping_interval_sec: number;
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
  phase?: import('./operationTypes').Phase;
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
  port_recommendations?: PortRecommendation[];
}
export interface PortRecommendation {
  switch_id: number;
  port: string;
  code: 'EEE_ON_AV_PORT' | 'FLOWCTRL_ON_AV_PORT' | 'NDI4_NEEDS_FLOWCTRL' | 'DSCP_TRUST' |
    'BCAST_STORM_CTRL_UPLINK' | 'MCAST_STORM_CTRL_UPLINK' | 'QSYS_MTU' | 'UPLINK_MTU_MISMATCH' | 'DUPLEX';
  current: string;
  recommended: string;
  source?: string;
  reference?: string;
  lpi?: string;
  version?: string;
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
  portRecommendations?: PortRecommendation[];
  alerts?: import('./operationTypes').Alert[];
  ts?: string;
  requestedTs?: string;
}

export interface ReplaySnapshot {
  ts: string;
  switches: SwitchState[];
  port_recommendations?: PortRecommendation[];
  alerts?: import('./operationTypes').Alert[];
}

export interface InventoryRow {
  registered?: boolean;
  mac: string;
  label: string;
  note: string;
  ip: string;
  ip_stale?: boolean;
  planned_ip?: string;
  ip_mismatch?: boolean;
  ping_ok?: boolean | null;
  ping_rtt_ms?: number | null;
  ping_ts?: string | null;
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
