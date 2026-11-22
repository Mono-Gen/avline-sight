import { asArray } from './format';
import type { Alert, Port, SwitchSummary, Topology } from './types';
import { alertStates, alertTone, isPortAlert, summarizeAlerts } from './wallAlerts';

export function lampState(port: Port, alerts: Alert[], reachable: boolean) {
  const relevant = alerts.filter(alert => isPortAlert(alert) && alert.if_index === port.if_index);
  const tone = alertTone(summarizeAlerts(relevant), reachable && !port.excluded && port.oper === 'up');
  const label = tone !== 'unknown' ? alertStates[tone].label :
    port.excluded || relevant.some(alert => alert.suppressed) ? 'rack.suppressed' : !reachable ? 'rack.offline' : 'rack.unused';
  return { tone, symbol: alertStates[tone].symbol, label } as const;
}

export function sortedAlerts(alerts: Alert[]): Alert[] {
  const severity = (value: string) => ({ CRITICAL: 0, WARNING: 1, INFO: 2 }[value] ?? 3);
  return asArray(alerts).slice().sort((left, right) => Number(left.suppressed) - Number(right.suppressed) ||
    severity(left.severity) - severity(right.severity) || Date.parse(right.ts) - Date.parse(left.ts) || right.event_id - left.event_id);
}

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
