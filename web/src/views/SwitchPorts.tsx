import { eventHeadline, eventTitle, t } from '../i18n';
import type { TranslationKey } from '../i18n';
import { alertTone, hasCriticalActive, matchingPortAlerts, summarizeAlerts } from '../wallAlerts';
import { useEffect, useState } from 'react';
import { StateMark } from '../components/SwitchIndex';
import { DeviceInfo } from '../components/DeviceInfo';
import { VlanFacts } from '../components/VlanFacts';
import { PortVlan, portModeLabel } from '../components/PortVlan';
import { PortLegend, switchHealth } from '../components/RackRow';
import { jstTime, pcTimestamp } from '../pcTime';
import { withoutUnusedLags } from '../rackModel';
import { issueAlerts } from '../monitorHealth';
import { api, json } from '../api';
import { Chart, FrontPanel } from '../components/Common';
import { PollingStatus } from '../components/PollingStatus';
import { PortHardware } from '../components/PortHardware';
import { PingButton } from '../components/PingButton';
import { confirmCounterReset } from '../components/GroupActions';
import { asArray, decimal, formatBps, resolveSwitchId } from '../format';
import type { Alert, Device, Event, MetricResult, ModelDefinitions, Port, SwitchSummary } from '../types';
import { METRICS_REFRESH_MS, usePolling } from '../usePolling';
import { useSwitchState } from '../useSwitchState';
import type { PortMonitorTarget, ViewProps } from './props';
import { groupSwitches } from './props';
import { PaneResizer } from '../components/PaneResizer';
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
  const lines = new Map<string, { key: string; title: string; active: boolean; critical: boolean; count: number }>();
  for (const alert of alerts.filter(item => item.active || !item.acked)) {
    const title = expert ? eventTitle(alert) : eventHeadline(alert);
    const line = lines.get(title) ?? { key: String(alert.event_id), title, active: false, critical: false, count: 0 };
    line.count++;
    line.active = line.active || alert.active;
    line.critical = line.critical || alert.active && alert.severity === 'CRITICAL';
    lines.set(title, line);
  }
  return [...lines.values()].sort((a, b) => Number(b.critical) - Number(a.critical) || Number(b.active) - Number(a.active));
}

// Single-switch view of the port monitor (plan section 21): the former port detail page rearranged into switch health, port panel, linked-port table and a right-hand detail.
type SwitchPortsProps = ViewProps & {
  // Opens the trap setup help from the "no traps yet" hint.
  onTrapHelp?: () => void;
  // Summary for the one-line switch health (passed from the port monitor).
  focusSummary?: SwitchSummary;
  // Port requested by "open port" and similar; opened in the detail once the state arrives.
  initialPort?: PortMonitorTarget;
  // Alerts of this switch with display names filled in; falls back to operations alerts.
  alerts?: Alert[];
  // Health thresholds from the project settings (same rule as the rack row).
  thresholds?: Record<string, number>;
  onClosePort?: () => void;
  // Opens this switch's edit form (name, address, community, group; delete is inside it).
  onEdit?: () => void;
};

