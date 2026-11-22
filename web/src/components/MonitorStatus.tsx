import { t } from '../i18n';
import { jstTime } from '../pcTime';
import { ErrorSymbol } from './ErrorSymbol';

export function OverallStatus({ count, critical, ready, stopped, replay = false, label, active, unacked }: {
  count: number; critical: boolean; ready: boolean; stopped: boolean; replay?: boolean; label?: string; active?: number; unacked?: number;
}) {
  const unknown = stopped || !ready;
  const onlyUnacked = active === 0 && !!unacked;
  const tone = unknown ? 'unknown' : count ? critical && !onlyUnacked ? 'critical' : 'warning' : 'healthy';
  return <div className={`overall-status status-${tone}`} role="status">
    {label && <span>{label}</span>}
    <strong>{unknown ? '—' : count ? active !== undefined || critical ? <ErrorSymbol /> : '▲' : '✓'} {unknown
      ? t(stopped ? 'cleanup.unknown' : 'cleanup.pending') : count && active !== undefined && unacked !== undefined
        ? t('wall.counts', { active, unacked }) : t(count ? 'cleanup.issues' : 'cleanup.normal', { count })}</strong>
    {replay && <small>{t('rack.replay')}</small>}
  </div>;
}

export function MonitorStopped({ updatedAt }: { updatedAt?: number }) {
  return <div className="monitor-stalled" role="alert" aria-live="assertive">
    <strong>⚠ {t('cleanup.stopped')}</strong>
    <time dateTime={updatedAt !== undefined ? new Date(updatedAt).toISOString() : undefined}>{t('rack.lastUpdated', { at: jstTime(updatedAt ?? NaN) })}</time>
    <p>{t('cleanup.stoppedHelp')}</p>
  </div>;
}
