import type { DeviceAliasImportResponse, ReplayRef } from './types';

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
