import { issueAlerts } from './monitorHealth';
import type { Alert, InventoryRow, SwitchState } from './types';

export function replayIssueCount(states: SwitchState[]) {
  return states.reduce((count, state) => count + (state.alert_counts?.CRITICAL || 0) + (state.alert_counts?.WARNING || 0), 0);
}

export function summarizeAlerts(alerts: Alert[]) {
  const items = issueAlerts(alerts).filter(alert => alert.active || !alert.acked);
  const activeItems = items.filter(alert => alert.active);
  return {
    items,
    count: items.length,
    activeItems,
    active: activeItems.length,
    unacked: items.filter(alert => !alert.acked).length,
  };
}

export const alertStates = {
  critical: { symbol: '!', label: 'rack.active', color: '#ff4d5e' },
  warning: { symbol: '!', label: 'wall.recovered', color: '#facc15' },
  healthy: { symbol: '✓', label: 'cleanup.switchOK', color: '#2dd4bf' },
  unknown: { symbol: '—', label: 'cleanup.switchUnknown', color: '#9aa3b2' },
} as const;

export function alertTone(counts: ReturnType<typeof summarizeAlerts>, known: boolean) {
  return counts.active ? 'critical' : counts.unacked ? 'warning' : known ? 'healthy' : 'unknown';
}

export const isPortAlert = (alert: Alert) => alert.if_index > 0 && !['FAN_FAIL', 'PSU_FAIL'].includes(alert.code);

function matchesPort(alert: Alert, switchId: number | undefined, ifIndex: number | undefined) {
  return alert.switch_id === switchId && alert.if_index === ifIndex && isPortAlert(alert);
}

export function matchingPortAlerts(alerts: Alert[], switchId: number, ifIndex: number) {
  return alerts.filter(alert => matchesPort(alert, switchId, ifIndex));
}

export function matchingDeviceAlerts(alerts: Alert[], device: InventoryRow) {
  const mac = device.mac.toLowerCase();
  return alerts.filter(alert => alert.mac ? alert.mac.toLowerCase() === mac : matchesPort(alert, device.switch_id, device.if_index));
}

export interface AlertGroup { key: string; alert: Alert; members: Alert[] }

export function groupWallAlerts(alerts: Alert[]): AlertGroup[] {
  const groups = new Map<string, Alert[]>();
  const rows = summarizeAlerts(alerts).items.filter(alert => !alert.acked)
    .sort((a, b) => Date.parse(b.ts) - Date.parse(a.ts) || b.event_id - a.event_id);
  for (const alert of rows) {
    const timestamp = Date.parse(alert.ts);
    const minute = Number.isFinite(timestamp) ? Math.floor(timestamp / 60000) : alert.event_id;
    const key = JSON.stringify([alert.switch_id, alert.if_index, minute]);
    const members = groups.get(key) || [];
    members.push(alert);
    groups.set(key, members);
  }
  // Ongoing groups always come first so a live fault is never pushed down by recovered ones (newest first within each).
  return [...groups].map(([key, members]) => ({
    key,
    alert: members.find(alert => alert.active && alert.severity === 'CRITICAL') || members.find(alert => alert.active) || members[0],
    members,
  })).sort((a, b) => Number(b.members.some(alert => alert.active)) - Number(a.members.some(alert => alert.active)));
}

export function wallAlertCount(heights: number[], available: number, gap: number, moreHeight: number): number {
  const total = heights.reduce((sum, height) => sum + height, 0) + Math.max(0, heights.length - 1) * gap;
  if (total <= available) return heights.length;
  let used = moreHeight;
  let count = 0;
  for (const height of heights) {
    if (used + height + gap > available) break;
    used += height + gap;
    count++;
  }
  return count;
}
