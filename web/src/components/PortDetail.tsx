import { api } from '../api';
import { asArray } from '../format';
import { pcTimestamp } from '../pcTime';
import { eventHint, eventTitle, t } from '../i18n';
import { pingLabel } from '../deviceCsv';
import type { Alert, InventoryRow, MetricResult, Port } from '../types';
import { usePolling } from '../usePolling';
import { Chart } from './Chart';

export function PortDetail({ switchId, switchName, port, devices, alerts, close, replay }: {
  switchId: number; switchName: string; port: Port; devices: InventoryRow[]; alerts: Alert[]; close: () => void; replay: boolean;
}) {
  const metrics = usePolling(signal => {
    const to = new Date();
    const from = new Date(to.getTime() - 15 * 60 * 1000);
    const params = new URLSearchParams({ switch_id: String(switchId), if_index: String(port.if_index), from: from.toISOString(), to: to.toISOString() });
    return api<MetricResult>(`/api/metrics/port?${params}`, { signal });
  }, 10000, [switchId, port.if_index], !replay);
  const known = new Set(devices.map(device => device.mac));
  const list: Pick<InventoryRow, 'mac' | 'label' | 'lldp_name' | 'ip' | 'planned_ip' | 'ip_mismatch' | 'ping' | 'ping_ok' | 'ping_ts' | 'ping_rtt_ms'>[] = [
    ...devices,
    ...asArray(port.devices).filter(device => !known.has(device.mac)).map(device => ({
      mac: device.mac, label: device.label || '', lldp_name: device.lldp_name || '', ip: device.ip || '',
    })),
  ];
  return <aside className="port-detail" role="dialog" aria-label={t('rack.portDetail')}>
    <div className="flex items-center justify-between gap-2"><h2>{t('rack.portDetail')} · {switchName} / {port.name}</h2><button onClick={close}>{t('rack.close')}</button></div>
    {list.map(device => <section key={device.mac} className="border-b border-slate-600 py-3">
      <h3 className="font-bold">{device.label || device.lldp_name || device.mac}</h3>
      <p>MAC: {device.mac || '—'}</p><p>IP: {device.ip || '—'} {device.ip_mismatch && <span title={t('operations.mismatch')}>▲</span>}</p>
      <p>{t('rack.plannedIP')}: {device.planned_ip || '—'}</p>
      <p>{t('rack.pingStatus')}: {device.ping ? pingLabel(device.ping.status, device.ping.rtt_ms)
        : device.ping_ok == null ? t('rack.notPinged') : pingLabel(device.ping_ok ? 'ok' : 'timeout', device.ping_rtt_ms || 0)} {device.ping?.at || device.ping_ts ? pcTimestamp(device.ping?.at || device.ping_ts || '') : ''}</p>
    </section>)}
    {alerts.map(alert => <article key={alert.event_id} className="border-b border-slate-600 py-3">
      <h3>{alert.severity === 'CRITICAL' ? '✖' : '▲'} {eventTitle(alert)}</h3>
      <p>{eventHint(alert)}</p>
      <p>{t('rack.raised')}: <time dateTime={alert.ts}>{pcTimestamp(alert.ts)}</time></p>
      <p>{t('rack.cleared')}: {alert.cleared_at ? <time dateTime={alert.cleared_at}>{pcTimestamp(alert.cleared_at)}</time> : '—'}</p>
      <p>{t('rack.acked')}: {alert.acked_at ? <time dateTime={alert.acked_at}>{pcTimestamp(alert.acked_at)}</time> : '—'}</p>
    </article>)}
    <h3 className="mt-4 font-bold">{t('rack.bandwidth')}</h3>
    {replay && <p>{t('rack.noMetrics')}</p>}
    {metrics.pending && <p role="status">{t('rack.loading')}</p>}
    {metrics.error && <p role="alert">⚠ {metrics.error}</p>}
    {metrics.data && (asArray(metrics.data.points).length ? <Chart points={metrics.data.points} keys={['rx_bps', 'tx_bps']} /> : <p>{t('rack.noMetrics')}</p>)}
    {metrics.updatedAt && <p className="text-sm">{t('rack.lastUpdated', { at: pcTimestamp(metrics.updatedAt.getTime()) })}</p>}
  </aside>;
}
