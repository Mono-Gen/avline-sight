import { getLang, t } from './i18n';
import { jstTime, pcTimestamp } from './pcTime';
import type { DeviceAliasImportResponse, InventoryRow, PingStatus, ReplayRef } from './types';

export type DeviceScope = 'all' | 'unnamed';

interface DeviceFilters {
  scope: DeviceScope;
  q: string;
  vlans: readonly number[];
  group?: string;
}

export function deviceUrl(filters: DeviceFilters, replay?: ReplayRef, download = false): string {
  const base = replay ? `/api/replay/${encodeURIComponent(replay.id)}/devices` : '/api/devices';
  const params = new URLSearchParams();
  if (replay) params.set('ts', replay.ts);
  params.set('scope', filters.scope);
  if (filters.q.trim()) params.set('q', filters.q.trim());
  if (filters.vlans.length) params.set('vlans', filters.vlans.join(','));
  if (filters.group !== undefined) params.set('group', filters.group);
  if (download) params.set('lang', getLang());
  return `${base}${download ? '/export.csv' : ''}?${params}`;
}

function csvCell(value: string | number | undefined) {
  let text = String(value ?? '');
  if (/^ *[\t\r\n']/.test(text) || (/^ *[=+\-@]/.test(text) && !/^[+-]?\d+(?:\.\d+)?$/.test(text))) text = "'" + text;
  return /[",\r\n]|^\s/.test(text) ? '"' + text.replace(/"/g, '""') + '"' : text;
}

export function visibleDeviceCsv(rows: InventoryRow[]) {
  const records = rows.map(row => {
    const lastSeen = row.last_seen ? pcTimestamp(row.last_seen, true) : '—';
    const last = lastSeen === '—' ? t('deviceCsv.notDetected')
      : row.seen ? lastSeen : `${lastSeen} (${t('deviceCsv.notDetected')})`;
    const source = row.via_uplink ? `${row.source} (${t('ui.m223')})` : row.source;
    const ping = row.ping && `${pingLabel(row.ping.status, row.ping.rtt_ms)} (${jstTime(row.ping.at)})`;
    const speed = row.speed_mbps >= 1000 && row.speed_mbps % 1000 === 0 ? `${row.speed_mbps / 1000}G`
      : row.speed_mbps > 0 ? `${row.speed_mbps}M` : '';
    return [row.mac, row.label, row.note, row.planned_ip, row.ip, row.vendor, row.lldp_name, row.switch_name,
      row.port, row.vlan || '', row.poe_w?.toFixed(1), speed, last, source, ping].map(csvCell).join(',');
  });
  return new Blob(['\uFEFF', t('deviceCsv.header'), '\r\n', ...records.flatMap(record => [record, '\r\n'])], { type: 'text/csv;charset=utf-8' });
}

export function decodeDeviceCsv(bytes: Uint8Array): {
  csv: string;
  encoding: string | 'UTF-8' | 'Shift_JIS';
} {
  if (bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf) {
    return { csv: new TextDecoder('utf-8').decode(bytes), encoding: t('ui.m205') };
  }
  try {
    return { csv: new TextDecoder('utf-8', { fatal: true }).decode(bytes), encoding: 'UTF-8' };
  } catch {
    return { csv: new TextDecoder('shift_jis').decode(bytes), encoding: 'Shift_JIS' };
  }
}

export function importSummary(response: DeviceAliasImportResponse): string {
  const { added, updated, unchanged, deleted } = response.result;
  return t('ui.m206', { p0: added, p1: updated, p2: unchanged, p3: deleted, p4: response.skipped_empty });
}


export const MAX_BULK_PING = 256;


export function bulkPingTimeoutMs(count: number): number {
  return Math.ceil(Math.max(count, 1) / 16) * 2000 + 10000;
}


export function pingTargets(rows: readonly InventoryRow[]): string[] {
  const seen = new Set<string>();
  for (const row of rows) {
    const ip = row.ip || row.planned_ip;
    if (ip) seen.add(ip);
  }
  return [...seen];
}


export function pingLabel(status: PingStatus, rttMs: number): string {
  switch (status) {
    case 'ok': return t('ui.m207', { p0: Math.round(rttMs) });
    case 'timeout': return t('ui.m208');
    case 'unreachable': return t('ui.m209');
    default: return t('ui.m210');
  }
}
