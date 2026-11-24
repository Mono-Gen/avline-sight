import { eventTitle, t } from '../i18n';
import { alertStates, alertTone, summarizeAlerts } from '../wallAlerts';
import { ErrorSymbol } from './ErrorSymbol';
import type { Alert, InventoryRow, Port, SwitchSummary } from '../types';

export function WallSwitchTile({ summary, alerts, stale, selected = false, onSelect }: {
  summary: SwitchSummary; alerts: Alert[]; ports?: Port[]; devices?: InventoryRow[]; stale: boolean; suppressed?: boolean;
  selected?: boolean; onSelect?: () => void;
}) {
  const counts = summarizeAlerts(alerts);
  const current = counts.activeItems;
  const tone = alertTone(counts, !stale && summary.reachable);
  const state = alertStates[tone];
  return <article className={`wall-switch-tile status-${tone}${selected ? ' wall-switch-selected' : ''}`}>
    <h2>{onSelect ? <button className="wall-switch-filter" aria-pressed={selected} onClick={onSelect}>{summary.name}</button> : summary.name}</h2>
    <p className="wall-switch-meta">{summary.group_tag || t('rack.ungrouped')} {summary.ip && <>· {summary.ip}</>}</p>
    {/* Same shapes everywhere: ongoing = round "!", recovered and unacknowledged = triangle. */}
    <strong>{tone === 'critical' ? <><ErrorSymbol /> </> : tone === 'warning' ? <><span className="warning-symbol">▲</span> </> : null}{t(state.label)}</strong>
    {counts.count > 0 && <p className="wall-switch-counts">{t('wall.counts', counts)}</p>}
    {current.length > 0 && <ul>{current.slice(0, 3).map(alert => <li key={alert.event_id}>
      {alert.device_name || alert.port || summary.name} · {eventTitle(alert)}
    </li>)}</ul>}
    {current.length > 3 && <p>{t('wall.more', { count: current.length - 3 })}</p>}
    {summary.needs_credentials ? <p>{t('configFile.credentialsRequired')}</p> :
      !summary.reachable && <p>{t('rack.offline')}</p>}
    {stale && <p>{t('rack.stale')}</p>}
  </article>;
}
