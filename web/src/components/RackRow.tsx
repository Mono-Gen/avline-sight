import type React from 'react';
import { api } from '../api';
import { asArray, decimal, formatBps, formatTime } from '../format';
import { frontPanelRows } from '../frontPanelLayout';
import { eventHeadline, eventTitle, t } from '../i18n';
import { ActionPopup } from './ActionPopup';
import { ErrorSymbol } from './ErrorSymbol';
import { lampState } from '../rackModel';
import { isPortAlert, matchingPortAlerts, summarizeAlerts } from '../wallAlerts';
import type { Alert, InventoryRow, ModelDefinitions, Port, SwitchConfig, SwitchSummary } from '../types';
import type { ViewProps } from '../views/props';
import { portMarkLegend, portMarks, portVlanGroup, vlanColor } from './PortVlan';

export const lampStates = {
  critical: { symbol: '!', label: 'pages.abnormal' },
  caution: { symbol: '▲', label: 'pages.caution' },
  warning: { symbol: '▲', label: 'pages.unconfirmed' },
  healthy: { symbol: '✓', label: 'pages.normal' },
  unused: { symbol: '·', label: 'pages.unused' },
  unknown: { symbol: '?', label: 'pages.unknown' },
} as const;

// single: the one-switch front panel, which has no switch dots but shows excluded and PoE marks.
export type RackColor = 'state' | 'vlan';

// vlans: the VLANs shown in the colour-by-VLAN view, with names when known.
export function PortLegend({ single = false, color = 'state', vlans = [] }: { single?: boolean; color?: RackColor; vlans?: { vlan: number; name?: string }[] } = {}) {
  return <ul className={`rack-legend${single ? ' front-legend' : ''}`} aria-label={t('pages.legend')}>
    {color === 'vlan' ? <>
      {vlans.map(({ vlan, name }) => <li key={vlan}><span className="vlan-chip" style={{ '--vlan-color': vlanColor(vlan) } as React.CSSProperties} aria-hidden="true">{vlan}</span>
        {name ? `VLAN ${vlan} ${name}` : `VLAN ${vlan}`}</li>)}
      <li><span className="vlan-chip vlan-chip-multi" aria-hidden="true">T</span>{t('rack.vlanMulti')}</li>
      <li><span className="lamp-critical" aria-hidden="true">!</span>{t('rack.vlanAlertKept')}</li>
    </> : Object.entries(lampStates).map(([tone, { symbol, label }]) => <li key={tone}>
      <span className={`lamp-${tone}`} aria-hidden="true">{symbol}</span>{t(label)}
    </li>)}
    <MarkLegend />
    {single ? <>
      <li><span aria-hidden="true">×</span>{t('ui.m133')}</li>
      <li><span aria-hidden="true">⚡</span>{t('ui.m139')}</li>
      {/* This label already starts with ⛔. */}
      <li>{t('ui.m138')}</li>
    </> : <>
      {/* The dot before a switch name: responding or not. */}
      <li className="rack-legend-switch"><span aria-hidden="true">●</span>{t('pages.switchReachable')}</li>
      <li className="rack-legend-switch"><span aria-hidden="true">⚠</span>{t('pages.switchUnreachable')}</li>
    </>}
  </ul>;
}

// The legend shows short names only; the meanings open from ⓘ, kept on screen so nobody has to look up the README during a show.
function MarkLegend() {
  return <>{portMarkLegend().map(({ mark, label }) => <li key={mark}><span className="port-role-marks" aria-hidden="true">{mark}</span>{label}</li>)}
    <li><details className="legend-help"><summary aria-label={t('trunk.marksHelp')} title={t('trunk.marksHelp')}>ⓘ</summary>
      <ul>{(['helpTrunk', 'helpGeneral', 'helpLag', 'helpBlocked'] as const).map(key => <li key={key}>{t(`trunk.${key}`)}</li>)}</ul>
    </details></li></>;
}

