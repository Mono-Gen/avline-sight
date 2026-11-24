import { api, json } from '../api';
import { asArray, decimal, formatBps, formatTime } from '../format';
import { frontPanelRows } from '../frontPanelLayout';
import { eventTitle, t } from '../i18n';
import { ActionPopup } from './ActionPopup';
import { ErrorSymbol } from './ErrorSymbol';
import { lampState } from '../rackModel';
import { isPortAlert, matchingPortAlerts, summarizeAlerts } from '../wallAlerts';
import type { Alert, InventoryRow, ModelDefinitions, Port, SwitchConfig, SwitchSummary } from '../types';
import type { ViewProps } from '../views/props';

const lampStates = {
  critical: { symbol: '!', label: 'pages.abnormal' },
  warning: { symbol: '▲', label: 'pages.unconfirmed' },
  healthy: { symbol: '✓', label: 'pages.normal' },
  unused: { symbol: '·', label: 'pages.unused' },
  unknown: { symbol: '?', label: 'pages.unknown' },
} as const;

export function PortLegend() {
  return <ul className="rack-legend" aria-label={t('pages.legend')}>
    {Object.entries(lampStates).map(([tone, { symbol, label }]) => <li key={tone}>
      <span className={`lamp-${tone}`} aria-hidden="true">{symbol}</span>{t(label)}
    </li>)}
  </ul>;
}

