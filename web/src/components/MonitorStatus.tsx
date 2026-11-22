import { t } from '../i18n';
import { pcTimestamp } from '../pcTime';

export function OverallStatus({ count, critical, ready, stopped, replay = false }: {
  count: number; critical: boolean; ready: boolean; stopped: boolean; replay?: boolean;
}) {
  const unknown = stopped || !ready;
  const tone = unknown ? 'unknown' : count ? critical ? 'critical' : 'warning' : 'healthy';
  return <div className={`overall-status status-${tone}`} role="status">
    <strong>{unknown ? '—' : count ? critical ? '✖' : '▲' : '✓'} {unknown
      ? t(stopped ? 'cleanup.unknown' : 'cleanup.pending') : t(count ? 'cleanup.issues' : 'cleanup.normal', { count })}</strong>
    {replay && <small>{t('rack.replay')}</small>}
  </div>;
}

export function MonitorStopped({ updatedAt }: { updatedAt?: number }) {
  return <div className="monitor-stalled" role="alert" aria-live="assertive">
    <strong>⚠ {t('cleanup.stopped')}</strong>
    <time dateTime={updatedAt ? new Date(updatedAt).toISOString() : undefined}>{t('cleanup.lastUpdate', { at: updatedAt ? pcTimestamp(updatedAt) : '—' })}</time>
    <p>{t('cleanup.stoppedHelp')}</p>
  </div>;
}
