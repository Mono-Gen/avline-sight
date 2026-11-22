import { api, json } from '../api';
import { asArray, decimal, formatBps, formatTime } from '../format';
import { frontPanelRows } from '../frontPanelLayout';
import { t } from '../i18n';
import { lampState } from '../rackModel';
import type { Alert, InventoryRow, ModelDefinitions, Port, SwitchConfig, SwitchSummary } from '../types';
import type { ViewProps } from '../views/props';

export function PortLamp({ port, alerts, devices, suppressed, reachable, wall, onSelect }: {
  port: Port; alerts: Alert[]; devices: InventoryRow[]; suppressed: boolean; reachable: boolean; wall: boolean; onSelect: () => void;
}) {
  const state = lampState(port, alerts, devices, suppressed, reachable);
  const label = `${port.name} · ${devices.map(device => device.label || device.lldp_name || device.mac).join(', ') || asArray(port.devices).map(device => device.label || device.lldp_name || device.mac).join(', ')} · ${t(`rack.${state.tone}` as 'rack.healthy')}`;
  const content = <><span className="lamp-symbol">{state.symbol}</span><span className="lamp-number">{port.name.match(/(\d+)\D*$/)?.[1] || port.name}</span></>;
  const className = `port-lamp lamp-${state.tone}${state.unacked ? ' lamp-unacked' : ''}`;
  return wall ? <span className={className} title={label} aria-label={label}>{content}</span>
    : <button className={className} title={label} aria-label={label} onClick={onSelect}>{content}</button>;
}

function Sparkline({ samples, label }: { samples: { rx: number; tx: number }[]; label: string }) {
  const max = Math.max(1, ...samples.flatMap(sample => [sample.rx, sample.tx]));
  const points = (key: 'rx' | 'tx') => samples.map((sample, index) => `${index / Math.max(1, samples.length - 1) * 58},${18 - sample[key] / max * 16}`).join(' ');
  return <svg role="img" aria-label={label} viewBox="0 0 60 20" className="uplink-spark"><title>{label}</title>
    <polyline points={points('rx')} fill="none" stroke="currentColor" strokeWidth="1.6" />
    <polyline points={points('tx')} fill="none" stroke="currentColor" strokeWidth="1" strokeDasharray="2 2" />
  </svg>;
}

