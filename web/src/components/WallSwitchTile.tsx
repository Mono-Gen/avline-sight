import { asArray } from '../format';
import { t } from '../i18n';
import { issueAlerts } from '../monitorHealth';
import { lampState } from '../rackModel';
import type { Alert, InventoryRow, Port, SwitchSummary } from '../types';

export function WallSwitchTile({ summary, alerts, ports, devices, stale, suppressed = false }: {
  summary: SwitchSummary; alerts: Alert[]; ports: Port[]; devices: InventoryRow[]; stale: boolean; suppressed?: boolean;
}) {
  const issues = issueAlerts(alerts);
  let critical = issues.some(alert => alert.severity === 'CRITICAL');
  let warning = issues.length > 0;
  const names = new Set(issues.map(alert => [alert.port, alert.device_name].filter(Boolean).join(' · ') || alert.title));
  for (const port of ports) {
    const attached = devices.filter(device => device.if_index === port.if_index);
    const state = lampState(port, alerts.filter(alert => alert.if_index === port.if_index), attached, suppressed, summary.reachable);
    critical ||= state.tone === 'critical';
    warning ||= state.tone === 'warning';
    if (state.tone === 'critical' || state.tone === 'warning') {
      names.add([port.name, ...attached.map(device => device.label || device.lldp_name || device.mac)].join(' · '));
    }
  }
  const unknown = stale || (!summary.reachable || ports.length === 0) && !critical && !warning;
  const tone = unknown ? 'unknown' : critical ? 'critical' : warning ? 'warning' : 'healthy';
  const count = Math.max(issues.length, names.size);
  return <article className={`wall-switch-tile status-${tone}`}>
    <h2>{unknown ? '—' : count ? critical ? '✖' : '▲' : '✓'} {summary.name}</h2>
    <p>{summary.group_tag || t('rack.ungrouped')} · {summary.ip}</p>
    <strong>{unknown ? t('cleanup.switchUnknown') : count ? t('cleanup.issues', { count }) : t('cleanup.switchOK')}</strong>
    {names.size > 0 && <ul>{[...names].map(name => <li key={name}>{name}</li>)}</ul>}
    {!summary.reachable && <p>{t('rack.offline')}</p>}
    {stale && <p>{t('rack.stale')}</p>}
  </article>;
}