export function PortLamp({ port, alerts, devices, reachable, wall, onSelect }: {
  port: Port; alerts: Alert[]; devices: InventoryRow[]; reachable: boolean; wall: boolean; onSelect: () => void;
}) {
  const state = lampState(port, alerts, reachable);
  const attached = devices.length ? devices : asArray(port.devices);
  const tone = state.label === 'rack.unused' && port.oper === 'down' ? 'unused' : state.tone;
  const { symbol, label: stateLabel } = lampStates[tone];
  const label = [port.name, ...attached.map(device => [device.label || device.lldp_name || device.mac, device.ip].filter(Boolean).join(' ')),
    t(stateLabel), state.label === 'rack.suppressed' && t(state.label)].filter(Boolean).join(' · ');
  const content = <><span className="lamp-number">{port.name.match(/(\d+)\D*$/)?.[1] || port.name}</span>
    <span className="lamp-symbol" aria-hidden="true">{symbol}</span></>;
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

export function RackRow({ summary: sw, ports, devices, alerts, definitions, wall, stale, uplinks, samples, actions, onPort, onEdit, onHelp, refresh }: {
  summary: SwitchSummary; ports: Port[]; devices: InventoryRow[]; alerts: Alert[]; definitions: ModelDefinitions;
  wall: boolean; stale: boolean; uplinks: Set<string>; samples: Record<string, { rx: number; tx: number }[]>;
  actions: ViewProps; onPort: (port: Port) => void; onEdit: (config: SwitchConfig | null) => void; onHelp: () => void; refresh: () => Promise<void>;
}) {
  const rows = frontPanelRows(ports, definitions[sw.model]);
  const portAlerts = summarizeAlerts(alerts).items.filter(isPortAlert);
  const callouts = new Map<number, Alert>();
  for (const alert of portAlerts) {
    if (!callouts.has(alert.if_index) || alert.active && !callouts.get(alert.if_index)?.active) callouts.set(alert.if_index, alert);
  }
  const above = (value: number, threshold: number) => threshold > 0 && value >= threshold;
  const thresholds = actions.settings?.thresholds;
  const health = [
    { key: 'cpu', icon: '▣', value: `${decimal(sw.cpu_5s)}%`, bad: above(sw.cpu_5s, thresholds?.cpu_pct ?? 80) },
    { key: 'temp', icon: '♨', value: `${decimal(sw.temp_max_c)}°C`, bad: above(sw.temp_max_c, thresholds?.temp_c ?? 65) || ['warning', 'critical', 'shutdown'].includes(sw.temp_state) },
    { key: 'psu', icon: '⏻', value: t(sw.psu_ok ? 'pages.normal' : 'pages.abnormal'), bad: !sw.psu_ok },
    { key: 'fan', icon: '✣', value: t(sw.fans_ok ? 'pages.normal' : 'pages.abnormal'), bad: !sw.fans_ok },
    { key: 'poe', icon: 'ϟ', value: `${decimal(sw.poe_used_w)} / ${decimal(sw.poe_budget_w)} W`, bad: sw.poe_budget_w > 0 && above(sw.poe_used_w / sw.poe_budget_w * 100, thresholds?.poe_pct ?? 80) },
  ] as const;
  return <article id={`rack-switch-${sw.id}`} tabIndex={-1} className={`rack-row${!sw.reachable || stale ? ' rack-offline' : ''}`} title={[`${sw.model} ${sw.fw}`.trim(), t('rack.lastTrap', { at: sw.last_trap_ts ? formatTime(sw.last_trap_ts) : '—' }), sw.last_error].filter(Boolean).join(' · ')}>
    <div className="rack-identity">
      <strong title={sw.name}>{sw.reachable ? '●' : '⚠'} {sw.name}</strong><span>{sw.ip}</span>
      {!wall && <div className="rack-switch-actions">
        {!sw.last_trap_ts && <button className="trap-help-button" title={t('rack.noTrap')} aria-label={t('rack.noTrap')} onClick={onHelp}>ⓘ</button>}
        <ActionPopup label={`${sw.name} ${t('rack.actions')}`}>
          <button onClick={() => actions.select(sw.id)}>{t('rack.ports')}</button>
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
            <button className="button-danger" disabled={actions.busy} onClick={() => {
              if (window.confirm(t('rack.removeConfirm', { name: sw.name }))) actions.run(async () => {
                await api(`/api/switches/${sw.id}`, json('DELETE')); await refresh();
              });
            }}>{t('rack.remove')}</button>
          </>}
        </ActionPopup>
      </div>}
    </div>
    <div className="rack-front">
      {stale && <span className="rack-stale">{t('rack.stale')}</span>}
      {sw.needs_credentials ? <span>{t('configFile.credentialsRequired')}</span> :
        !ports.length && <span>{t(sw.reachable ? 'rack.loading' : 'rack.offline')}</span>}
      {rows.map((row, index) => <div className="rack-port-row" key={index}>{row.slots.map((port, position) => port
        ? <div className="rack-port-slot" key={port.if_index}>
          <PortLamp port={port} alerts={matchingPortAlerts(alerts, sw.id, port.if_index)} devices={devices.filter(device => device.if_index === port.if_index)}
            reachable={sw.reachable && !stale} wall={wall} onSelect={() => onPort(port)} />
          {uplinks.has(`${sw.id}/${port.name}`) && <Sparkline samples={samples[`${sw.id}/${port.name}`] || []} label={`${port.name} ${t('rack.uplink')} ${formatBps(port.rx_bps)} / ${formatBps(port.tx_bps)}`} />}
        </div> : <span className="rack-port-gap" key={`gap-${position}`} />)}</div>)}
      {callouts.size > 0 && <div className="rack-callouts">{[...callouts.values()].slice(0, 3).map(alert => {
        const cause = alert.code === 'PHY_ERRORS' ? t('pages.physicalError') : eventTitle(alert);
        const text = `${alert.device_name || t('operations.unknownDevice')} · ${alert.port} ${cause}`;
        return <span key={alert.if_index} title={text} className={alert.active ? 'text-red-400' : 'text-amber-400'}>
          {alert.active ? <ErrorSymbol /> : '!'} {text}
        </span>;
      })}{callouts.size > 3 && <span>{t('rack.more', { count: callouts.size - 3 })}</span>}</div>}
    </div>
    <div className="rack-health">{health.map(item => {
      const known = sw.reachable && !stale;
      const label = `${t(`rack.${item.key}`)} · ${known ? item.value : t('pages.unknown')}`;
      return <span key={item.key} title={label} aria-label={label} className={known && item.bad ? 'health-abnormal' : ''}>
        {known && item.bad ? '⚠' : ''}{item.icon}
      </span>;
    })}</div>
  </article>;
}
