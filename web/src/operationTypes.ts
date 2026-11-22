export type Phase = 'setup' | 'rehearsal' | 'show';
export interface Alert {
  event_id: number;
  switch_id: number;
  switch_name: string;
  if_index: number;
  severity: string;
  code: string;
  title: string;
  ts: string;
  active: boolean;
  acked: boolean;
  cleared_at: string | null;
  acked_at?: string | null;
  port: string;
  mac?: string;
  device_name?: string;
  state: 'active' | 'latched';
  suppressed: boolean;
  suppressed_by: 'phase' | '';
  hint?: string;
  detail?: Record<string, unknown>;
  raw_counters?: Record<string, string>;
}
export interface SwitchGroup {
  name: string;
  switch_ids: number[];
}
