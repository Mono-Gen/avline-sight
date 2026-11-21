import { useEffect, useState } from 'react';
import { api } from '../api';
import { Badge } from '../components/Badge';
import { DiscoveryWizard, LabPanel, SwitchForm } from '../components/Dialogs';
import { SwitchCard } from '../components/SwitchCard';
import { asArray, formatTime } from '../format';
import type { SwitchConfig, SwitchState, SwitchSummary } from '../types';
import type { ViewProps } from './props';

function replaySummary(state: SwitchState): SwitchSummary {
  const ports = asArray(state.ports);
  const health = state.health;
  return {
    id: state.id,
    name: state.name,
    ip: state.ip,
    group_tag: state.group_tag,
    reachable: state.reachable,
    last_error: state.last_error || '',
    model: state.model,
    fw: state.fw,
    uptime_sec: state.uptime_sec,
    cpu_5s: health.cpu_5s,
    mem_free_pct: health.mem_free_pct,
    mbuf_used_pct: health.mbuf_used_pct,
    temp_max_c: health.temp_max_c,
    temp_state: health.temp_state,
    fans_ok: asArray(health.fans).every(item => item.state === 'operational' || item.state === 'normal'),
    psu_ok: asArray(health.psus).every(item => item.state === 'operational' || item.state === 'normal'),
    poe_budget_w: health.poe_budget_w,
    poe_used_w: health.poe_used_w,
    ports_up: asArray(ports).filter(port => port.oper === 'up').length,
    ports_total: ports.length,
    rx_bps: asArray(ports).reduce((sum, port) => sum + port.rx_bps, 0),
    tx_bps: asArray(ports).reduce((sum, port) => sum + port.tx_bps, 0),
    alerts: {
      critical: state.alert_counts?.CRITICAL || 0,
      warning: state.alert_counts?.WARNING || 0
    },
  };
}

export function OverviewView(props: ViewProps) {
  const [edit, setEdit] = useState<SwitchConfig | null>();
  const [discover, setDiscover] = useState(false);
  const [tag, setTag] = useState('');
  const [summaries, setSummaries] = useState<SwitchSummary[]>([]);
  const refresh = async () => {
    const overview = await api<{
      switches: SwitchSummary[];
    }>('/api/overview');
    setSummaries(asArray(overview.switches));
  };
  useEffect(() => setSummaries(asArray(props.overview.switches)), [props.overview]);
  const source = props.replay ? asArray(props.replay).map(replaySummary) : summaries;
  const preset = asArray(props.settings?.views).find(view => view.id === props.part);
  const switches = asArray(source).filter(item => (!tag || item.group_tag === tag) &&
    (!asArray(preset?.tags).length || asArray(preset?.tags).includes(item.group_tag)));
  return (
    <>
      <div className="flex gap-2">
        <select aria-label="グループ" value={tag} onChange={event => setTag(event.target.value)}>
          <option value="">すべてのグループ</option>
          {Array.from(new Set(asArray(source).map(item => item.group_tag))).map(value => <option key={value}>{value}</option>)}
        </select>
        <button disabled={props.locked} onClick={() => setEdit(null)}>追加</button>
        <button disabled={props.locked} onClick={() => setDiscover(true)}>探索</button>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {asArray(switches).map(item => <SwitchCard key={item.id} summary={item} actions={props} onEdit={setEdit} onRefresh={refresh} />)}
      </div>
      {!props.replay && <section className="panel">
        <h2>最近のイベント</h2>
        {asArray(asArray(props.overview.recent_events).slice(0, 20)).map(event => (
          <p key={event.id}>{formatTime(event.ts)} <Badge text={event.severity} /> {event.title}</p>
        ))}
      </section>}
      {props.status?.lab && !props.replay && <LabPanel run={props.run} busy={props.busy || props.locked} switches={asArray(props.overview.switches)} />}
      {edit !== undefined && <SwitchForm
        value={edit || undefined}
        close={() => {
          setEdit(undefined);
          void refresh().catch(() => undefined);
        }}
        run={props.run}
        busy={props.busy}
      />}
      {discover && <DiscoveryWizard
        close={() => {
          setDiscover(false);
          void refresh().catch(() => undefined);
        }}
        run={props.run}
        busy={props.busy}
      />}
    </>
  );
}
