import { pcTimestamp } from '../pcTime';
import { eventHint, eventTitle, t } from '../i18n';
import { sortedAlerts } from '../rackModel';
import type { Alert } from '../types';

export function AlertEntry({ alert, wall, busy, ack }: { alert: Alert; wall?: boolean; busy: boolean; ack?: (id: number) => void }) {
  const hint = eventHint(alert);
  return <article className={`alert-entry ${alert.suppressed ? 'alert-suppressed' : alert.severity === 'CRITICAL' ? 'alert-critical' : 'alert-warning'} ${alert.acked ? 'alert-acked' : ''}`}>
    <span className="alert-symbol" aria-hidden="true">{alert.suppressed ? '🔧' : alert.severity === 'CRITICAL' ? '✖' : '▲'}</span>
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap gap-x-3"><strong>{alert.switch_name} {alert.port} {alert.device_name}</strong>
        <time dateTime={alert.ts}>{pcTimestamp(alert.ts)}</time>
        <span>{t(alert.acked ? 'rack.ackedActive' : alert.active ? 'rack.active' : 'rack.latched')}</span>
        {alert.suppressed && <span>{t('rack.suppressed')}</span>}
      </div>
      <p>{eventTitle(alert)}</p>
      {!alert.active && <p>{t('rack.cleared')}: {alert.cleared_at
        ? <time dateTime={alert.cleared_at}>{pcTimestamp(alert.cleared_at)}</time> : t('rack.unknownTime')}</p>}
      {hint && <p className="alert-hint">{hint}</p>}
    </div>
    {!wall && !alert.acked && ack && <button data-action="ack" disabled={busy} onClick={() => ack(alert.event_id)}>{t('rack.ack')}</button>}
  </article>;
}
export function AlertBand({ alerts, wall = false, busy = false, ack, pending = false }: {
  alerts: Alert[]; wall?: boolean; busy?: boolean; ack?: (id?: number) => void; pending?: boolean;
}) {
  const rows = wall ? alerts.filter(alert => !alert.acked && !alert.suppressed)
    .sort((a, b) => Date.parse(b.ts) - Date.parse(a.ts) || b.event_id - a.event_id) : sortedAlerts(alerts);
  const size = wall ? Math.max(0.7, Math.min(1.25, 8 / Math.max(1, rows.length))) : undefined;
  return <section className={`alert-band${wall && rows.length > 6 ? ' wall-alerts-dense' : ''}`} aria-label={t('rack.alertBand')} style={{ fontSize: size ? `${size}rem` : undefined }}>
    <div className="flex items-center justify-between gap-2 mb-2">
      <h2 className="font-bold">{wall ? t('cleanup.allAlerts', { count: rows.length }) : t('rack.alertBand')} {!wall && <span className="tabular-nums">{rows.filter(alert => !alert.suppressed).length}</span>}</h2>
      {!wall && ack && rows.some(alert => !alert.acked) && <button data-action="ack-all" disabled={busy} onClick={() => ack()}>{t('rack.ackAll')}</button>}
    </div>
    <div className="alert-list" role="log" aria-live="polite">
      {rows.length ? rows.map(alert => <AlertEntry key={alert.event_id} alert={alert} wall={wall} busy={busy} ack={ack} />)
        : pending ? <p>{t('rack.loading')}</p> : <p className="alert-clear">{t('rack.clear')}</p>}
    </div>
  </section>;
}
