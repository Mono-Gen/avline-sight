import type React from 'react';
import { jstTime, pcTimestamp } from '../pcTime';
import { canAcknowledgeAlert } from '../events';
import { eventHeadline, eventHint, eventTitle, t } from '../i18n';
import { sortedAlerts } from '../rackModel';
import type { Alert } from '../types';
import { alertTone, groupWallAlerts, summarizeAlerts } from '../wallAlerts';
import { ErrorSymbol } from './ErrorSymbol';
import { AlertTechnicalDetails } from './AlertTechnicalDetails';

// Places a time element where the translation puts {time} (time first in Japanese, label first in English).
function withTime(key: 'alertState.raisedAt' | 'alertState.clearedAt', time: React.ReactNode) {
  const [before, after = ''] = t(key, { time: '\uE000' }).split('\uE000');
  return <>{before}{time}{after}</>;
}

export function AlertEntry({ alert, busy, ack, expert, wall = false }: { alert: Alert; busy: boolean; ack?: (id: number) => void; expert?: boolean; wall?: boolean }) {
  const hint = eventHint(alert);
  const tone = alertTone(summarizeAlerts([alert]), false);
  const recoveredAt = !alert.active && alert.cleared_at;
  const state = alert.active ? (alert.acked ? 'ackedActive' : 'active') : 'recovered';
  return <article className={`alert-entry alert-${tone} ${alert.active ? 'alert-active' : 'alert-recovered'}`}>
    <span className="alert-symbol" aria-hidden="true">{alert.suppressed ? '🔧' : tone === 'unknown' ? '—'
      : tone === 'critical' ? <ErrorSymbol /> : <span className={tone === 'caution' ? 'caution-symbol' : 'warning-symbol'}>▲</span>}</span>
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap gap-x-3"><strong>{alert.switch_name} {alert.port} {alert.device_name}</strong>
        {/* State first as a coloured badge, then short times without the zone (the header clock shows JST). */}
        <span className={`alert-state-badge state-${state}`}>{t(`alertState.${state}`)}</span>
        <span className="alert-times">
          {withTime('alertState.raisedAt', <time dateTime={alert.ts} title={pcTimestamp(alert.ts, true)}>{jstTime(alert.ts, false)}</time>)}
          {recoveredAt && <> → {withTime('alertState.clearedAt', <time dateTime={recoveredAt} title={pcTimestamp(recoveredAt, true)}>{jstTime(recoveredAt, false)}</time>)}</>}
        </span>
        {alert.suppressed && <span>{t('rack.suppressed')}</span>}
      </div>
      <p className="alert-title">{expert ? eventTitle(alert) : eventHeadline(alert)}</p>
      {expert && hint && !wall && <details className="alert-hint-fold"><summary>{t('alertState.whatToDo')}</summary><p className="alert-hint">{hint}</p></details>}
      {expert && <AlertTechnicalDetails event={alert} counters={alert.raw_counters ?? alert.detail?.raw_counters} />}
    </div>
    {ack && canAcknowledgeAlert(alert) && <button className="button-primary" data-action="ack" disabled={busy} onClick={() => ack(alert.event_id)}>{t('rack.ack')}</button>}
  </article>;
}
export function AlertBand({ alerts, wall = false, busy = false, ack, ackMany, pending = false, locked = false, undoCount = 0, undo, expert }: {
  alerts: Alert[]; wall?: boolean; busy?: boolean; ack?: (id?: number) => void; ackMany?: (ids: number[]) => void;
  pending?: boolean; locked?: boolean; undoCount?: number; undo?: () => void; expert?: boolean;
}) {
  const counts = summarizeAlerts(alerts);
  const rows = wall ? counts.items.filter(alert => !alert.acked) : sortedAlerts([
    ...counts.items, ...alerts.filter(alert => alert.suppressed && (alert.active || !alert.acked)),
  ]);
  const groups = wall ? groupWallAlerts(rows) : [];
  const canAck = !locked && (ack || ackMany);
  const ackRows = rows.filter(canAcknowledgeAlert);
  return <section className={`alert-band${wall && groups.length > 6 ? ' wall-alerts-dense' : ''}`} aria-label={t('rack.alertBand')}>
    <div className="alert-band-head">
      <h2 className="font-bold">{wall ? t('cleanup.allAlerts', { count: counts.unacked }) : t('rack.alertBand')} {!wall && <span className="tabular-nums">{counts.count}</span>}</h2>
      {canAck && ackRows.length > 0 && <button className="button-primary" data-action="ack-all" disabled={busy} onClick={() => {
        if (!ackMany) ack?.();
        else if (wall || window.confirm(t('rack.ackAllConfirm'))) ackMany(ackRows.map(alert => alert.event_id));
      }}>{t('rack.ackAll')}</button>}
    </div>
    {wall && locked && <p className="wall-lock-note">{t('wall.locked')}</p>}
    {!locked && undoCount > 0 && undo && <div className="ack-undo" role="status">{t('wall.acknowledged', { count: undoCount })} <button data-action="undo" disabled={busy} onClick={undo}>{t('wall.undo')}</button></div>}
    <div className="alert-list" role="log" aria-live="polite">
      {rows.length ? wall ? groups.map(group => <div key={group.key} className={`wall-alert-group ${group.members.some(alert => alert.active && alert.severity === 'CRITICAL') ? 'alert-active'
        : group.members.some(alert => alert.active) ? 'wall-group-caution' : 'alert-recovered'}`}>
        <AlertEntry alert={group.alert} busy={busy} expert={expert} wall ack={canAck ? () => ackMany ? ackMany(group.members.map(alert => alert.event_id)) : ack?.(group.alert.event_id) : undefined} />
        {group.members.length > 1 && <details><summary>{expert ? eventTitle(group.alert) : eventHeadline(group.alert)}{t('wall.related', { count: group.members.length })}</summary>
          {group.members.map(alert => <AlertEntry key={alert.event_id} alert={alert} busy={busy} expert={expert} wall />)}
        </details>}
      </div>) : rows.map(alert => <AlertEntry key={alert.event_id} alert={alert} busy={busy} expert={expert} ack={canAck ? ack : undefined} />)
        : pending ? <p>{t('rack.loading')}</p> : <p className="alert-clear">{t('rack.clear')}</p>}
    </div>
  </section>;
}
