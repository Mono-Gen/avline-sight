import { asArray } from '../format';
import type { SwitchConfig, SwitchState, SwitchSummary } from '../types';

export function replaySummary(state: SwitchState): SwitchSummary {
  const ports = asArray(state.ports);
  const health = state.health;
  return {
    id: state.id,
    name: state.name,
    ip: state.ip,
    group_tag: state.group_tag,
    reachable: state.reachable,
    last_error: state.last_error || '',
    model: state.model,
    fw: state.fw,
    uptime_sec: state.uptime_sec,
    cpu_5s: health.cpu_5s,
    mem_free_pct: health.mem_free_pct,
    mbuf_used_pct: health.mbuf_used_pct,
    temp_max_c: health.temp_max_c,
    temp_state: health.temp_state,
    fans_ok: asArray(health.fans).every(item => item.state === 'operational' || item.state === 'normal'),
    psu_ok: asArray(health.psus).every(item => item.state === 'operational' || item.state === 'normal'),
    poe_budget_w: health.poe_budget_w,
    poe_used_w: health.poe_used_w,
    ports_up: asArray(ports).filter(port => port.oper === 'up').length,
    ports_total: ports.length,
    rx_bps: asArray(ports).reduce((sum, port) => sum + port.rx_bps, 0),
    tx_bps: asArray(ports).reduce((sum, port) => sum + port.tx_bps, 0),
    alerts: {
      critical: state.alert_counts?.CRITICAL || 0,
      warning: state.alert_counts?.WARNING || 0
    },
  };
}


export function registeredSummary(config: SwitchConfig): SwitchSummary {
  return {
    id: config.id, name: config.name, ip: config.ip, group_tag: config.group_tag,
    reachable: false, fw: '', model: '', last_error: '', last_trap_ts: config.last_trap_ts,
    uptime_sec: 0, cpu_5s: 0, mem_free_pct: 0, mbuf_used_pct: 0, temp_max_c: 0,
    temp_state: '', fans_ok: true, psu_ok: true, poe_budget_w: 0, poe_used_w: 0,
    ports_up: 0, ports_total: 0, rx_bps: 0, tx_bps: 0, alerts: { critical: 0, warning: 0 },
  };
}

