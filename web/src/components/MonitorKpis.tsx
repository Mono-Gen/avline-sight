import { formatBps } from '../format';
import { t } from '../i18n';
import type { Alert } from '../types';
import { summarizeAlerts } from '../wallAlerts';

// Number panels above the port monitor. Always whole-project values, also while one switch or a filter is shown, so faults elsewhere are not missed.
export function MonitorKpis({ switches, alerts, uplink, alertsKnown = true }: {
  switches: readonly { reachable: boolean; ports_up?: number }[];
  alerts: readonly Alert[];
  // Uplink total summed from the existing snapshot; undefined when no uplink is known.
  uplink?: { rx: number; tx: number };
  // False when the alert list is not available (legacy replay): show "—" instead of a misleading zero.
  alertsKnown?: boolean;
}) {
  const counts = summarizeAlerts([...alerts]);
  const recovered = counts.count - counts.active;
  const reachable = switches.filter(sw => sw.reachable).length;
  const linked = switches.reduce((sum, sw) => sum + (sw.ports_up || 0), 0);
  const items: { key: string; label: string; value: string; tone?: 'critical' | 'warning'; small?: boolean }[] = [
    { key: 'switches', label: t('focus.kpiSwitches'), value: `${reachable} / ${switches.length}`, tone: reachable < switches.length ? 'critical' : undefined },
    { key: 'active', label: t('focus.kpiActive'), value: alertsKnown ? String(counts.active) : '—', tone: alertsKnown && counts.active ? 'critical' : undefined },
    { key: 'recovered', label: t('focus.kpiRecovered'), value: alertsKnown ? String(recovered) : '—', tone: alertsKnown && recovered ? 'warning' : undefined },
    { key: 'linked', label: t('focus.kpiLinked'), value: String(linked) },
    { key: 'uplink', label: t('focus.kpiUplink'), small: true,
      value: uplink ? t('metrics.rxtx', { rx: formatBps(uplink.rx), tx: formatBps(uplink.tx) }) : '—' },
  ];
  return <section className="kpi-row" aria-label={t('focus.kpi')}>
    {items.map(item => <div key={item.key} className={`kpi${item.tone ? ` kpi-${item.tone}` : ''}`} data-kpi={item.key}>
      <div className="kpi-label">{item.label}</div>
      <div className={`kpi-value${item.small ? ' kpi-value-small' : ''}`}>{item.value}</div>
      <div className="kpi-scope">{t('focus.kpiScope')}</div>
    </div>)}
  </section>;
}
