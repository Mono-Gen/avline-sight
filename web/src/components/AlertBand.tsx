import { pcTimestamp } from '../pcTime';
import { eventHint, eventTitle, t } from '../i18n';
import { sortedAlerts } from '../rackModel';
import type { Alert } from '../types';
import { groupWallAlerts, summarizeAlerts } from '../wallAlerts';
import { ErrorSymbol } from './ErrorSymbol';

export function AlertEntry({ alert, wall, busy, ack }: { alert: Alert; wall?: boolean; busy: boolean; ack?: (id: number) => void }) {
  const hint = eventHint(alert);
  return <article className={`alert-entry ${alert.suppressed ? 'alert-suppressed' : alert.active ? 'alert-active' : 'alert-recovered'} ${alert.severity === 'CRITICAL' ? 'alert-critical' : 'alert-warning'} ${alert.acked ? 'alert-acked' : ''}`}>
    <span className="alert-symbol" aria-hidden="true">{alert.suppressed ? '🔧' : alert.severity === 'CRITICAL' ? <ErrorSymbol /> : '▲'}</span>
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap gap-x-3"><strong>{alert.switch_name} {alert.port} {alert.device_name}</strong>
        <span>{t('rack.raised')} <time dateTime={alert.ts}>{pcTimestamp(alert.ts)}</time>
          {!alert.active && alert.cleared_at && <> → {t('rack.cleared')} <time dateTime={alert.cleared_at}>{pcTimestamp(alert.cleared_at)}</time></>}</span>
        <span>{t(alert.acked ? 'rack.ackedActive' : alert.active ? 'rack.active' : 'wall.recovered')}</span>
        {alert.suppressed && <span>{t('rack.suppressed')}</span>}
      </div>
      <p>{eventTitle(alert)}</p>
      {hint && <p className="alert-hint">{hint}</p>}
    </div>
    {!alert.acked && ack && <button data-action="ack" disabled={busy} onClick={() => ack(alert.event_id)}>{t('rack.ack')}</button>}
  </article>;
}
export function AlertBand({ alerts, wall = false, busy = false, ack, ackMany, pending = false, locked = false, undoCount = 0, undo }: {
  alerts: Alert[]; wall?: boolean; busy?: boolean; ack?: (id?: number) => void; ackMany?: (ids: number[]) => void;
  pending?: boolean; locked?: boolean; undoCount?: number; undo?: () => void;
}) {
  const rows = wall ? summarizeAlerts(alerts).items.filter(alert => !alert.acked) : sortedAlerts(alerts);
  const groups = wall ? groupWallAlerts(rows) : [];
  const canAck = !locked && (ack || ackMany);
  return <section className={`alert-band${wall && groups.length > 6 ? ' wall-alerts-dense' : ''}`} aria-label={t('rack.alertBand')}>
    <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
      <h2 className="font-bold">{wall ? t('cleanup.allAlerts', { count: rows.length }) : t('rack.alertBand')} {!wall && <span className="tabular-nums">{rows.filter(alert => !alert.suppressed).length}</span>}</h2>
      {canAck && rows.some(alert => !alert.acked) && <button data-action="ack-all" disabled={busy} onClick={() => wall && ackMany
        ? ackMany(rows.map(alert => alert.event_id)) : ack?.()}>{t('rack.ackAll')}</button>}
    </div>
    {wall && locked && <p className="wall-lock-note">{t('wall.locked')}</p>}
    {!locked && undoCount > 0 && undo && <div className="ack-undo" role="status">{t('wall.acknowledged', { count: undoCount })} <button data-action="undo" disabled={busy} onClick={undo}>{t('wall.undo')}</button></div>}
    <div className="alert-list" role="log" aria-live="polite">
      {rows.length ? wall ? groups.map(group => <div key={group.key} className={`wall-alert-group ${group.members.some(alert => alert.active) ? 'alert-active' : 'alert-recovered'}`}>
        <AlertEntry alert={group.alert} wall busy={busy} ack={canAck ? () => ackMany ? ackMany(group.members.map(alert => alert.event_id)) : ack?.(group.alert.event_id) : undefined} />
        {group.members.length > 1 && <details><summary>{eventTitle(group.alert)}{t('wall.related', { count: group.members.length })}</summary>
          {group.members.map(alert => <AlertEntry key={alert.event_id} alert={alert} wall busy={busy} />)}
        </details>}
      </div>) : rows.map(alert => <AlertEntry key={alert.event_id} alert={alert} busy={busy} ack={canAck ? ack : undefined} />)
        : pending ? <p>{t('rack.loading')}</p> : <p className="alert-clear">{t('rack.clear')}</p>}
    </div>
  </section>;
}
