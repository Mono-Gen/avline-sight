import { t } from '../i18n';
import { useState } from 'react';
import { api, json } from '../api';
import { Chart, Table } from '../components/Common';
import { PollingStatus } from '../components/PollingStatus';
import { asArray, formatTime } from '../format';
import type { Audit, MetricResult } from '../types';
import { METRICS_REFRESH_MS, TOPOLOGY_REFRESH_MS, usePolling } from '../usePolling';
import { useSwitchState } from '../useSwitchState';
import type { ViewProps } from './props';

export function HealthAudit(p: ViewProps) {
  const [diff, setDiff] = useState<object[]>([]);
  const report = usePolling(async signal => {
    const [audit, baselines] = await Promise.all([
      api<Audit>('/api/audit', { signal }),
      api<{
        id: number;
        name: string;
        captured_ts: string
      }[]>('/api/baselines', { signal }),
    ]);
    return { audit, baselines };
  }, TOPOLOGY_REFRESH_MS, [], !p.replay);
  const metrics = usePolling(signal => api<MetricResult>(`/api/metrics/switch?switch_id=${p.selected}`, { signal }),
    METRICS_REFRESH_MS, [p.selected], !p.replay && p.selected > 0);
  const { state, polling: statePolling } = useSwitchState(p.selected, p.state, p.replay);
  const audit = report.data?.audit;
  const bases = report.data?.baselines;
  const load = report.refresh;
  const health = state?.health;
  const summary = health ? {
    ...health,
    fans: undefined,
    psus: undefined,
    processes: undefined
  } : undefined;
  return <section className="panel space-y-4">
    <select value={p.selected} onChange={e => p.select(Number(e.target.value))}>
      <option value="0">{t('ui.m254')}</option>
      {asArray((p.replay || p.overview.switches || [])).map(s => <option key={s.id} value={s.id}>
        {s.name}
      </option>)}
    </select>
    {!p.replay && <PollingStatus {...statePolling} />}
    {state && <>
      <h2>{t('ui.m255')}</h2>
      <Table
        rows={summary ? [Object.fromEntries(Object.entries(summary).filter(([, value]) => value !== undefined))] : []}
      />
      <h2>{t('ui.m059')}</h2>
      <Table rows={asArray(health?.fans)} />
      <h2>{t('ui.m060')}</h2>
      <Table rows={asArray(health?.psus)} />
      <h2>{t('ui.m061')}</h2>
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
        points={metrics.data?.points || []}
        keys={['cpu_5s', 'mem_free_pct', 'mbuf_used_pct', 'temp_max_c', 'poe_used_w']}
      />
      <PollingStatus {...metrics} />
      <PollingStatus {...report} />
      <h2>{t('ui.m256')}</h2>
      <Table rows={audit?.findings || []} />
      <h2>{t('ui.m257')}</h2>
      <Table
        rows={asArray(Object.entries(audit?.fw_versions || {})).map(([version, switches]) => ({
          version,
          switches
        }))}
      />
      <h2>{t('ui.m258')}</h2>
      <button
        disabled={p.locked || p.busy}
        onClick={() => {
          const name = prompt(t('ui.m259'));
          if (name) p.run(async () => {
            await api('/api/baselines', json('POST', { name }));
            await load();
          });
        }}
      >{t('ui.m260')}</button>
      {asArray((bases || [])).map(b => <div key={b.id}>
        {b.name}

        {formatTime(b.captured_ts)}

        <button
          onClick={() => p.run(async () => {
            const r = await api<{
              diffs: object[];
            }>(`/api/baselines/${b.id}/diff`);
            setDiff(r.diffs || []);
          })}
        >{t('ui.m261')}</button>

        <button
          className="button-danger"
          disabled={p.locked || p.busy}
          onClick={() => {
            if (confirm(t('ui.m262', { p0: b.name }))) p.run(async () => {
              await api(`/api/baselines/${b.id}`, json('DELETE'));
              await load();
            });
          }}
        >{t('ui.m263')}</button>
      </div>)}
      <Table rows={diff} />
    </>}
  </section>;
}
