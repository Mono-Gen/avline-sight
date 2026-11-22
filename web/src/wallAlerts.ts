import { issueAlerts } from './monitorHealth';
import type { Alert } from './types';

export function summarizeAlerts(alerts: Alert[]) {
  const items = issueAlerts(alerts).filter(alert => alert.active || !alert.acked);
  return {
    items,
    count: items.length,
    active: items.filter(alert => alert.active).length,
    unacked: items.filter(alert => !alert.acked).length,
    critical: items.some(alert => alert.severity === 'CRITICAL'),
  };
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
  return [...groups].map(([key, members]) => ({
    key,
    alert: members.find(alert => alert.active && alert.severity === 'CRITICAL') || members.find(alert => alert.active) || members[0],
    members,
  }));
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
