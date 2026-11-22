import { api, json } from '../api';
import { MAX_BULK_PING, bulkPingTimeoutMs, deviceUrl, pingTargets } from '../deviceCsv';
import { asArray } from '../format';
import { groupRequest } from '../groupRequest';
import { t } from '../i18n';
import type { BulkPingResponse, InventoryRow } from '../types';

interface ResetResult { switch_id: number; ok: boolean; }
type Run = (action: () => Promise<void>) => void;

export function resetGroup(group: string, label: string, run: Run, afterReset: (results: ResetResult[]) => Promise<void>, profileId?: number) {
  if (!window.confirm(t('rack.resetConfirm', { group: label }))) return;
  run(async () => {
    const result = await api<{ results: ResetResult[] }>('/api/counters/reset', groupRequest(profileId, 'POST', { group }));
    await afterReset(asArray(result.results));
  });
}

export function GroupActions({ group, all = false, profileId, locked, wall, busy, devices, run, refresh, notice, collapse, collapseLabel, onEdit, replay }: {
  group: string; locked: boolean; wall: boolean; busy: boolean; devices: InventoryRow[];
  all?: boolean; profileId?: number;
  run: Run; refresh: () => Promise<unknown>; notice: (value: string) => void;
  collapse: () => void; collapseLabel: string; onEdit: () => void; replay: boolean;
}) {
  if (wall) return null;
  const label = all ? t('rack.all') : group || t('rack.ungrouped');
  const target = all ? '*' : group;
  const legacyReserved = !all && ['*', '.', '..'].includes(group);
  const csv = deviceUrl({ scope: 'all', q: '', vlans: [], group: all ? undefined : group }, undefined, true);
  const ping = () => run(async () => {
    const targets = pingTargets(all ? devices : devices.filter(device => device.group_tag === group));
    if (!targets.length) {
      notice(t('rack.noTargets'));
      return;
    }
    let ok = 0;
    for (let offset = 0; offset < targets.length; offset += MAX_BULK_PING) {
      const ips = targets.slice(offset, offset + MAX_BULK_PING);
      const result = await api<BulkPingResponse>('/api/diagnostics/ping-bulk', json('POST', { ips }), bulkPingTimeoutMs(ips.length));
      ok += result.ok;
    }
    notice(t('rack.pingResult', { ok, total: targets.length }));
    await refresh();
  });

  return <details className="group-menu" data-action="group-menu">
    <summary aria-label={`${label} · ${t('rack.actions')}`}>{t('rack.actions')} ▾</summary>
    <div className="group-menu-items">
      {!locked && !legacyReserved && <>
        {[15, 30, 60, 120].map(minutes => <button key={minutes} data-action="work" disabled={busy} onClick={() => run(async () => {
          await api('/api/work', groupRequest(profileId, 'POST', { group: target, minutes }));
          await refresh();
        })}>{t('rack.startWork', { minutes })}</button>)}
        <button data-action="work" title={all ? t('rack.endWorkNote') : undefined} disabled={busy} onClick={() => run(async () => {
          await api(`/api/work/${encodeURIComponent(target)}`, groupRequest(profileId, 'DELETE'));
          await refresh();
        })}>{t('rack.endWork')}</button>
        <button data-action="ping" disabled={busy} onClick={ping}>{t('rack.ping')}</button>
        <button data-action="reset" disabled={busy} onClick={() => resetGroup(target, label, run, async rows => {
          const failed = rows.filter(row => !row.ok);
          const summary = t('rack.resetResult', { ok: rows.length - failed.length, total: rows.length });
          notice(failed.length ? `⚠ ${summary} · ${failed.map(row => `#${row.switch_id}`).join(', ')}` : summary);
          await refresh();
        }, profileId)}>{t('rack.reset')}</button>
      </>}
      {!locked && <button data-action="edit-group" disabled={busy} onClick={onEdit}>{t(all ? 'operations.order' : 'operations.rename')}</button>}
      {!replay && !legacyReserved && <a href={csv} download>{t('rack.csv')}</a>}
      <button onClick={collapse}>{collapseLabel}</button>
    </div>
  </details>;
}