// Device shown by its label, LLDP name or (known) vendor; the MAC only when nothing better is known.
export function deviceName(d: Pick<Device, 'label' | 'lldp_name' | 'vendor' | 'mac'>) {
  return d.label || d.lldp_name || d.vendor || d.mac;
}
// Second line: IP, the MAC when it is not already the name, and the vendor when the name is a label. Never the same value twice.
export function deviceDetails(d: Pick<Device, 'label' | 'lldp_name' | 'vendor' | 'mac' | 'ip'>) {
  const name = deviceName(d);
  return [d.ip, name !== d.mac ? d.mac : '', name !== d.vendor ? d.vendor : ''].filter(Boolean).join(' · ');
}
// Only a duplex the switch actually reports; raw values such as "unknown(0)" are not shown.
function duplexLabel(duplex: string | undefined) {
  return /full/i.test(duplex || '') ? t('focus.fullDuplex') : /half/i.test(duplex || '') ? t('focus.halfDuplex') : '';
}
// When the link last changed, in PC time (derived from the switch's ifLastChange, so a wrong switch clock does not matter).
// Time only for today, date and time otherwise, plus a short "how long ago". Unknown (never collected) gives null.
export function linkChangeText(value: string | undefined, now = Date.now()) {
  const date = new Date(value || '');
  if (!value || !Number.isFinite(date.getTime()) || date.getUTCFullYear() < 2) return null;
  const time = jstTime(date, false);
  const stamp = pcTimestamp(value, false, now);
  const when = stamp === jstTime(date) ? time : stamp.replace(jstTime(date), time);
  const minutes = Math.max(0, Math.floor((now - date.getTime()) / 60000));
  const ago = minutes < 60 ? t('focus.minutesAgo', { n: minutes }) : minutes < 48 * 60 ? t('focus.hoursAgo', { n: Math.floor(minutes / 60) })
    : t('focus.daysAgo', { n: Math.floor(minutes / 1440) });
  return { when: t('focus.since', { time: when }), ago };
}
export function speedLabel(mbps: number) {
  return mbps >= 1000 ? `${mbps % 1000 === 0 ? mbps / 1000 : decimal(mbps / 1000)}G` : mbps > 0 ? `${mbps}M` : '—';
}
// RX+TX over the snapshots received since the view opened (no extra requests to the switch or the server).
function Trend({ values }: { values: number[] }) {
  if (values.length < 2) return <span aria-hidden="true">—</span>;
  const max = Math.max(1, ...values);
  const points = values.map((value, index) => `${index / (values.length - 1) * 100},${18 - value / max * 16}`).join(' ');
  return <svg className="focus-trend" viewBox="0 0 100 20" preserveAspectRatio="none" aria-hidden="true">
    <polyline points={points} fill="none" stroke="currentColor" strokeWidth="1.4" vectorEffect="non-scaling-stroke" />
  </svg>;
}
const TREND_SAMPLES = 60;
const HISTORY_ROWS = 5;

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
  // PoE details only while the port powers something or reports a fault (always in expert view).
  const poeShown = !!port?.poe && (!!p.expert || port.poe.power_w > 0 || (!!port.poe.fault && port.poe.fault !== 'none') ||
    /fault/i.test(port.poe.detect || '') || port.poe.denied_cnt > 0 || port.poe.overload_cnt > 0 || port.poe.short_cnt > 0);
  // One chart at a time with its own unit, instead of every metric on one scale.
  const [chartTab, setChartTab] = useState('bandwidth');
  // help: one-line explanation under the chart. labels: legend names (never show internal keys).
  const chartTabs: { id: string; label: TranslationKey; keys: string[]; format?: (value: number) => string; unit?: string; help: TranslationKey; labels: Record<string, string> }[] = [
    { id: 'bandwidth', label: 'focus.chartBandwidth', keys: ['rx_bps', 'tx_bps'], format: formatBps, help: 'chart.bandwidthHelp',
      labels: { rx_bps: t('chart.rxBps'), tx_bps: t('chart.txBps') } },
    { id: 'errors', label: 'focus.chartErrors', keys: ['err_delta', 'discard_delta'], help: 'chart.errorsHelp',
      labels: { err_delta: t('chart.err'), discard_delta: t('chart.discard') } },
    { id: 'multicast', label: 'metrics.multicast', keys: ['rx_mc_pps', 'tx_mc_pps'], unit: 'pps', help: 'chart.multicastHelp',
      labels: { rx_mc_pps: t('chart.rxMc'), tx_mc_pps: t('chart.txMc') } },
    ...(p.expert ? [{ id: 'hardware', label: 'focus.chartHardware' as const, keys: ['poe_w', 'sfp_rx_dbm'], help: 'chart.hardwareHelp' as const,
      labels: { poe_w: t('chart.poe'), sfp_rx_dbm: t('chart.sfpRx') } }] : []),
  ];
  const activeTab = chartTabs.find(tab => tab.id === chartTab) ?? chartTabs[0];
  // One sample per new poll (keyed by last_poll), kept for linked ports only.
  const [trend, setTrend] = useState<Record<number, number[]>>({});
  useEffect(() => {
    if (!s || p.replay) return;
    setTrend(previous => {
      const next: Record<number, number[]> = {};
      for (const x of asArray(s.ports)) {
        if (x.oper !== 'up') continue;
        next[x.if_index] = [...(previous[x.if_index] || []).slice(-(TREND_SAMPLES - 1)), (x.rx_bps || 0) + (x.tx_bps || 0)];
      }
      return next;
    });
  }, [s?.id, s?.last_poll]); // eslint-disable-line react-hooks/exhaustive-deps
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
      // Only the latest few for this port; the full history is in Logs.
      api<Event[]>(`/api/events?switch_id=${p.selected}&if_index=${idx}&limit=${HISTORY_ROWS}`, { signal }),
    ]);
    return { series, history: asArray(history).filter(event => event.if_index === idx).slice(0, HISTORY_ROWS) };
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
      {p.onEdit && <button data-action="edit-switch" disabled={p.busy || p.locked || !!p.replay} onClick={p.onEdit}>{t('rack.edit')}</button>}
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
        {/* Model and firmware live in the device info panel; keep only the IP here (used for Ping and other tools). */}
        <span className="focus-health-meta">{summary.ip}</span>
        {/* Traps arrive only on events, so this is a hint, not a fault: shown only until the first trap from this switch. */}
        {!p.replay && !summary.last_trap_ts && p.onTrapHelp && <button type="button" className="focus-trap-hint" title={t('rack.noTrapHint')}
          aria-label={`${t('rack.noTrapShort')}: ${t('rack.noTrapHint')}`} onClick={p.onTrapHelp}>{t('rack.noTrapShort')} ⓘ</button>}
        {summary.reachable && <span className="focus-health-items">
          {switchHealth(summary, p.thresholds).filter(item => item.key !== 'poe' || summary.poe_budget_w > 0).map(item =>
            <span key={item.key}>{t(`rack.${item.key}`)} <span className={item.bad ? 'health-abnormal' : undefined}>{item.bad ? '⚠ ' : ''}{item.value}</span></span>)}
        </span>}
      </section>}
      {summary && <DeviceInfo device={{ ...s, ip: summary.ip, last_trap_ts: summary.last_trap_ts }} peers={asArray(p.overview?.switches)} />}
      <section className="panel focus-card"><h3>{t('focus.ports')}</h3>
      {/* Tiles show symbols only, so the legend sits with them (no hover on touch screens). */}
      <PortLegend single />
      <FrontPanel
        showNote={p.expert}
        alerts={switchAlerts}
        switchId={s.id}
        reachable={s.reachable}
        model={s.model}
        definitions={models.data}
        selected={idx}
        onSelect={choose}
        ports={withoutUnusedLags(asArray((s.ports || [])).filter(x => !preset?.vlans?.length || asArray(preset.vlans).includes(x.pvid)), switchAlerts)}
      />
      </section>
      {/* Linked ports: device name first, MAC and IP on the second line; fault rows get a state-coloured edge. */}
      <section className="panel focus-card"><h3>{t('focus.linkedPorts')}</h3>
        {linkedPorts.length ? <table className="focus-ports-table">
          <thead><tr><th>{t('focus.colPort')}</th><th>{t('focus.colDevice')}</th><th>{t('focus.colState')}</th>
            <th>{t('focus.colSpeed')}</th><th>{t('focus.colVlan')}</th><th>{t('focus.colPoe')}</th><th>{t('focus.colErrors')}</th>
            <th className="num">{t('focus.colRate')}</th>
            {/* Shown only when the table has room (right-hand detail closed), see .col-extra in style.css. */}
            <th className="num col-extra">{t('focus.colMulticast')}</th><th className="col-extra">{t('focus.colChanged')}</th>
            <th title={t('focus.trendHelp')}>{t('focus.colTrend')}</th></tr></thead>
          <tbody>{linkedPorts.map(x => {
            const counts = summarizeAlerts(portAlerts(x));
            const first = asArray(x.devices)[0];
            const lines = portAlertLines(portAlerts(x), p.expert);
            const poeW = x.poe && x.poe.power_w > 0 ? x.poe.power_w : 0;
            return <tr key={x.if_index} className={[hasCriticalActive(counts) ? 'focus-row-active' : counts.active ? 'focus-row-caution' : counts.count ? 'focus-row-recovered' : '',
              x.if_index === idx ? 'focus-row-selected' : ''].filter(Boolean).join(' ') || undefined}>
              <td><button type="button" className="focus-port-link" aria-pressed={x.if_index === idx} onClick={() => choose(x)}>{x.name}</button></td>
              <td className="focus-device" title={first ? [deviceName(first), deviceDetails(first)].filter(Boolean).join(' · ') : undefined}>{first ? <>{deviceName(first)}
                {asArray(x.devices).length > 1 && <span className="focus-more"> +{asArray(x.devices).length - 1}</span>}
                <small>{deviceDetails(first)}</small></> : '—'}</td>
              <td>{lines.length ? <span className={lines[0].critical ? 'rack-critical-count' : lines[0].active ? 'rack-caution-count' : 'rack-warning-count'}>{lines[0].critical ? '!' : '▲'} {lines[0].title}</span>
                : t('focus.linked')}</td>
              <td>{speedLabel(x.speed_mbps)}</td>
              {/* Show Trunk instead of a VLAN number (the PVID alone does not show that several VLANs are carried). */}
              <td title={x.mode === 'trunk' ? [t('trunk.tagged'), asArray(x.tagged_vlans).join(', ')].join(': ') : undefined}>{x.mode === 'trunk' ? portModeLabel(x, s) : (x.pvid || '—')}</td>
              <td>{poeW ? `${decimal(poeW)} W` : '—'}</td>
              {/* Errors and discards: coloured only when above zero. */}
              <td className="num"><span className={x.err_delta > 0 ? 'metric-bad' : undefined}>{x.err_delta}</span> / <span className={x.discard_delta > 0 ? 'metric-bad' : undefined}>{x.discard_delta}</span></td>
              {/* RX above TX, so the column stays narrow next to the detail. */}
              <td className="num focus-rate"><span>{formatBps(x.rx_bps)}</span><small>{formatBps(x.tx_bps)}</small></td>
              <td className="num focus-rate col-extra"><span>{decimal(x.rx_mc_pps)} pps</span><small>{decimal(x.tx_mc_pps)} pps</small></td>
              <td className="focus-rate col-extra">{(() => {
                const changed = linkChangeText(x.last_change);
                return changed ? <><span>{changed.when}</span><small>{changed.ago}</small></> : '—';
              })()}</td>
              <td><Trend values={trend[x.if_index] || []} /></td>
            </tr>;
          })}</tbody>
        </table> : <p>{t('focus.noLinkedPorts')}</p>}
      </section>
      <VlanFacts vlans={asArray(s.igmp?.vlans)} ports={asArray(s.ports)} globalSnoopingOff={!!s.igmp?.snooping_known && !s.igmp?.snooping_on} />
      </div>
      {port && <PaneResizer cssVar="--detail-width" storageKey="pane-detail-width" side="right" min={288} max={960} defaultPx={400} label={t('pane.detail')} />}
      {port && <aside className="panel focus-detail" aria-label={t('rack.portDetail')}>
        {/* Plan section 22: sections in reading order; settings folded; raw settings only in expert view. */}
        <div className="focus-detail-head"><div><div className="detail-title">{port.name}</div>
          <div className="detail-sub">{[s.name, (port.type || '').toUpperCase(), port.pvid ? `VLAN ${port.pvid}` : ''].filter(Boolean).join(' · ')}</div></div>
          <button type="button" aria-label={t('focus.closeDetail')} onClick={closePort}>{t('rack.close')}</button></div>
        <p className={`detail-state ${port.oper === 'up' ? 'detail-up' : 'detail-down'}`}>
          {port.oper === 'up' ? [t('focus.linked'), port.speed_mbps ? speedLabel(port.speed_mbps) : '', duplexLabel(port.duplex)].filter(Boolean).join(' · ') : t('focus.down')}</p>
        {/* Why this port is flagged, first and in its state colour (ongoing critical red, ongoing warning orange, recovered yellow). */}
        {portAlertLines(portAlerts(port), p.expert)
          .map(line => <p key={line.key} className={`port-alert-line ${line.critical ? 'state-active' : line.active ? 'state-caution' : 'state-recovered'}`}>
            {line.critical ? '!' : '▲'} {line.title}{line.count > 1 && <span className="port-alert-count"> ×{line.count}</span>}
          </p>)}
        <section className="detail-section"><h4>{t('focus.nowValues')}</h4>
          <dl className="detail-kv">
            {/* Receive and send on their own lines so a value never breaks in the middle. */}
            <dt>{t('focus.bandwidth')}</dt><dd className="detail-big detail-pair"><span>{t('metrics.rxOnly', { value: formatBps(port.rx_bps) })}</span><span>{t('metrics.txOnly', { value: formatBps(port.tx_bps) })}</span></dd>
            <dt>{t('focus.colErrors')}</dt><dd><span className={port.err_delta > 0 ? 'metric-bad' : undefined}>{port.err_delta}</span> / <span className={port.discard_delta > 0 ? 'metric-bad' : undefined}>{port.discard_delta}</span></dd>
            <dt>{t('metrics.multicast')}</dt><dd className="detail-pair"><span>{t('metrics.rxOnly', { value: `${decimal(port.rx_mc_pps)} pps` })}</span><span>{t('metrics.txOnly', { value: `${decimal(port.tx_mc_pps)} pps` })}</span></dd>
            {port.poe && !poeShown && <><dt>PoE</dt><dd className="detail-muted">{t('focus.poeNone')}</dd></>}
          </dl>
        </section>
        {/* VLAN mode and the VLANs this port carries (Trunk / Auto-Trunk). */}
        <PortVlan port={port} state={s} />
        {/* PoE table only while powering or faulted (always in expert view); SFP and cable test when present. */}
        {(poeShown || port.sfp || port.cable) && <PortHardware port={poeShown ? port : { ...port, poe: null }} />}
        <section className="detail-section">
          <h4>{t('ui.m090')}</h4>
          {(port.devices || []).length > 1 && <p>{t('ui.m287')}{port.devices.length}
            {t('ui.m288')}</p>}
          <ul className="divide-y divide-slate-500">
            {asArray(port.devices).map(d => <li key={d.mac} className="space-y-1 py-2">
              <div className="flex flex-wrap items-baseline gap-x-2">
                <strong className="min-w-0 break-words" title={d.label && d.lldp_name || undefined}>{deviceName(d)}</strong>
                {d.link_local && <span className="text-xs">Link-Local</span>}
              </div>
              {/* MAC and vendor only when they are not already the name above. */}
            {[deviceName(d) !== d.mac ? d.mac : '', deviceName(d) !== d.vendor ? d.vendor : '', d.ip].some(Boolean) &&
              <p className="break-words text-sm opacity-80">{[deviceName(d) !== d.mac ? d.mac : '', deviceName(d) !== d.vendor ? d.vendor : '', d.ip].filter(Boolean).join(' · ')}</p>}
              {d.ip && <div className="flex flex-wrap items-center gap-2 text-sm">
                <PingButton key={d.ip} ip={d.ip} disabled={p.locked || !!p.replay} />
              </div>}
            </li>)}
          </ul>
        </section>
        {!p.replay && <section className="detail-section"><h4>{t('focus.chart')}</h4>
          <div className="detail-chart-tools">
            <div role="group" aria-label={t('focus.chart')} className="detail-tabs">
              {chartTabs.map(tab => <button key={tab.id} type="button" aria-pressed={activeTab.id === tab.id} onClick={() => setChartTab(tab.id)}>{t(tab.label)}</button>)}
            </div>
            <select value={range} onChange={e => setRange(Number(e.target.value))}>
              {[15, 60, 360, 1440].map(n => <option key={n} value={n}>
                {n < 60 ? t('ui.m299', { p0: n }) : t('ui.m300', { p0: n / 60 })}
              </option>)}
            </select>
          </div>
          <Chart points={metrics.data?.series.points || []} keys={activeTab.keys} format={activeTab.format} height={200}
            labels={activeTab.labels} unit={activeTab.unit} />
          <p className="chart-help">{t(activeTab.help)}</p>
        </section>}
        {!p.replay && <section className="detail-section"><h4>{t('focus.history', { count: HISTORY_ROWS })}</h4>
          <PollingStatus {...metrics} />
          {/* One line per event: time, level, what happened. Everything else is in Logs. */}
          {metrics.data && (metrics.data.history.length ? <ul className="port-history">
            {metrics.data.history.map(event => <li key={event.id} className={`port-history-${event.severity.toLowerCase()}`}>
              <time dateTime={event.ts}>{pcTimestamp(event.ts, true)}</time>
              <span className="port-history-level">{event.severity === 'CRITICAL' ? t('ui.m005') : event.severity === 'WARNING' ? t('ui.m006') : t('ui.m007')}</span>
              <span className="port-history-text" title={eventTitle(event)}>{p.expert ? eventTitle(event) : eventHeadline(event)}</span>
            </li>)}
          </ul> : <p className="muted">{t('focus.noHistory')}</p>)}
          {p.openLogs && <button type="button" data-action="all-logs"
            onClick={() => p.openLogs?.({ switchId: s.id, ifIndex: port.if_index, switchName: s.name, portName: port.name })}>{t('focus.allLogs')}</button>}
        </section>}
        <details className="detail-settings"><summary>{t('focus.portSettings')}</summary>
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
        </details>
        {p.expert && <section className="detail-section detail-expert"><h4>{t('focus.expertOnly')}</h4>
          <dl className="detail-kv">{[
            [t('portSummary.eee'), port.eee],
            [t('portSummary.pause'), port.pause],
            [t('portSummary.udld'), port.udld],
            [t('portSummary.stp'), port.stp_state],
            [t('portSummary.trust'), port.trust],
          ].flatMap(([label, value]) => [<dt key={`${label}-label`}>{label}</dt>, <dd key={`${label}-value`}>{value || '—'}</dd>])}</dl>
        </section>}
      </aside>}
    </div>}
  </>;
}
