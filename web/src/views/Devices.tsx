import { useState } from 'react';
import { api, json } from '../api';
import { Badge } from '../components/Badge';
import { DeviceCsvImportDialog } from '../components/DeviceCsvImportDialog';
import { PollingStatus } from '../components/PollingStatus';
import { bulkPingTimeoutMs, deviceUrl, importSummary, MAX_BULK_PING, pingLabel, pingTargets } from '../deviceCsv';
import type { DeviceScope } from '../deviceCsv';
import { asArray, decimal, formatTime } from '../format';
import type { BulkPingResult, BulkPingResponse, InventoryRow } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';

const columns = ['名称', 'MAC', 'IP', 'Ping', 'メーカー', 'LLDP名', 'スイッチ', 'ポート', 'VLAN', 'PoE(W)', '速度', '最終確認', '検出元'];
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
              <td className="whitespace-nowrap" title={row.ping ? `実行: ${formatTime(row.ping.at)}` : undefined}>
                {row.ping ? pingLabel(row.ping.status, row.ping.rtt_ms) : emptyCell}
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
  const [manual, setManual] = useState('');
  const [manualResults, setManualResults] = useState<BulkPingResult[]>([]);
  const targets = pingTargets(rows);
  const pingDisabledReason = isReplay ? 'オフライン再生中はPingできません。'
    : targets.length === 0 ? 'IPアドレスの分かる機器が表示されていません。'
      : targets.length > MAX_BULK_PING ? `一度に${MAX_BULK_PING}件までです。検索やVLANで絞り込んでください。` : undefined;
  const manualDisabledReason = isReplay ? 'オフライン再生中はPingできません。'
    : p.locked ? '閲覧専用ロック中は手入力のPingはできません（一覧の機器へのPingは可能です）。' : undefined;
  const runPing = (body: { ips?: string[]; manual?: string }) => p.run(async () => {
    setNotice('');
    // 手入力は展開後の件数が画面側では分からないため、上限件数で待つ
    const r = await api<BulkPingResponse>('/api/diagnostics/ping-bulk', json('POST', body),
      bulkPingTimeoutMs(body.ips ? body.ips.length : MAX_BULK_PING));
    const listed = new Set(rows.map(row => row.ip));
    // 一覧にないIP（手入力）の結果だけ別表に出す。一覧の行は再取得した ping 列に反映される
    setManualResults(asArray(r.results).filter(x => !listed.has(x.ip)));
    setNotice(`Ping完了: 応答 ${r.ok} / ${r.total}件（${(r.elapsed_ms / 1000).toFixed(1)}秒）`);
    setRevision(value => value + 1);
  });
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
      {!isReplay && (
        <div className="flex flex-wrap items-center gap-3">
          <button disabled={Boolean(pingDisabledReason) || p.busy} title={pingDisabledReason}
            onClick={() => runPing({ ips: targets })}>表示中の機器にPing（{targets.length}件）</button>
          <label>手入力 <input
            value={manual}
            placeholder="192.168.1.10, 192.168.1.20-40, 10.0.0.0/28"
            disabled={Boolean(manualDisabledReason)}
            onChange={event => setManual(event.target.value)}
          /></label>
          <button disabled={Boolean(manualDisabledReason) || !manual.trim() || p.busy} title={manualDisabledReason} onClick={() => {
            if (window.confirm(`次の宛先にPingを送ります（1回${MAX_BULK_PING}件まで）。
${manual.trim()}`)) runPing({ manual });
          }}>手入力のIPにPing</button>
          {(pingDisabledReason || manualDisabledReason) && (
            <span className="text-sm text-slate-500 dark:text-slate-400">{manualDisabledReason || pingDisabledReason}</span>
          )}
        </div>
      )}
      <p className="text-sm text-slate-500 dark:text-slate-400">「無応答」は故障とは限りません（監視PCから経路がない、機器がPingに応答しない設定など）。</p>
      {notice && <p role="status">✓ {notice}</p>}
      {manualResults.length > 0 && (
        <div className="overflow-auto">
          <h3 className="font-bold">一覧にないIPの結果 {manualResults.length}件</h3>
          <table className="text-sm">
            <thead><tr><th scope="col">IP</th><th scope="col">結果</th><th scope="col">詳細</th></tr></thead>
            <tbody>
              {manualResults.map(x => <tr key={x.ip}><td className="font-mono">{x.ip}</td><td>{pingLabel(x.status, x.rtt_ms)}</td><td>{x.detail || emptyCell}</td></tr>)}
            </tbody>
          </table>
        </div>
      )}
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
