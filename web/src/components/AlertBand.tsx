import { useEffect, useRef, useState } from 'react';
import { pcTimestamp } from '../pcTime';
import { eventHint, eventTitle, t } from '../i18n';
import { sortedAlerts } from '../rackModel';
import type { Alert } from '../types';
import { wallAlertCount } from '../wallAlerts';

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
  const rows = sortedAlerts(alerts);
  const band = useRef<HTMLElement>(null);
  const heading = useRef<HTMLDivElement>(null);
  const more = useRef<HTMLParagraphElement>(null);
  const [visibleCount, setVisibleCount] = useState<number>();
  useEffect(() => {
    const element = band.current;
    const parent = element?.parentElement;
    if (!wall || !element || !parent) return;
    const measure = () => {
      const list = element.querySelector<HTMLElement>('.alert-list');
      if (!list || !heading.current || !more.current) return;
      const style = getComputedStyle(element);
      const headStyle = getComputedStyle(heading.current);
      const gap = parseFloat(getComputedStyle(list).rowGap) || 0;
      const overhead = heading.current.offsetHeight + (parseFloat(headStyle.marginBottom) || 0)
        + (parseFloat(style.paddingTop) || 0) + (parseFloat(style.paddingBottom) || 0)
        + (parseFloat(style.borderTopWidth) || 0) + (parseFloat(style.borderBottomWidth) || 0);
      const heights = Array.from(list.querySelectorAll<HTMLElement>('[data-wall-alert]'), row => row.offsetHeight);
      const budget = parent.clientHeight * 0.6 - overhead;
      setVisibleCount(wallAlertCount(heights, Math.max(0, budget), gap, more.current.offsetHeight));
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(parent);
    observer.observe(element);
    element.querySelectorAll<HTMLElement>('[data-wall-alert]').forEach(row => observer.observe(row));
    return () => observer.disconnect();
  }, [wall, alerts]);
  const shown = wall ? Math.min(visibleCount ?? rows.length, rows.length) : rows.length;
  const remaining = rows.length - shown;
  return <section ref={band} className="alert-band" aria-label={t('rack.alertBand')}>
    <div ref={heading} className="flex items-center justify-between gap-2 mb-2">
      <h2 className="font-bold">{t('rack.alertBand')} <span className="tabular-nums">{rows.filter(alert => !alert.suppressed).length}</span></h2>
      {!wall && ack && rows.some(alert => !alert.acked) && <button data-action="ack-all" disabled={busy} onClick={() => ack()}>{t('rack.ackAll')}</button>}
    </div>
    <div className="alert-list" role="log" aria-live="polite">
      {rows.length ? rows.map((alert, index) => wall
        ? <div key={alert.event_id} data-wall-alert className={index >= shown ? 'wall-alert-overflow' : undefined} aria-hidden={index >= shown || undefined}>
          <AlertEntry alert={alert} wall busy={busy} />
        </div>
        : <AlertEntry key={alert.event_id} alert={alert} busy={busy} ack={ack} />)
        : pending ? <p>{t('rack.loading')}</p> : <p className="alert-clear">{t('rack.clear')}</p>}
      {wall && <p ref={more} className={remaining ? 'wall-alert-more' : 'wall-alert-more wall-alert-overflow'} aria-hidden={!remaining || undefined}>
        {t('rack.moreAlerts', { count: remaining })}
      </p>}
    </div>
  </section>;
}
