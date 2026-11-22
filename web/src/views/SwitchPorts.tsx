import { t } from '../i18n';
import { useEffect, useState } from 'react';
import { api, json } from '../api';
import { Chart, FrontPanel, Table } from '../components/Common';
import { PollingStatus } from '../components/PollingStatus';
import { PortHardware } from '../components/PortHardware';
import { confirmCounterReset } from '../components/GroupActions';
import { asArray, decimal, formatBps, resolveSwitchId } from '../format';
import type { Event, MetricResult, ModelDefinitions, Port } from '../types';
import { METRICS_REFRESH_MS, usePolling } from '../usePolling';
import { useSwitchState } from '../useSwitchState';
import type { ViewProps } from './props';
import { groupSwitches } from './props';
const roles = [
  '',
  'dante',
  'aes67',
  'ndi',
  'ndi4',
  'sdvoe',
  'st2110',
  'qsys',
  'lighting',
  'avb',
  'control',
  'uplink'
];
export function SwitchPorts(p: ViewProps) {
  const [idx, setIdx] = useState(0);
  const [range, setRange] = useState(15);
  const [memo, setMemo] = useState('');
  const [excluded, setExcluded] = useState(false);
  const [speed, setSpeed] = useState(0);
  const [role, setRole] = useState('');
  const [until, setUntil] = useState(0);
  const [now, setNow] = useState(Date.now());
  const switches = groupSwitches(p);
  const selected = resolveSwitchId(switches, p.selected);
  const { state: s, polling: statePolling } = useSwitchState(selected, p.state, p.replay);
  const models = usePolling(signal => api<ModelDefinitions>('/api/models', { signal }), 0);
  useEffect(() => {
    
    if (selected !== p.selected) {
      setIdx(0);
      if (selected) p.select(selected);
    }
  }, [selected, p.selected]);
  const port = asArray(s?.ports)?.find(x => x.if_index === idx);
  const preset = asArray(p.settings?.views)?.find(v => v.id === p.part);
  const choose = (x: Port) => {
    setIdx(x.if_index);
    setMemo(x.memo || '');
    setExcluded(x.excluded);
    setSpeed(x.expected_speed_mbps || 0);
    setRole(x.av_role || '');
  };
  const metrics = usePolling(async signal => {
    const end = new Date();
    const from = new Date(end.getTime() - range * 60000);
    const params = new URLSearchParams({
      switch_id: String(p.selected),
      if_index: String(idx),
      from: from.toISOString(),
      to: end.toISOString(),
    });
    const [series, history] = await Promise.all([
      api<MetricResult>(`/api/metrics/port?${params}`, { signal }),
      api<Event[]>(`/api/events?switch_id=${p.selected}`, { signal }),
    ]);
    return { series, history: asArray(history).filter(event => event.if_index === idx) };
  }, METRICS_REFRESH_MS, [p.selected, idx, range], !p.replay && p.selected > 0 && idx > 0);
  useEffect(() => {
    if (!until) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [until]);
  return <>
    <div className="flex gap-2">
      <select
        value={selected}
        onChange={e => {
          p.select(Number(e.target.value));
          setIdx(0);
        }}
      >
        <option value="0">{t('ui.m254')}</option>
        {switches.map(x => <option key={x.id} value={x.id}>
          {x.name}
        </option>)}
      </select>
      <button disabled={!selected || selected !== p.selected || p.locked} onClick={() => p.openTerminal(selected)}>{t('ui.m279')}</button>
      <button
        disabled={!selected || selected !== p.selected || !!p.replay}
        onClick={() => p.run(async () => {
          const a = await api<{
            web_url: string;
          }>(`/api/switches/${p.selected}/access-info`);
          const u = new URL(a.web_url);
          if (!asArray(['http:', 'https:']).includes(u.protocol)) throw new Error(t('ui.m280'));
          window.open(u.href, '_blank', 'noopener,noreferrer');
        })}
      >WebUI
      </button>
    </div>
    {!p.replay && <PollingStatus {...statePolling} />}
    {s && selected === p.selected && <>
      <FrontPanel
        alerts={p.replay ? [] : p.operations?.alerts}
        switchId={s.id}
        reachable={s.reachable}
        model={s.model}
        definitions={models.data}
        selected={idx}
        onSelect={choose}
        ports={asArray((s.ports || [])).filter(x => !preset?.vlans?.length || asArray(preset.vlans).includes(x.pvid))}
      />
      {port && <section className="panel space-y-3">
        <h2 className="text-xl">
          {port.name}
          ·
          {port.type}
          ·
          {port.speed_mbps}
          Mbps
        </h2>
        <p>{t('ui.m281')}{formatBps(port.rx_bps)} {t('ui.m282')}{formatBps(port.tx_bps)} {t('ui.m283')}{port.err_delta} {t('ui.m284')}{port.discard_delta}</p>
        <p>{t('ui.m285')}{decimal(port.rx_mc_pps)} {t('ui.m286')}{decimal(port.tx_mc_pps)} pps</p>
        <p>EEE
          {port.eee}
          · PAUSE
          {port.pause}
          · UDLD
          {port.udld}
          · STP
          {port.stp_state}
          · Trust
          {port.trust}
        </p>
        <PortHardware port={port} />
        <h3>{t('ui.m090')}</h3>
        {(port.devices || []).length > 1 && <p>{t('ui.m287')}{port.devices.length}
          {t('ui.m288')}</p>}
        {asArray((port.devices || [])).map(d => <div key={d.mac}>
          {d.mac}

          {d.vendor}

          {d.ip}

          {d.lldp_name}

          {d.label}

          {d.link_local ? 'Link-Local' : ''}

          {d.ip && <>
            <button
              disabled={p.busy || p.locked}
              onClick={() => p.run(async () => {
                const r = await api<{
                  ok: boolean;
                  rtt_ms: number;
                  output: string;
                }>('/api/diagnostics/ping', json('POST', { ip: d.ip }));
                alert(`${r.ok ? t('ui.m289') : t('ui.m290')} ${r.rtt_ms}ms ${r.output}`);
              })}
            >Ping
            </button>

            <a target="_blank" rel="noreferrer" href={`http://${d.ip}`}>HTTP
            </a>

            <a target="_blank" rel="noreferrer" href={`https://${d.ip}`}>HTTPS
            </a>
          </>}
        </div>)}
        <div>
          <label>{t('ui.m101')}<input value={memo} onChange={e => setMemo(e.target.value)} />
          </label>

          <label>
            <input type="checkbox" checked={excluded} onChange={e => setExcluded(e.target.checked)} />
            {t('ui.m133')}</label>

          <label>{t('ui.m291')}<input type="number" value={speed} onChange={e => setSpeed(Number(e.target.value))} />
          </label>

          <select value={role} onChange={e => setRole(e.target.value)}>
            {asArray(roles).map(r => <option key={r} value={r}>
              {r || t('ui.m292')}
            </option>)}
          </select>
          <button
            className="button-primary"
            disabled={p.busy || p.locked}
            onClick={() => p.run(async () => {
              await api(`/api/switches/${p.selected}/ports/${idx}`, json('PUT', {
                memo,
                excluded,
                expected_speed_mbps: speed,
                av_role: role
              }));
            })}
          >{t('ui.m187')}</button>

          <button
            disabled={p.busy || p.locked}
            onClick={() => p.run(async () => {
              const next = [
                ...asArray(p.watch).filter(w => !(w.switch_id === p.selected && w.if_index === idx)),
                {
                  switch_id: p.selected,
                  if_index: idx,
                  label: `${s.name} ${port.name}`
                }
              ];
              await api('/api/watchlist', json('PUT', next));
              p.setWatch(next);
            })}
          >{t('ui.m293')}</button>
        </div>
        <div className="operation-area" role="group" aria-label={t('rack.actions')}><button
          className="button-danger"
          disabled={p.busy || p.locked}
          onClick={() => {
            if (confirmCounterReset(t('ui.m294'), p.operations?.phase || p.status?.phase)) p.run(async () => {
              await api(`/api/switches/${p.selected}/counters/reset`, json('POST', { if_index: idx }));
              setUntil(Date.now() + 600000);
              setNow(Date.now());
            });
          }}
        >{t('ui.m295')}</button></div>

        {until > now && <span>{t('ui.m296')}{Math.ceil((until - now) / 1000)}
          {t('ui.m297')}{port.err_since_reset}
          {t('ui.m298')}{port.discard_since_reset}
        </span>}
        {!p.replay && <>
          <select value={range} onChange={e => setRange(Number(e.target.value))}>
            {[15, 60, 360, 1440].map(n => <option key={n} value={n}>
              {n < 60 ? t('ui.m299', { p0: n }) : t('ui.m300', { p0: n / 60 })}
            </option>)}
          </select>
          <Chart
            points={metrics.data?.series.points || []}
            keys={['rx_bps', 'tx_bps', 'rx_mc_pps', 'err_delta', 'discard_delta', 'poe_w', 'sfp_rx_dbm']}
          />
          <h3>{t('ui.m301')}</h3>
          <PollingStatus {...metrics} />
          <Table rows={metrics.data?.history || []} />
        </>}
      </section>}
    </>}
  </>;
}
