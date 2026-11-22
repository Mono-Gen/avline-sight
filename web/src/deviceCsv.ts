import { getLang, t } from './i18n';
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
