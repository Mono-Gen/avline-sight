import type { DeviceAliasImportResponse, InventoryRow, PingStatus, ReplayRef } from './types';

export type DeviceScope = 'all' | 'unnamed';

interface DeviceFilters {
  scope: DeviceScope;
  q: string;
  vlans: readonly number[];
}

export function deviceUrl(filters: DeviceFilters, replay?: ReplayRef, download = false): string {
  const base = replay ? `/api/replay/${encodeURIComponent(replay.id)}/devices` : '/api/devices';
  const params = new URLSearchParams();
  if (replay) params.set('ts', replay.ts);
  params.set('scope', filters.scope);
  if (filters.q.trim()) params.set('q', filters.q.trim());
  if (filters.vlans.length) params.set('vlans', filters.vlans.join(','));
  return `${base}${download ? '/export.csv' : ''}?${params}`;
}

export function decodeDeviceCsv(bytes: Uint8Array): {
  csv: string;
  encoding: 'UTF-8 (BOM付き)' | 'UTF-8' | 'Shift_JIS';
} {
  // BOMがあるときは明示された文字コードを優先し、別の文字コードへ読み替えない。
  if (bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf) {
    return { csv: new TextDecoder('utf-8').decode(bytes), encoding: 'UTF-8 (BOM付き)' };
  }
  try {
    return { csv: new TextDecoder('utf-8', { fatal: true }).decode(bytes), encoding: 'UTF-8' };
  } catch {
    return { csv: new TextDecoder('shift_jis').decode(bytes), encoding: 'Shift_JIS' };
  }
}

export function importSummary(response: DeviceAliasImportResponse): string {
  const { added, updated, unchanged, deleted } = response.result;
  return `追加 ${added}件 / 更新 ${updated}件 / 変更なし ${unchanged}件 / 削除 ${deleted}件 / 名称空欄で無視 ${response.skipped_empty}件`;
}

/** 一括Pingの上限（サーバーの maxBulkPing と同じ）。 */
export const MAX_BULK_PING = 256;

/**
 * 一括Pingの応答待ち上限。サーバーは16並列・1宛先最大2回×1秒なので、全宛先が無応答だと
 * ceil(件数/16)×2秒かかる（256件で32秒）。それに通信の余裕10秒を足す。
 */
export function bulkPingTimeoutMs(count: number): number {
  return Math.ceil(Math.max(count, 1) / 16) * 2000 + 10000;
}

/** 表示中の行から、Pingを打てるIP（重複なし）を集める。過去のIPも含める（電源断・抜線の切り分けに使うため）。 */
export function pingTargets(rows: readonly InventoryRow[]): string[] {
  const seen = new Set<string>();
  for (const row of rows) {
    if (row.ip && !seen.has(row.ip)) seen.add(row.ip);
  }
  return [...seen];
}

/** Ping結果の表示文言。「無応答」は故障を意味しない（経路なし・ICMP遮断もある）ので断定しない。 */
export function pingLabel(status: PingStatus, rttMs: number): string {
  switch (status) {
    case 'ok': return `✓ 応答 ${Math.round(rttMs)}ms`;
    case 'timeout': return '⚠ 無応答';
    case 'unreachable': return '⚠ 到達不能';
    default: return '✕ 送信失敗';
  }
}
