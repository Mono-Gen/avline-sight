import { api, json } from '../api';
import { MAX_BULK_PING, bulkPingTimeoutMs, pingLabel, pingTargets } from '../deviceCsv';
import { asArray } from '../format';
import { groupRequest } from '../groupRequest';
import { t } from '../i18n';
import { pcTimestamp } from '../pcTime';
import type { BulkPingResponse, BulkPingResult, InventoryRow, Phase, PingStatus } from '../types';

export interface PingNoReply { ip: string; names: string[]; status: PingStatus; }
// Kept by the caller per group until closed: while running, then the result with the addresses that did not answer.
export type PingOutcome = { running: number } | { ok: number; total: number; noReply: PingNoReply[]; at: string };

// Names of the devices behind each address, so "no reply" says which device, not only an IP.
export function noReplyRows(results: readonly BulkPingResult[], devices: readonly InventoryRow[]): PingNoReply[] {
  return results.filter(result => result.status !== 'ok').map(result => ({
    ip: result.ip, status: result.status,
    names: [...new Set(devices.filter(device => (device.ip || device.planned_ip) === result.ip)
      .map(device => device.label || device.lldp_name || device.vendor || device.mac).filter(Boolean))],
  }));
}

// No hooks: rendered by the caller next to the group's actions.
export function PingOutcomeView({ outcome, close, openDevices }: { outcome: PingOutcome; close: () => void; openDevices?: () => void }) {
  if ('running' in outcome) return <div className="ping-outcome" role="status">{t('pingOutcome.running', { count: outcome.running })}</div>;
  const failed = outcome.noReply.length > 0;
  return <div className={failed ? 'ping-outcome ping-outcome-failed' : 'ping-outcome'} role="status">
    <div className="ping-outcome-head">
      <strong>{t('pingOutcome.summary', { ok: outcome.ok, total: outcome.total })}</strong>
      <span className="muted">{pcTimestamp(outcome.at)}</span>
      {openDevices && <button type="button" data-action="open-devices" onClick={openDevices}>{t('pingOutcome.openDevices')}</button>}
      <button type="button" onClick={close}>{t('rack.close')}</button>
    </div>
    {failed && <ul>{outcome.noReply.map(row => <li key={row.ip}>
      <span>{pingLabel(row.status, 0)}</span> <strong>{row.names.join(', ') || t('operations.unknownDevice')}</strong> <span className="muted">{row.ip}</span>
    </li>)}</ul>}
  </div>;
}

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

export function GroupActions({ group, all = false, profileId, locked, wall, busy, devices, run, refresh, notice, replay, phase, onPing }: {
  group: string; locked: boolean; wall: boolean; busy: boolean; devices: InventoryRow[];
  all?: boolean; profileId?: number; phase?: Phase;
  run: Run; refresh: () => Promise<unknown>; notice: (value: string) => void;
  // Progress and result of "Ping all"; without it the result goes to notice as one line.
  onPing?: (outcome: PingOutcome | undefined) => void;
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
      const results: BulkPingResult[] = [];
      onPing?.({ running: targets.length });
      try {
        for (let offset = 0; offset < targets.length; offset += MAX_BULK_PING) {
          const ips = targets.slice(offset, offset + MAX_BULK_PING);
          const result = await api<BulkPingResponse>('/api/diagnostics/ping-bulk', json('POST', { ips }), bulkPingTimeoutMs(ips.length));
          ok += result.ok;
          results.push(...asArray(result.results));
        }
      } catch (error) {
        // The error itself is shown by run; drop the "running" line.
        onPing?.(undefined);
        throw error;
      }
      if (onPing) onPing({ ok, total: targets.length, noReply: noReplyRows(results, devices), at: new Date().toISOString() });
      else notice(t('rack.pingResult', { ok, total: targets.length }));
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
