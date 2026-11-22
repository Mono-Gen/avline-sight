import { asArray } from './format';
import type { Alert, InventoryRow, Phase, Port, SwitchSummary, Topology } from './types';

export type LampTone = 'critical' | 'warning' | 'healthy' | 'unused' | 'suppressed' | 'offline';
export const isPortAlert = (alert: Alert) => alert.if_index > 0 && !['FAN_FAIL', 'PSU_FAIL'].includes(alert.code);

export function criticalLink(alert: Alert, devices: InventoryRow[]) {
  return isPortAlert(alert) && alert.code === 'LINK_DOWN' &&
    devices.some(device => device.if_index === alert.if_index && device.registered && !device.via_uplink);
}

export function lampState(port: Port, alerts: Alert[], devices: InventoryRow[], suppressed: boolean, reachable: boolean, uplink = false) {
  const relevant = asArray(alerts).filter(isPortAlert);
  const visible = relevant.filter(alert => !alert.suppressed);
  let tone: LampTone = 'unused';
  if (suppressed || port.excluded || relevant.some(alert => alert.suppressed) && !visible.length) tone = 'suppressed';
  else if (!reachable && !visible.length) tone = 'offline';
  else if (visible.some(alert => alert.severity === 'CRITICAL' || criticalLink(alert, devices)) || port.sfp?.los || port.sfp?.tx_fault ||
    Boolean(port.poe?.fault && port.poe.fault.toLowerCase() !== 'none')) tone = 'critical';
  else if (visible.some(alert => alert.severity === 'WARNING') || devices.some(device => device.ip_mismatch || device.ping_ok === false ||
    device.ping && device.ping.status !== 'ok') || [port.err_delta, port.discard_delta, port.fcs_delta, port.cos_drop_delta].some(value => value > 0)) tone = 'warning';
  else if (!reachable) tone = 'offline';
  else if (port.oper === 'up' && (uplink || asArray(port.devices).length || devices.some(device => device.seen))) tone = 'healthy';
  const symbols: Record<LampTone, string> = { critical: '✖', warning: '▲', healthy: '●', unused: '·', suppressed: '🔧', offline: '⚠' };
  return { tone, symbol: symbols[tone], unacked: tone !== 'suppressed' && visible.some(alert => !alert.acked) };
}

export function sortedAlerts(alerts: Alert[]): Alert[] {
  const severity = (value: string) => ({ CRITICAL: 0, WARNING: 1, INFO: 2 }[value] ?? 3);
  return asArray(alerts).slice().sort((left, right) => Number(left.suppressed) - Number(right.suppressed) ||
    severity(left.severity) - severity(right.severity) || Date.parse(right.ts) - Date.parse(left.ts) || right.event_id - left.event_id);
}

export function phaseSuppressesPorts(phase: Phase) { return phase === 'setup' || phase === 'teardown'; }

export function uplinkPorts(topology: Topology | undefined, switches: SwitchSummary[]): Set<string> {
  const ids = new Set(asArray(topology?.nodes).filter(node => node.kind === 'switch' &&
    switches.some(sw => sw.id === node.switch_id)).map(node => node.id));
  const nodes = new Map(asArray(topology?.nodes).map(node => [node.id, node.switch_id]));
  const result = new Set<string>();
  for (const edge of asArray(topology?.edges)) {
    if (ids.has(edge.source) && ids.has(edge.target)) {
      result.add(`${nodes.get(edge.source)}/${edge.local_port}`);
      result.add(`${nodes.get(edge.target)}/${edge.remote_port}`);
    }
  }
  return result;
}