export function PortLamp({ port, alerts, devices, reachable, wall, onSelect, color = 'state' }: {
  port: Port; alerts: Alert[]; devices: InventoryRow[]; reachable: boolean; wall: boolean; onSelect: () => void; color?: RackColor;
}) {
  const state = lampState(port, alerts, reachable);
  const attached = devices.length ? devices : asArray(port.devices);
  const tone = state.label === 'rack.unused' && port.oper === 'down' ? 'unused' : state.tone;
  const { symbol, label: stateLabel } = lampStates[tone];
  const marks = portMarks(port);
  const label = [port.name, ...attached.map(device => [device.label || device.lldp_name || device.mac, device.ip].filter(Boolean).join(' ')),
    t(stateLabel), state.label === 'rack.suppressed' && t(state.label), ...marks.map(m => m.label)].filter(Boolean).join(' · ');
  const number = <span className="lamp-number">{port.name.match(/(\d+)\D*$/)?.[1] || port.name}</span>;
  const roleLine = marks.length > 0 && <span className={`lamp-role${marks.some(m => m.mark === 'B') ? ' port-role-blocked' : ''}`} aria-hidden="true">{marks.map(m => m.mark).join('')}</span>;
  if (color === 'vlan' && !wall) {
    // Colour by VLAN: the fill is the VLAN, the bottom line its number (or T/G for several VLANs).
    // Faults keep their symbol and a red/amber outline so a problem is never hidden by this view.
    const group = portVlanGroup(port);
    const issue = ['critical', 'caution', 'warning'].includes(tone);
    // Offline, stale, excluded or suppressed ports must not look like live ones: keep their symbol and dim them.
    const unknown = tone === 'unknown' || port.excluded;
    const mark = port.excluded && !issue ? '×' : symbol;
    const vlanLabel = group.multi ? '' : group.vlan ? `VLAN ${group.vlan}` : '';
    const style = group.vlan && !group.multi ? { '--vlan-color': vlanColor(group.vlan) } as React.CSSProperties : undefined;
    const cls = `port-lamp lamp-vlan${group.multi ? ' lamp-vlan-multi' : ''}${!group.multi && !group.vlan ? ' lamp-vlan-none' : ''}${issue ? ` lamp-vlan-issue-${tone}` : ''}${unknown && !issue ? ' lamp-vlan-unknown' : ''}${port.oper !== 'up' ? ' lamp-vlan-down' : ''}`;
    const vlanTitle = [label, vlanLabel].filter(Boolean).join(' · ');
    return <button className={cls} style={style} title={vlanTitle} aria-label={vlanTitle} onClick={onSelect}>
      {number}
      {(issue || unknown) && <span className="lamp-symbol" aria-hidden="true">{mark}</span>}
      {group.multi ? roleLine : <span className={`lamp-role${marks.some(m => m.mark === 'B') ? ' port-role-blocked' : ''}`} aria-hidden="true">
        {[group.vlan ?? '', ...marks.filter(m => m.mark === 'L' || m.mark === 'B').map(m => m.mark)].filter(Boolean).join(' ')}</span>}
    </button>;
  }
  const content = <>{number}
    <span className="lamp-symbol" aria-hidden="true">{symbol}</span>
    {roleLine}</>;
  const className = `port-lamp lamp-${tone}`;
  return wall ? <span className={className} title={label} aria-label={label}>{content}</span>
    : <button className={className} title={label} aria-label={label} onClick={onSelect}>{content}</button>;
}

