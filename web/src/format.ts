// 空配列と不正な応答を同じ安全な描画経路へ通し、監視画面全体の停止を防ぐ。
export function asArray<T>(value: T): (T extends readonly (infer Item)[] ? NonNullable<Item> : never)[] {
  return (Array.isArray(value) ? value.filter(item => item != null) : []) as (T extends readonly (infer Item)[] ? NonNullable<Item> : never)[];
}

export function decimal(value: unknown, digits = 1): string {
  return typeof value === 'number' && Number.isFinite(value)
    ? value.toLocaleString('ja-JP', {
      minimumFractionDigits: digits,
      maximumFractionDigits: digits
    })
    : '—';
}

export function formatBps(value: number): string {
  if (!Number.isFinite(value)) return '—';
  const magnitude = Math.abs(value);
  if (magnitude >= 1e9) return `${decimal(value / 1e9)} Gbps`;
  if (magnitude >= 1e6) return `${decimal(value / 1e6)} Mbps`;
  if (magnitude >= 1e3) return `${decimal(value / 1e3)} kbps`;
  return `${decimal(value)} bps`;
}

export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—';
  const minutes = Math.floor(seconds / 60);
  return `${Math.floor(minutes / 1440)}日 ${Math.floor(minutes / 60) % 24}時間 ${minutes % 60}分`;
}

export function resolveSwitchId(switches: readonly {
  id: number;
}[], selected: number): number {
  const list = asArray(switches);
  return list.some(item => item.id === selected) ? selected : list[0]?.id || 0;
}

export function labSwitchIndex(name: string): number | null {
  const match = name.match(/^AV-SW-(\d+)$/i);
  const number = match ? Number(match[1]) : 0;
  return number > 0 && Number.isSafeInteger(number) ? number - 1 : null;
}

export function alertCount(counts: object | null | undefined, severity: string): number {
  // 実バックエンドの大文字キーとAPI文書の小文字キーの両方を受け入れる。
  const values = (counts || {}) as Record<string, unknown>;
  const value = values[severity.toLowerCase()] ?? values[severity.toUpperCase()];
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

// 仕様2.3-3: 保存はUTC、表示は監視PCのローカル時刻とする
export function formatTime(value: unknown): string {
  if (typeof value !== 'string' || value === '') return '—';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString('ja-JP', { hour12: false });
}
