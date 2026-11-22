import { useState } from 'react';
import { api } from '../api';
import { Badge } from '../components/Badge';
import { DeviceCsvImportDialog } from '../components/DeviceCsvImportDialog';
import { PollingStatus } from '../components/PollingStatus';
import { deviceUrl, importSummary } from '../deviceCsv';
import type { DeviceScope } from '../deviceCsv';
import { asArray, decimal, formatTime } from '../format';
import type { InventoryRow } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';

const columns = ['名称', 'MAC', 'IP', 'メーカー', 'LLDP名', 'スイッチ', 'ポート', 'VLAN', 'PoE(W)', '速度', '最終確認', '検出元'];
const emptyCell = '—';

function lastSeen(value: string | undefined): string {
  // Goのゼロ時刻はJSONでは省略されないため、観測記録なしとして扱う。
  return value && value !== '0001-01-01T00:00:00Z' ? formatTime(value) : '記録なし';
}

export function DeviceInventoryTable({ rows }: { rows: InventoryRow[] }) {
  return (
    <div className="overflow-auto">
      <table className="w-full text-sm">
        <thead><tr>{columns.map(column => <th key={column} scope="col" className="whitespace-nowrap">{column}</th>)}</tr></thead>
        <tbody>
          {asArray(rows).map(row => (
            <tr key={row.mac}>
              <td>
                {row.label || <span className="text-slate-500 dark:text-slate-400">（名前なし）</span>}
                {!row.seen && <span className="ml-2 whitespace-nowrap"><Badge text="○ 未検出" /></span>}
              </td>
              <td className="whitespace-nowrap font-mono">{row.mac}</td>
              <td>
                {row.ip || emptyCell}
                {row.ip && row.ip_stale && (
                  <span className="ml-2 whitespace-nowrap" title="最終観測時のIPアドレスです。">
                    <Badge text="◷ 過去のIP" />
                  </span>
                )}
              </td>
              <td>{row.vendor || emptyCell}</td>
              <td>{row.lldp_name || emptyCell}</td>
              <td>{row.switch_name || emptyCell}</td>
              <td>{row.port || emptyCell}</td>
              <td>{row.vlan || emptyCell}</td>
              <td>{decimal(row.poe_w)}</td>
              <td className="whitespace-nowrap">{row.speed_mbps > 0 ? `${row.speed_mbps} Mbps` : emptyCell}</td>
              <td className="whitespace-nowrap">{lastSeen(row.last_seen)}</td>
              <td>
                {row.source || emptyCell}
                {row.via_uplink && (
                  <span className="ml-2 whitespace-nowrap" title="スイッチ間リンクの観測から推定した所在です。">
                    <Badge text="↪ 経路上" />
                  </span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Devices(p: ViewProps) {
  const [scope, setScope] = useState<DeviceScope>('all');
  const [q, setQuery] = useState('');
  const [importOpen, setImportOpen] = useState(false);
  const [notice, setNotice] = useState('');
  const [revision, setRevision] = useState(0);
  const preset = asArray(p.settings?.views).find(view => view.id === p.part);
  const vlans = asArray(preset?.vlans);
  const isReplay = p.replay !== null;
  const replay = p.replayRef;
  const enabled = !isReplay || Boolean(replay);
  const filters = { scope, q, vlans };
  const url = enabled ? deviceUrl(filters, replay) : '';
  const profileId = p.status?.profile.id;
  // 表示条件が変わった直後も、前のプロファイルや再生時刻の行を表示しない。
  const key = JSON.stringify([url, profileId, revision]);
  const inventory = usePolling(async signal => ({ key, rows: await api<InventoryRow[]>(url, { signal }) }),
    isReplay ? 0 : STATE_REFRESH_MS, [key], enabled, 300);
  const rows = inventory.data?.key === key ? asArray(inventory.data.rows) : [];
  const importDisabledReason = isReplay ? 'オフライン再生中はCSVを読み込めません。'
    : p.locked ? '閲覧専用ロック中はCSVを読み込めません。' : undefined;

  return (
    <section className="panel space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div role="group" aria-label="表示範囲" className="flex gap-1">
          <button aria-pressed={scope === 'all'} onClick={() => setScope('all')}>すべて</button>
          <button aria-pressed={scope === 'unnamed'} onClick={() => setScope('unnamed')}>名前なしのみ</button>
        </div>
        <label>検索 <input type="search" value={q} onChange={event => setQuery(event.target.value)} /></label>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        {enabled && <>
          <a className="underline" href={deviceUrl({ ...filters, scope: 'all' }, replay, true)} download>一覧をCSV書出</a>
          <a className="underline" href={deviceUrl({ ...filters, scope: 'unnamed' }, replay, true)} download>名前なし雛形をCSV書出</a>
        </>}
        <button disabled={Boolean(importDisabledReason) || p.busy} title={importDisabledReason} onClick={() => {
          setNotice('');
          setImportOpen(true);
        }}>CSV読込</button>
        {importDisabledReason && <span className="text-sm text-slate-500 dark:text-slate-400">{importDisabledReason}</span>}
      </div>
      {notice && <p role="status">✓ {notice}</p>}
      {enabled ? <PollingStatus {...inventory} /> : <p role="status">再生時刻を取得中…</p>}
      {inventory.data?.key === key && rows.length === 0 && <p>該当する接続機器はありません。</p>}
      <DeviceInventoryTable rows={rows} />
      {importOpen && !importDisabledReason && (
        <DeviceCsvImportDialog
          key={profileId}
          busy={p.busy}
          run={p.run}
          close={() => setImportOpen(false)}
          onImported={result => {
            setImportOpen(false);
            setNotice(`機器名CSVを反映しました。${importSummary(result)}`);
            // 確定前のポーリング応答を無効にし、反映後の一覧を取得する。
            setRevision(value => value + 1);
          }}
        />
      )}
    </section>
  );
}