function Sparkline({ samples, label }: { samples: { rx: number; tx: number }[]; label: string }) {
  const max = Math.max(1, ...samples.flatMap(sample => [sample.rx, sample.tx]));
  const plot = samples.length === 1 ? [samples[0], samples[0]] : samples;
  const points = (key: 'rx' | 'tx') => plot.map((sample, index) => `${index / Math.max(1, plot.length - 1) * 58},${18 - sample[key] / max * 16}`).join(' ');
  return <svg role="img" aria-label={label} viewBox="0 0 60 20" className="uplink-spark"><title>{label}</title>
    <polyline points={points('rx')} fill="none" stroke="currentColor" strokeWidth="1.6" />
    <polyline points={points('tx')} fill="none" stroke="currentColor" strokeWidth="1" strokeDasharray="2 2" />
  </svg>;
}

// Switch health items with the same thresholds everywhere (rack row and single-switch view).
export function switchHealth(sw: SwitchSummary, thresholds?: Record<string, number>) {
  const above = (value: number, threshold: number) => threshold > 0 && value >= threshold;
  return [
    { key: 'cpu', icon: '▣', value: `${decimal(sw.cpu_5s)}%`, bad: above(sw.cpu_5s, thresholds?.cpu_pct ?? 80) },
    { key: 'temp', icon: '♨', value: `${decimal(sw.temp_max_c)}°C`, bad: above(sw.temp_max_c, thresholds?.temp_c ?? 65) || ['warning', 'critical', 'shutdown'].includes(sw.temp_state) },
    { key: 'psu', icon: '⏻', value: t(sw.psu_ok ? 'pages.normal' : 'pages.abnormal'), bad: !sw.psu_ok },
    { key: 'fan', icon: '✣', value: t(sw.fans_ok ? 'pages.normal' : 'pages.abnormal'), bad: !sw.fans_ok },
    { key: 'poe', icon: 'ϟ', value: `${decimal(sw.poe_used_w)} / ${decimal(sw.poe_budget_w)} W`, bad: sw.poe_budget_w > 0 && above(sw.poe_used_w / sw.poe_budget_w * 100, thresholds?.poe_pct ?? 80) },
  ] as const;
}

