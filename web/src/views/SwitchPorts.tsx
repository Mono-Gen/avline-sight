import { eventHeadline, eventTitle, t } from '../i18n';
import { alertTone, matchingPortAlerts, summarizeAlerts } from '../wallAlerts';
import { useEffect, useState } from 'react';
import { StateMark } from '../components/SwitchIndex';
import { switchHealth } from '../components/RackRow';
import { issueAlerts } from '../monitorHealth';
import { api, json } from '../api';
import { Chart, FrontPanel, Table } from '../components/Common';
import { PollingStatus } from '../components/PollingStatus';
import { PortHardware } from '../components/PortHardware';
import { confirmCounterReset } from '../components/GroupActions';
import { asArray, decimal, formatBps, resolveSwitchId } from '../format';
import type { Alert, Event, MetricResult, ModelDefinitions, Port, SwitchSummary } from '../types';
import { METRICS_REFRESH_MS, usePolling } from '../usePolling';
import { useSwitchState } from '../useSwitchState';
import type { PortMonitorTarget, ViewProps } from './props';
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
// One line per alert title (ongoing first), with a count for repeats, so the reason is readable at a glance.
function portAlertLines(alerts: Alert[], expert = false) {
  const lines = new Map<string, { key: string; title: string; active: boolean; count: number }>();
  for (const alert of alerts.filter(item => item.active || !item.acked)) {
    const title = expert ? eventTitle(alert) : eventHeadline(alert);
    const line = lines.get(title) ?? { key: String(alert.event_id), title, active: false, count: 0 };
    line.count++;
    line.active = line.active || alert.active;
    lines.set(title, line);
  }
  return [...lines.values()].sort((a, b) => Number(b.active) - Number(a.active));
}

// Single-switch view of the port monitor (plan section 21): the former port detail page rearranged into switch health, port panel, linked-port table and a right-hand detail.
type SwitchPortsProps = ViewProps & {
  // Summary for the one-line switch health (passed from the port monitor).
  focusSummary?: SwitchSummary;
  // Port requested by "open port" and similar; opened in the detail once the state arrives.
  initialPort?: PortMonitorTarget;
  // Alerts of this switch with display names filled in; falls back to operations alerts.
  alerts?: Alert[];
  // Health thresholds from the project settings (same rule as the rack row).
  thresholds?: Record<string, number>;
  onClosePort?: () => void;
};