export function RackRow({ summary: sw, ports, devices, alerts, definitions, suppressed, wall, stale, uplinks, samples, actions, onPort, onEdit, onHelp, refresh }: {
  summary: SwitchSummary; ports: Port[]; devices: InventoryRow[]; alerts: Alert[]; definitions: ModelDefinitions;
  suppressed: boolean; wall: boolean; stale: boolean; uplinks: Set<string>; samples: Record<string, { rx: number; tx: number }[]>;
  actions: ViewProps; onPort: (port: Port) => void; onEdit: (config: SwitchConfig | null) => void; onHelp: () => void; refresh: () => Promise<void>;
}) {
  const rows = frontPanelRows(ports, definitions[sw.model]);
  const portAlerts = alerts.filter(alert => alert.if_index > 0);
  const callouts = Array.from(new Map(portAlerts.map(alert => [alert.if_index, alert])).values());
  const health = [
    { key: 'cpu', icon: '▣', value: `${decimal(sw.cpu_5s)}%`, bad: sw.cpu_5s >= (actions.settings?.thresholds.cpu_pct ?? 80) },
    { key: 'temp', icon: '♨', value: `${decimal(sw.temp_max_c)}°C`, bad: sw.temp_max_c >= (actions.settings?.thresholds.temp_c ?? 65) || Boolean(sw.temp_state && !['normal', 'operational', 'ok'].includes(sw.temp_state)) },
    { key: 'psu', icon: '⏻', value: '', bad: !sw.psu_ok },
    { key: 'fan', icon: '✣', value: '', bad: !sw.fans_ok },
    { key: 'poe', icon: 'ϟ', value: `${decimal(sw.poe_used_w)} / ${decimal(sw.poe_budget_w)} W`, bad: sw.poe_budget_w > 0 && sw.poe_used_w / sw.poe_budget_w * 100 >= (actions.settings?.thresholds.poe_pct ?? 80) },
  ] as const;
  return <article className={`rack-row${!sw.reachable || stale ? ' rack-offline' : ''}`} title={`${sw.model} ${sw.fw} · ${t('rack.lastTrap', { at: sw.last_trap_ts ? formatTime(sw.last_trap_ts) : '—' })}`}>
    <div className="rack-identity">
      <strong title={sw.name}>{sw.reachable ? '●' : '⚠'} {sw.name}</strong><span>{sw.ip}</span>
      {!wall && <div className="rack-switch-actions">
        {!sw.last_trap_ts && <button className="trap-help-button" title={t('rack.noTrap')} aria-label={t('rack.noTrap')} onClick={onHelp}>ⓘ</button>}
        <details className="group-menu"><summary aria-label={`${sw.name} ${t('rack.actions')}`}>⋯</summary><div className="group-menu-items">
          <button onClick={() => actions.select(sw.id)}>{t('rack.ports')}</button>
          {!actions.replay && <button disabled={actions.busy} onClick={() => actions.run(async () => {
            const access = await api<{ web_url: string }>(`/api/switches/${sw.id}/access-info`);
            const url = new URL(access.web_url);
            if (!['http:', 'https:'].includes(url.protocol)) throw new Error(t('rack.invalidURL'));
            window.open(url.href, '_blank', 'noopener,noreferrer');
          })}>WebUI</button>}
          {!actions.locked && <>
            <button disabled={actions.busy} onClick={() => actions.openTerminal(sw.id)}>{t('rack.terminal')}</button>
            <button disabled={actions.busy} onClick={() => actions.run(async () => {
              const configs = await api<SwitchConfig[]>('/api/switches');
              const config = asArray(configs).find(item => item.id === sw.id);
              if (config) onEdit(config);
            })}>{t('rack.edit')}</button>
            <button disabled={actions.busy} onClick={() => {
              if (window.confirm(t('rack.removeConfirm', { name: sw.name }))) actions.run(async () => {
                await api(`/api/switches/${sw.id}`, json('DELETE')); await refresh();
              });
            }}>{t('rack.remove')}</button>
          </>}
        </div></details>
      </div>}
    </div>
    <div className="rack-front">
      {stale && <span className="rack-stale">{t('rack.stale')}</span>}
      {!ports.length && <span>{t('rack.loading')}</span>}
      {rows.map((row, index) => <div className="rack-port-row" key={index}>{row.slots.map((port, position) => port
        ? <div className="rack-port-slot" key={port.if_index}>
          <PortLamp port={port} alerts={alerts.filter(alert => alert.if_index === port.if_index)} devices={devices.filter(device => device.if_index === port.if_index)}
            suppressed={suppressed} reachable={sw.reachable && !stale} wall={wall} onSelect={() => onPort(port)} />
          {uplinks.has(`${sw.id}/${port.name}`) && <Sparkline samples={samples[`${sw.id}/${port.name}`] || []} label={`${port.name} ${t('rack.uplink')} ${formatBps(port.rx_bps)} / ${formatBps(port.tx_bps)}`} />}
        </div> : <span className="rack-port-gap" key={`gap-${position}`} />)}</div>)}
      {callouts.length > 0 && <div className="rack-callouts">{callouts.slice(0, 3).map(alert => <span key={alert.if_index} className={alert.suppressed ? '' : alert.severity === 'CRITICAL' ? 'text-red-400' : 'text-amber-400'}>
        {alert.suppressed ? '🔧' : alert.severity === 'CRITICAL' ? '✖' : '▲'} {alert.device_name || devices.find(device => device.if_index === alert.if_index)?.label || alert.port}
      </span>)}{callouts.length > 3 && <span>{t('rack.more', { count: callouts.length - 3 })}</span>}</div>}
    </div>
    <div className="rack-health">{health.map(item => <span key={item.key} title={`${t(`rack.${item.key}`)} ${item.value}`} aria-label={`${t(`rack.${item.key}`)} ${item.bad ? '⚠' : '✓'} ${item.value}`} className={item.bad ? 'health-abnormal' : ''}>{item.bad ? '⚠' : ''}{item.icon}</span>)}</div>
  </article>;
}
