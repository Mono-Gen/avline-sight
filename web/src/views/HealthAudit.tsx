import { useEffect, useState } from 'react';
import { api, json } from '../api';
import { Chart, Table } from '../components/Common';
import { asArray } from '../format';
import type { Audit, MetricResult } from '../types';
import type { ViewProps } from './props';

export function HealthAudit(p: ViewProps) {
  const [audit, setAudit] = useState<Audit>();
  const [bases, setBases] = useState<{
    id: number;
    name: string;
    captured_ts: string;
  }[]>([]);
  const [diff, setDiff] = useState<object[]>([]);
  const [metrics, setMetrics] = useState<MetricResult>();
  const load = () => api<typeof bases>('/api/baselines').then(setBases);
  useEffect(() => {
    api<Audit>('/api/audit').then(setAudit).catch(() => {
    });
    void load().catch(() => undefined);
  }, []);
  useEffect(() => {
    if (p.selected && !p.replay) api<MetricResult>(`/api/metrics/switch?switch_id=${p.selected}`).then(setMetrics).catch(() => setMetrics(undefined));
  }, [p.selected, p.replay]);
  const state = p.replay ? asArray(p.replay).find(s => s.id === p.selected) : p.state;
  const health = state?.health;
  const summary = health ? {
    ...health,
    fans: undefined,
    psus: undefined,
    processes: undefined
  } : undefined;
  return <section className="panel space-y-4">
    <select value={p.selected} onChange={e => p.select(Number(e.target.value))}>
      <option value="0">スイッチを選択
      </option>
      {asArray((p.replay || p.overview.switches || [])).map(s => <option key={s.id} value={s.id}>
        {s.name}
      </option>)}
    </select>
    {state && <>
      <h2>筐体ヘルス / PoE
      </h2>
      <Table
        rows={summary ? [Object.fromEntries(Object.entries(summary).filter(([, value]) => value !== undefined))] : []}
      />
      <h2>ファン</h2>
      <Table rows={asArray(health?.fans)} />
      <h2>電源</h2>
      <Table rows={asArray(health?.psus)} />
      <h2>プロセス</h2>
      <Table rows={asArray(health?.processes)} />
      <h2>SFP
      </h2>
      <Table
        rows={asArray((state.ports || [])).filter(x => x.sfp).map(x => ({
          port: x.name,
          ...x.sfp
        }))}
      />
    </>}
    {!p.replay && <>
      <Chart
        points={metrics?.points || []}
        keys={['cpu_5s', 'mem_free_pct', 'mbuf_used_pct', 'temp_max_c', 'poe_used_w']}
      />
      <h2>監査
      </h2>
      <Table rows={audit?.findings || []} />
      <h2>ファームウェア
      </h2>
      <Table
        rows={asArray(Object.entries(audit?.fw_versions || {})).map(([version, switches]) => ({
          version,
          switches
        }))}
      />
      <h2>ベースライン
      </h2>
      <button
        disabled={p.locked || p.busy}
        onClick={() => {
          const name = prompt('ベースライン名');
          if (name) p.run(async () => {
            await api('/api/baselines', json('POST', { name }));
            await load();
          });
        }}
      >作成
      </button>
      {asArray((bases || [])).map(b => <div key={b.id}>
        {b.name}

        {b.captured_ts}

        <button
          onClick={() => p.run(async () => {
            const r = await api<{
              diffs: object[];
            }>(`/api/baselines/${b.id}/diff`);
            setDiff(r.diffs || []);
          })}
        >差分
        </button>

        <button
          disabled={p.locked || p.busy}
          onClick={() => {
            if (confirm(`${b.name} を削除しますか？`)) p.run(async () => {
              await api(`/api/baselines/${b.id}`, json('DELETE'));
              await load();
            });
          }}
        >削除
        </button>
      </div>)}
      <Table rows={diff} />
    </>}
  </section>;
}