export function RackRow({ summary: sw, ports, devices, alerts, definitions, vlans = [], wall, stale, uplinks, samples, actions, onPort, onEdit, color = 'state' }: {
  summary: SwitchSummary; ports: Port[]; devices: InventoryRow[]; alerts: Alert[]; definitions: ModelDefinitions;
  vlans?: readonly number[];
  wall: boolean; stale: boolean; uplinks: Set<string>; samples: Record<string, { rx: number; tx: number }[]>;
  actions: ViewProps; onPort: (port: Port) => void; onEdit: (config: SwitchConfig | null) => void; color?: RackColor;
}) {
  const rows = frontPanelRows(ports, definitions[sw.model]);
  const portAlerts = summarizeAlerts(alerts).items.filter(isPortAlert);
  const callouts = new Map<number, Alert>();
  for (const alert of portAlerts) {
    if (!callouts.has(alert.if_index) || alert.active && !callouts.get(alert.if_index)?.active) callouts.set(alert.if_index, alert);
  }
  const health = switchHealth(sw, actions.settings?.thresholds);
  return <article id={`rack-switch-${sw.id}`} tabIndex={-1} className={`rack-row${!sw.reachable || stale ? ' rack-offline' : ''}`} title={[`${sw.model} ${sw.fw}`.trim(), t('rack.lastTrap', { at: sw.last_trap_ts ? formatTime(sw.last_trap_ts) : '—' }), sw.last_error].filter(Boolean).join(' · ')}>
    <div className="rack-identity">
      {/* The name opens the switch alone in the port monitor (it replaces the former "Ports" menu item). */}
      <strong title={sw.name}>{sw.reachable ? '●' : '⚠'} {wall ? sw.name : <button type="button" className="rack-switch-name" data-action="open-switch"
        onClick={() => actions.select(sw.id)}>{sw.name}</button>}</strong><span>{sw.ip}</span>
      {/* In replay (always locked) the menu would be empty: nothing to show. */}
      {!wall && !(actions.replay && actions.locked) && <div className="rack-switch-actions">
        <ActionPopup label={`${sw.name} ${t('rack.actions')}`} text={`${t('rack.actions')} ▾`}>
          {!actions.replay && <button disabled={actions.busy} onClick={() => actions.run(async () => {
            const access = await api<{ web_url: string }>(`/api/switches/${sw.id}/access-info`);
            const url = new URL(access.web_url);
            if (!['http:', 'https:'].includes(url.protocol)) throw new Error(t('rack.invalidURL'));
            window.open(url.href, '_blank', 'noopener,noreferrer');
          })}>WebUI</button>}
          {!actions.locked && <>
            {actions.expert && actions.openTerminal && <button disabled={actions.busy}
              onClick={() => actions.openTerminal?.(sw.id)}>{t('rack.terminal')}</button>}
            <button disabled={actions.busy} onClick={() => actions.run(async () => {
              const configs = await api<SwitchConfig[]>('/api/switches');
              const config = asArray(configs).find(item => item.id === sw.id);
              if (config) onEdit(config);
            })}>{t('rack.edit')}</button>
          </>}
        </ActionPopup>
      </div>}
    </div>
    <div className="rack-front">
      {stale && <span className="rack-stale">{t('rack.stale')}</span>}
      {sw.needs_credentials ? <span>{t('configFile.credentialsRequired')}</span> :
        !ports.length && <span>{t(sw.reachable ? 'rack.loading' : 'rack.offline')}</span>}
      {rows.map((row, index) => <div className="rack-port-row" key={index}>{row.slots.map((port, position) => port && (!vlans.length || vlans.includes(port.pvid))
        ? <div className="rack-port-slot" key={port.if_index}>
          <PortLamp port={port} alerts={matchingPortAlerts(alerts, sw.id, port.if_index)} devices={devices.filter(device => device.if_index === port.if_index)}
            reachable={sw.reachable && !stale} wall={wall} onSelect={() => onPort(port)} color={color} />
          {uplinks.has(`${sw.id}/${port.name}`) && <Sparkline samples={samples[`${sw.id}/${port.name}`] || []} label={`${port.name} ${t('rack.uplink')} ${formatBps(port.rx_bps)} / ${formatBps(port.tx_bps)}`} />}
        </div> : <span className="rack-port-gap" key={`gap-${position}`} />)}</div>)}
      {callouts.size > 0 && <div className="rack-callouts">{[...callouts.values()].slice(0, 3).map(alert => {
        const cause = actions.expert ? eventTitle(alert) : alert.code === 'PHY_ERRORS' ? t('pages.physicalError') : eventHeadline(alert);
        const text = `${alert.device_name || t('operations.unknownDevice')} · ${alert.port} ${cause}`;
        const critical = alert.active && alert.severity === 'CRITICAL';
        return <span key={alert.if_index} title={text} className={critical ? 'text-red-400' : alert.active ? 'rack-caution-count' : 'text-amber-400'}>
          {critical ? <ErrorSymbol /> : '▲'} {text}
        </span>;
      })}{callouts.size > 3 && <span>{t('rack.more', { count: callouts.size - 3 })}</span>}</div>}
    </div>
    {(() => {
      const known = sw.reachable && !stale;
      const details = health.map(item => `${t(`rack.${item.key}`)} ${known ? item.value : t('pages.unknown')}`).join(' · ');
      const bad = known ? health.filter(item => item.bad) : [];
      return <div className="rack-health" title={details} tabIndex={0} role="group" aria-label={`${t('pages.bodyLabel')}: ${details}`}>
        {!known ? <span>{t('pages.bodyUnknown')}</span> : !bad.length ? <span className="health-ok">{t('pages.bodyOk')}</span>
          : bad.map(item => <span key={item.key} className="health-abnormal">⚠ {t(`rack.${item.key}`)} {item.value}</span>)}
      </div>;
    })()}
  </article>;
}