export function SwitchPorts(p: SwitchPortsProps) {
  const [idx, setIdx] = useState(0);
  const [appliedTarget, setAppliedTarget] = useState<PortMonitorTarget>();
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
  const closePort = () => {
    setIdx(0);
    p.onClosePort?.();
  };
  // Select the requested port once, when the state arrives, without overriding later choices.
  useEffect(() => {
    const target = p.initialPort;
    if (!target || appliedTarget === target || !s || s.id !== target.switchId) return;
    const found = asArray(s.ports).find(x => target.ifIndex ? x.if_index === target.ifIndex : x.name === target.portName);
    if (!found) return;
    setAppliedTarget(target);
    choose(found);
  }, [p.initialPort, s]); // eslint-disable-line react-hooks/exhaustive-deps
  // In replay the caller passes the replay's own alerts; live alerts are never mixed into a replay.
  const switchAlerts = p.alerts ?? (p.replay ? [] : asArray(p.operations?.alerts).filter(alert => alert.switch_id === s?.id));
  // Only real issues (not INFO, not phase-suppressed), matching the lamps, the index and the number panels.
  const portAlerts = (port: Port) => s ? issueAlerts(matchingPortAlerts(switchAlerts, s.id, port.if_index)) : [];
  const linkedPorts = asArray(s?.ports).filter(x => x.oper === 'up' || summarizeAlerts(portAlerts(x)).count > 0);
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
  const summary = p.focusSummary;
  return <>
    {/* Switches are chosen in the left index; only actions here. */}
    <div className="flex gap-2">
      {p.expert &&<button disabled={!selected || selected !== p.selected || p.locked || !p.openTerminal}
        onClick={() => p.openTerminal?.(selected)}>{t('ui.m279')}</button>}
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
    {s && selected === p.selected && <div className="focus-layout">
      <div className="focus-main">
      {s.needs_credentials && <p role="status">⚠ {t('configFile.credentialsRequired')}</p>}
      {/* Switch health on one line; only abnormal values are coloured. */}
      {summary && <section className="panel focus-health" aria-label={t('focus.health')}>
        <span className="focus-health-name"><StateMark tone={alertTone(summarizeAlerts(switchAlerts), summary.reachable)} />{summary.name}</span>
        <span className="focus-health-meta">{[summary.model, summary.ip, summary.fw && `FW ${summary.fw}`].filter(Boolean).join(' · ')}</span>
        {summary.reachable && <span className="focus-health-items">
          {switchHealth(summary, p.thresholds).filter(item => item.key !== 'poe' || summary.poe_budget_w > 0).map(item =>
            <span key={item.key}>{t(`rack.${item.key}`)} <span className={item.bad ? 'health-abnormal' : undefined}>{item.bad ? '⚠ ' : ''}{item.value}</span></span>)}
        </span>}
      </section>}
      <section className="panel focus-card"><h3>{t('focus.ports')}</h3>
      <FrontPanel
        showNote={p.expert}
        alerts={switchAlerts}
        switchId={s.id}
        reachable={s.reachable}
        model={s.model}
        definitions={models.data}
        selected={idx}
        onSelect={choose}
        ports={asArray((s.ports || [])).filter(x => !preset?.vlans?.length || asArray(preset.vlans).includes(x.pvid))}
      />
      </section>
      {/* Linked ports: device name first, MAC and IP on the second line; fault rows get a state-coloured edge. */}
      <section className="panel focus-card"><h3>{t('focus.linkedPorts')}</h3>
        {linkedPorts.length ? <table className="focus-ports-table">
          <thead><tr><th>{t('focus.colPort')}</th><th>{t('focus.colDevice')}</th><th>{t('focus.colState')}</th><th>{t('focus.colRate')}</th></tr></thead>
          <tbody>{linkedPorts.map(x => {
            const counts = summarizeAlerts(portAlerts(x));
            const first = asArray(x.devices)[0];
            const lines = portAlertLines(portAlerts(x), p.expert);
            return <tr key={x.if_index} className={[counts.active ? 'focus-row-active' : counts.count ? 'focus-row-recovered' : '',
              x.if_index === idx ? 'focus-row-selected' : ''].filter(Boolean).join(' ') || undefined}>
              <td><button type="button" className="focus-port-link" aria-pressed={x.if_index === idx} onClick={() => choose(x)}>{x.name}</button></td>
              <td>{first ? <>{first.label || first.lldp_name || first.mac}
                {asArray(x.devices).length > 1 && ` +${asArray(x.devices).length - 1}`}
                {/* The MAC is already the name when the device has no label or LLDP name; do not repeat it. */}
                <small>{[first.label || first.lldp_name ? first.mac : '', first.ip].filter(Boolean).join(' · ')}</small></> : '—'}</td>
              <td>{lines.length ? <span className={lines[0].active ? 'rack-critical-count' : 'rack-warning-count'}>{lines[0].active ? '!' : '▲'} {lines[0].title}</span>
                : t('focus.linked')}</td>
              <td className="whitespace-nowrap">{formatBps(x.rx_bps)} / {formatBps(x.tx_bps)}</td>
            </tr>;
          })}</tbody>
        </table> : <p>{t('focus.noLinkedPorts')}</p>}
      </section>
      </div>
      {port && <aside className="panel focus-detail space-y-3" aria-label={t('rack.portDetail')}>
        <div className="focus-detail-head"><strong>{s.name} / {port.name}</strong>
          <button type="button" aria-label={t('focus.closeDetail')} onClick={closePort}>{t('rack.close')}</button></div>
        {/* Why this port is flagged, first and in its state colour (ongoing red, recovered/unacknowledged yellow). */}
        {portAlertLines(portAlerts(port), p.expert)
          .map(line => <p key={line.key} className={`port-alert-line ${line.active ? 'state-active' : 'state-recovered'}`}>
            {line.active ? '!' : '▲'} {line.title}{line.count > 1 && <span className="port-alert-count"> ×{line.count}</span>}
          </p>)}
        <h2 className="text-xl">
          {port.name}
          ·
          {port.type}
          ·
          {port.speed_mbps}
          Mbps
        </h2>
        {/* Same shape for bandwidth and packet rates: "RX value / TX value", with a space after every label. */}
        <p className="metric-line">{t('metrics.rxtx', { rx: formatBps(port.rx_bps), tx: formatBps(port.tx_bps) })}
          <span className="metric-sep"> · </span><span className={port.err_delta > 0 ? 'metric-bad' : undefined}>{t('metrics.errors', { value: port.err_delta })}</span>
          <span className="metric-sep"> · </span><span className={port.discard_delta > 0 ? 'metric-bad' : undefined}>{t('metrics.discards', { value: port.discard_delta })}</span></p>
        <p className="metric-line">{t('metrics.multicast')} {t('metrics.rxtx', { rx: `${decimal(port.rx_mc_pps)} pps`, tx: `${decimal(port.tx_mc_pps)} pps` })}</p>
        <p>{[
          [t('portSummary.eee'), port.eee],
          [t('portSummary.pause'), port.pause],
          [t('portSummary.udld'), port.udld],
          [t('portSummary.stp'), port.stp_state],
          [t('portSummary.trust'), port.trust],
        ].map(pair => pair.join(' ')).join(' · ')}</p>
        <PortHardware port={port} />
        <h3>{t('ui.m090')}</h3>
        {(port.devices || []).length > 1 && <p>{t('ui.m287')}{port.devices.length}
          {t('ui.m288')}</p>}
        <ul className="divide-y divide-slate-500">
          {asArray(port.devices).map(d => <li key={d.mac} className="space-y-1 py-2">
            <div className="flex flex-wrap items-baseline gap-x-2">
              <strong className="min-w-0 break-words" title={d.label && d.lldp_name || undefined}>{d.label || d.lldp_name || d.mac}</strong>
              {d.link_local && <span className="text-xs">Link-Local</span>}
            </div>
            <p className="break-words text-sm opacity-80">{[d.mac, d.vendor, d.ip].filter(Boolean).join(' · ')}</p>
            {d.ip && <div className="flex flex-wrap items-center gap-2 text-sm">
              <button type="button" className="px-1.5 py-0.5"
                disabled={p.busy || p.locked}
                onClick={() => p.run(async () => {
                  const r = await api<{
                    ok: boolean;
                    rtt_ms: number;
                    output: string;
                  }>('/api/diagnostics/ping', json('POST', { ip: d.ip }));
                  alert(`${r.ok ? t('ui.m289') : t('ui.m290')} ${r.rtt_ms}ms ${r.output}`);
                })}
              >Ping</button>
              <a className="px-1.5 py-0.5 underline" target="_blank" rel="noreferrer" href={`http://${d.ip}`}>HTTP</a>
              <a className="px-1.5 py-0.5 underline" target="_blank" rel="noreferrer" href={`https://${d.ip}`}>HTTPS</a>
            </div>}
          </li>)}
        </ul>
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex max-w-full items-center gap-2 whitespace-nowrap">
            {t('ui.m101')}
            <input className="min-w-0" value={memo} onChange={e => setMemo(e.target.value)} />
          </label>

          <label className="flex items-center gap-2">
            <input type="checkbox" checked={excluded} onChange={e => setExcluded(e.target.checked)} />
            {t('ui.m133')}
          </label>

          <label className="flex max-w-full items-center gap-2 whitespace-nowrap">
            {t('ui.m291')}
            <input className="min-w-0" type="number" value={speed} onChange={e => setSpeed(Number(e.target.value))} />
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
          className=""
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
          <Table rows={metrics.data?.history || []} hide={p.expert ? [] : ['detail', 'hint']} expert={p.expert} />
        </>}
      </aside>}
    </div>}
  </>;
}
