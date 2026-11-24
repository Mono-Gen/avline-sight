import { api, json } from '../api';
import { MAX_BULK_PING, bulkPingTimeoutMs, pingTargets } from '../deviceCsv';
import { asArray } from '../format';
import { groupRequest } from '../groupRequest';
import { t } from '../i18n';
import type { BulkPingResponse, InventoryRow, Phase } from '../types';

interface ResetResult { switch_id: number; ok: boolean; }
type Run = (action: () => Promise<void>) => void;

export function confirmCounterReset(message: string, phase?: Phase) {
  return window.confirm(phase === 'show' ? `${message}\n${t('pages.resetLoad')}` : message);
}

export function confirmBulkPing(count: number, phase?: Phase) {
  return phase !== 'show' || window.confirm(t('pages.pingConfirm', { count }));
}

export function resetGroup(group: string, label: string, run: Run, afterReset: (results: ResetResult[]) => Promise<void>, profileId?: number, phase?: Phase) {
  if (!confirmCounterReset(t('rack.resetConfirm', { group: label }), phase)) return;
  run(async () => {
    const result = await api<{ results: ResetResult[] }>('/api/counters/reset', groupRequest(profileId, 'POST', { group }));
    await afterReset(asArray(result.results));
  });
}

export function GroupActions({ group, all = false, profileId, locked, wall, busy, devices, run, refresh, notice, replay, phase }: {
  group: string; locked: boolean; wall: boolean; busy: boolean; devices: InventoryRow[];
  all?: boolean; profileId?: number; phase?: Phase;
  run: Run; refresh: () => Promise<unknown>; notice: (value: string) => void;
  replay: boolean;
}) {
  if (wall) return null;
  const label = all ? t('rack.all') : group || t('rack.ungrouped');
  const target = all ? '*' : group;
  const legacyReserved = !all && ['*', '.', '..'].includes(group);
  const ping = () => {
    const targets = pingTargets(all ? devices : devices.filter(device => device.group_tag === group));
    if (!targets.length) {
      notice(t('rack.noTargets'));
      return;
    }
    if (!confirmBulkPing(targets.length, phase)) return;
    run(async () => {
      let ok = 0;
      for (let offset = 0; offset < targets.length; offset += MAX_BULK_PING) {
        const ips = targets.slice(offset, offset + MAX_BULK_PING);
        const result = await api<BulkPingResponse>('/api/diagnostics/ping-bulk', json('POST', { ips }), bulkPingTimeoutMs(ips.length));
        ok += result.ok;
      }
      notice(t('rack.pingResult', { ok, total: targets.length }));
      await refresh();
    });
  };

  return <div className="group-actions" aria-label={label}>
    <button data-action="ping" disabled={locked || busy || replay || legacyReserved} onClick={ping}>{t('rack.ping')}</button>
    <button data-action="reset" disabled={locked || busy || replay || legacyReserved} onClick={() => resetGroup(target, label, run, async rows => {
      const failed = rows.filter(row => !row.ok);
      const summary = t('rack.resetResult', { ok: rows.length - failed.length, total: rows.length });
      notice(failed.length ? `⚠ ${summary} · ${failed.map(row => `#${row.switch_id}`).join(', ')}` : summary);
      await refresh();
    }, profileId, phase)}>{t('rack.reset')}</button>
  </div>;
}
