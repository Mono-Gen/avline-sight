import type { Alert } from './types';

// A 15-second deadline tolerates seven missed 2-second overview ticks before marking data stale.
export const MONITOR_STALE_MS = 15000;

export function monitoringStopped(online: boolean, updatedAt: number | undefined, now: number) {
  return !online || updatedAt === undefined || now - updatedAt >= MONITOR_STALE_MS;
}

export function isIssueAlert(alert: Alert) {
  return !alert.suppressed && (alert.severity === 'CRITICAL' || alert.severity === 'WARNING');
}

export function issueAlerts(alerts: Alert[]) {
  return alerts.filter(isIssueAlert);
}

export function newIssueAlerts(alerts: Alert[], previous: Set<number>) {
  return issueAlerts(alerts).filter(alert => !alert.acked && !previous.has(alert.event_id));
}
