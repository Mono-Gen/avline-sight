import { api, json } from '../api';
import { MAX_BULK_PING, bulkPingTimeoutMs, pingTargets } from '../deviceCsv';
import { asArray } from '../format';
import { getLang, t } from '../i18n';
import type { BulkPingResponse, InventoryRow } from '../types';

export function GroupActions({ group, locked, wall, busy, devices, run, refresh, notice }: {
  group: string; locked: boolean; wall: boolean; busy: boolean; devices: InventoryRow[];
  run: (action: () => Promise<void>) => void; refresh: () => Promise<unknown>; notice: (value: string) => void;
}) {
  if (wall) return null;
  const label = group === '*' ? t('rack.all') : group || t('rack.ungrouped');
  const csv = `/api/devices/export.csv?scope=all&lang=${getLang()}${group === '*' ? '' : `&group=${encodeURIComponent(group)}`}`;
  return <details className="group-menu" data-action="group-menu"><summary>{t('rack.actions')} ▾</summary>
    <div className="group-menu-items">
      {!locked && <>
        {[15, 30, 60, 120].map(minutes => <button key={minutes} disabled={busy} onClick={() => run(async () => {
          await api('/api/work', json('POST', { group, minutes })); await refresh();
        })}>{t('rack.startWork', { minutes })}</button>)}
        <button title={group === '*' ? t('rack.endWorkNote') : undefined} disabled={busy} onClick={() => run(async () => {
          await api(`/api/work/${encodeURIComponent(group)}`, json('DELETE')); await refresh();
        })}>{t('rack.endWork')}</button>
        <button disabled={busy} onClick={() => run(async () => {
          const targets = pingTargets(group === '*' ? devices : devices.filter(device => device.group_tag === group));
          if (!targets.length) { notice(t('rack.noTargets')); return; }
          let ok = 0;
          for (let offset = 0; offset < targets.length; offset += MAX_BULK_PING) {
            const ips = targets.slice(offset, offset + MAX_BULK_PING);
            const result = await api<BulkPingResponse>('/api/diagnostics/ping-bulk', json('POST', { ips }), bulkPingTimeoutMs(ips.length));
            ok += result.ok;
          }
          notice(t('rack.pingResult', { ok, total: targets.length })); await refresh();
        })}>{t('rack.ping')}</button>
        <button disabled={busy} onClick={() => {
          if (window.confirm(t('rack.resetConfirm', { group: label }))) run(async () => {
            const result = await api<{ results: { switch_id: number; ok: boolean; error?: string }[] }>('/api/counters/reset', json('POST', { group }));
            const rows = asArray(result.results);
            notice(`${t('rack.resetResult', { ok: rows.filter(row => row.ok).length, total: rows.length })}${rows.some(row => !row.ok) ? ` · ${rows.filter(row => !row.ok).map(row => `#${row.switch_id}: ${row.error || t('rack.offline')}`).join(', ')}` : ''}`);
            await refresh();
          });
        }}>{t('rack.reset')}</button>
      </>}
      <a href={csv} download>{t('rack.csv')}</a>
    </div>
  </details>;
}
