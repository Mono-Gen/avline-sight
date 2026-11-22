import { asArray, formatTime } from '../format';
import { eventHint, eventTitle, t } from '../i18n';
import { sortedAlerts } from '../rackModel';
import type { Alert, Event } from '../types';

export function enrichAlert(alert: Alert, history: Event[]): Alert {
  const event = asArray(history).find(item => item.id === alert.event_id);
  return { ...alert, detail: alert.detail || event?.detail, hint: alert.hint || event?.hint, acked_at: alert.acked_at || event?.acked_at };
}
export function AlertEntry({ alert, wall, busy, ack }: { alert: Alert; wall?: boolean; busy: boolean; ack?: (id: number) => void }) {
  return <article className={`alert-entry ${alert.suppressed ? 'alert-suppressed' : alert.severity === 'CRITICAL' ? 'alert-critical' : 'alert-warning'} ${alert.acked ? 'alert-acked' : ''}`}>
    <span className="alert-symbol" aria-hidden="true">{alert.suppressed ? '🔧' : alert.severity === 'CRITICAL' ? '✖' : '▲'}</span>
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap gap-x-3"><strong>{alert.switch_name} {alert.port} {alert.device_name}</strong>
        <time dateTime={alert.ts}>{formatTime(alert.ts)}</time>
        <span>{t(alert.suppressed ? 'rack.suppressed' : alert.acked ? 'rack.ackedActive' : alert.active ? 'rack.active' : 'rack.latched')}</span>
      </div>
      <p>{eventTitle(alert)}</p>
      {eventHint(alert) && <p className="alert-hint">{eventHint(alert)}</p>}
    </div>
    {!wall && !alert.acked && ack && <button data-action="ack" disabled={busy} onClick={() => ack(alert.event_id)}>{t('rack.ack')}</button>}
  </article>;
}
export function AlertBand({ alerts, history = [], wall = false, busy = false, ack }: {
  alerts: Alert[]; history?: Event[]; wall?: boolean; busy?: boolean; ack?: (id?: number) => void;
}) {
  const rows = sortedAlerts(alerts);
  return <section className="alert-band" aria-label={t('rack.alertBand')}>
    <div className="flex items-center justify-between gap-2 mb-2">
      <h2 className="font-bold">{t('rack.alertBand')} <span className="tabular-nums">{rows.filter(alert => !alert.suppressed).length}</span></h2>
      {!wall && ack && rows.some(alert => !alert.acked) && <button data-action="ack-all" disabled={busy} onClick={() => ack()}>{t('rack.ackAll')}</button>}
    </div>
    <div className="alert-list" role="log" aria-live="polite">
      {rows.length ? rows.map(alert => <AlertEntry key={alert.event_id} alert={enrichAlert(alert, history)} wall={wall} busy={busy} ack={ack} />)
        : <p className="text-emerald-700 dark:text-emerald-300">{t('rack.clear')}</p>}
    </div>
  </section>;
}
