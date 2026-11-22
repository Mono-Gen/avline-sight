import { t } from '../i18n';
import { useState } from 'react';
import { api, json } from '../api';
import { Badge } from '../components/Badge';
import { DeviceCsvImportDialog } from '../components/DeviceCsvImportDialog';
import { PollingStatus } from '../components/PollingStatus';
import { bulkPingTimeoutMs, deviceUrl, importSummary, MAX_BULK_PING, pingLabel, pingTargets } from '../deviceCsv';
import type { DeviceScope } from '../deviceCsv';
import { asArray, decimal, formatTime } from '../format';
import { pcTimestamp } from '../pcTime';
import type { BulkPingResult, BulkPingResponse, InventoryRow } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';
import { alertStates, alertTone, matchingDeviceAlerts, summarizeAlerts } from '../wallAlerts';
import type { Alert } from '../types';

const emptyCell = '—';

function lastSeen(value: string | undefined): string {
  
  return value && value !== '0001-01-01T00:00:00Z' ? formatTime(value) : t('ui.m217');
}

export function DeviceInventoryTable({ rows, alerts = [], onNavigate }: {
  rows: InventoryRow[]; alerts?: Alert[]; onNavigate?: ViewProps['openPortMonitor'];
}) {
  const columns = [t('rack.connectionState'), t('ui.m100'), 'MAC', 'IP', t('rack.plannedIP'), t('rack.pingStatus'), t('operations.pingAt'),
    t('ui.m074'), t('ui.m213'), t('ui.m018'), t('ui.m020'), 'VLAN', 'PoE(W)', t('ui.m214'), t('ui.m215'), t('ui.m216')];
  return (
    <div className="overflow-auto">
      <table className="w-full text-sm">
        <thead><tr>{columns.map(column => <th key={column} scope="col" className="whitespace-nowrap">{column}</th>)}</tr></thead>
        <tbody>
          {asArray(rows).map(row => {
            const switchId = row.switch_id;
            const tone = alertTone(summarizeAlerts(matchingDeviceAlerts(alerts, row)), row.seen && !!switchId);
            const state = alertStates[tone];
            const navigate = switchId && onNavigate ? () => onNavigate({ switchId, ifIndex: row.if_index, portName: row.port }) : undefined;
            return <tr key={row.mac} onClick={navigate} className={navigate ? 'device-port-row' : undefined}>
              <td><span className={`device-state status-${tone}`} title={t(state.label)} aria-label={t(state.label)}>{state.symbol}</span></td>
              <td>
                {row.label || <span className="text-slate-500 dark:text-slate-400">{t('ui.m091')}</span>}
                {!row.seen && <span className="ml-2 whitespace-nowrap"><Badge text={t('ui.m218')} /></span>}
              </td>
              <td className="whitespace-nowrap font-mono">{row.mac}</td>
              <td>
                {row.ip || emptyCell}
                {row.ip && row.ip_stale && (
                  <span className="ml-2 whitespace-nowrap" title={t('ui.m219')}>
                    <Badge text={t('ui.m220')} />
                  </span>
                )}
              </td>
              <td className="whitespace-nowrap">{row.planned_ip || emptyCell}
                {row.ip_mismatch && <span className="text-amber-700 dark:text-amber-300" title={t('operations.mismatch')} aria-label={t('operations.mismatch')}> ▲</span>}
              </td>
              <td className="whitespace-nowrap">{row.ping ? pingLabel(row.ping.status, row.ping.rtt_ms) :
                row.ping_ok == null ? emptyCell : pingLabel(row.ping_ok ? 'ok' : 'timeout', row.ping_rtt_ms ?? 0)}</td>
              <td className="whitespace-nowrap">{pcTimestamp(row.ping_ts || row.ping?.at || '')}</td>
              <td>{row.vendor || emptyCell}</td>
              <td>{row.lldp_name || emptyCell}</td>
              <td>{row.switch_name || emptyCell}</td>
              <td>{navigate ? <button onClick={event => { event.stopPropagation(); navigate(); }} aria-label={`${t('rack.nav.monitor')} · ${row.switch_name} ${row.port}`}>{row.port || row.switch_name}</button> : row.port || emptyCell}</td>
              <td>{row.vlan || emptyCell}</td>
              <td>{decimal(row.poe_w)}</td>
              <td className="whitespace-nowrap">{row.speed_mbps > 0 ? `${row.speed_mbps} Mbps` : emptyCell}</td>
              <td className="whitespace-nowrap">{lastSeen(row.last_seen)}</td>
              <td>
                {row.source || emptyCell}
                {row.via_uplink && (
                  <span className="ml-2 whitespace-nowrap" title={t('ui.m222')}>
                    <Badge text={t('ui.m223')} />
                  </span>
                )}
              </td>
            </tr>;
          })}
        </tbody>
      </table>
    </div>
  );
}

export function Devices(p: ViewProps) {
  const [scope, setScope] = useState<DeviceScope>('all');
  const [q, setQuery] = useState('');
  const [group, setGroup] = useState<string>();
  const [importOpen, setImportOpen] = useState(false);
  const [notice, setNotice] = useState('');
  const [revision, setRevision] = useState(0);
  const preset = asArray(p.settings?.views).find(view => view.id === p.part);
  const vlans = asArray(preset?.vlans);
  const isReplay = p.replay !== null;
  const replay = p.replayRef;
  const enabled = !isReplay || Boolean(replay);
  const groups = isReplay ? [...new Set(asArray(p.replay).map(sw => sw.group_tag))] :
    asArray(p.operations?.groups).map(item => item.name);
  const selectedGroup = group !== undefined && groups.includes(group) ? group : undefined;
  const filters = { scope, q, vlans, group: selectedGroup };
  const url = enabled ? deviceUrl(filters, replay) : '';
  const profileId = p.status?.profile.id;
  const key = JSON.stringify([url, profileId, revision]);
  const inventory = usePolling(async signal => ({ key, rows: await api<InventoryRow[]>(url, { signal }) }),
    isReplay ? 0 : STATE_REFRESH_MS, [key], enabled, 300);
  const rows = inventory.data?.key === key ? asArray(inventory.data.rows) : [];
  const [manual, setManual] = useState('');
  const [manualResults, setManualResults] = useState<BulkPingResult[]>([]);
  const targets = pingTargets(rows);
  const pingDisabledReason = isReplay ? t('ui.m224')
    : p.locked ? t('ui.m225')
    : targets.length === 0 ? t('ui.m226')
      : targets.length > MAX_BULK_PING ? t('ui.m227', { p0: MAX_BULK_PING }) : undefined;
  const manualDisabledReason = isReplay ? t('ui.m224')
    : p.locked ? t('ui.m225') : undefined;
  const runPing = (body: { ips?: string[]; manual?: string }) => p.run(async () => {
    setNotice('');
    const r = await api<BulkPingResponse>('/api/diagnostics/ping-bulk', json('POST', body),
      bulkPingTimeoutMs(body.ips ? body.ips.length : MAX_BULK_PING));
    const listed = new Set(pingTargets(rows));
    setManualResults(asArray(r.results).filter(x => !listed.has(x.ip)));
    setNotice(t('ui.m228', { p0: r.ok, p1: r.total, p2: (r.elapsed_ms / 1000).toFixed(1) }));
    setRevision(value => value + 1);
  });
  const importDisabledReason = isReplay ? t('ui.m229')
    : p.locked ? t('ui.m230') : undefined;

  return (
    <section className="panel space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div role="group" aria-label={t('ui.m231')} className="flex gap-1">
          <button aria-pressed={scope === 'all'} onClick={() => setScope('all')}>{t('ui.m232')}</button>
          <button aria-pressed={scope === 'unnamed'} onClick={() => setScope('unnamed')}>{t('ui.m233')}</button>
        </div>
        <label>{t('ui.m234')}<input type="search" value={q} onChange={event => setQuery(event.target.value)} /></label>
        <label>{t('operations.group')} <select value={selectedGroup === undefined ? '' : `g:${selectedGroup}`}
          onChange={event => setGroup(event.target.value ? event.target.value.slice(2) : undefined)}>
          <option value="">{t('rack.all')}</option>
          {groups.map(name => <option key={name} value={`g:${name}`}>{name || t('rack.ungrouped')}</option>)}
        </select></label>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        {enabled && <>
          <a className="underline" href={deviceUrl({ ...filters, scope: 'all' }, replay, true)} download>{t('ui.m235')}</a>
          <a className="underline" href={deviceUrl({ ...filters, scope: 'unnamed' }, replay, true)} download>{t('ui.m236')}</a>
        </>}
        <button disabled={Boolean(importDisabledReason) || p.busy} title={importDisabledReason} onClick={() => {
          setNotice('');
          setImportOpen(true);
        }}>{t('ui.m237')}</button>
        {importDisabledReason && <span className="text-sm text-slate-500 dark:text-slate-400">{importDisabledReason}</span>}
      </div>
      {!isReplay && (
        <div className="flex flex-wrap items-center gap-3">
          <button disabled={Boolean(pingDisabledReason) || p.busy} title={pingDisabledReason}
            onClick={() => runPing({ ips: targets })}>{t('ui.m238')}{targets.length}{t('ui.m239')}</button>
          <label>{t('ui.m240')}<input
            value={manual}
            placeholder="192.168.1.10, 192.168.1.20-40, 10.0.0.0/28"
            disabled={Boolean(manualDisabledReason)}
            onChange={event => setManual(event.target.value)}
          /></label>
          <button disabled={Boolean(manualDisabledReason) || !manual.trim() || p.busy} title={manualDisabledReason} onClick={() => {
            if (window.confirm(t('ui.m241', { p0: MAX_BULK_PING, p1: manual.trim() }))) runPing({ manual });
          }}>{t('ui.m242')}</button>
          {(pingDisabledReason || manualDisabledReason) && (
            <span className="text-sm text-slate-500 dark:text-slate-400">{manualDisabledReason || pingDisabledReason}</span>
          )}
        </div>
      )}
      <p className="text-sm text-slate-500 dark:text-slate-400">{t('ui.m243')}</p>
      {notice && <p role="status">✓ {notice}</p>}
      {manualResults.length > 0 && (
        <div className="overflow-auto">
          <h3 className="font-bold">{t('ui.m244')}{manualResults.length}{t('ui.m096')}</h3>
          <table className="text-sm">
            <thead><tr><th scope="col">IP</th><th scope="col">{t('ui.m245')}</th><th scope="col">{t('ui.m085')}</th></tr></thead>
            <tbody>
              {manualResults.map(x => <tr key={x.ip}><td className="font-mono">{x.ip}</td><td>{pingLabel(x.status, x.rtt_ms)}</td><td>{x.detail || emptyCell}</td></tr>)}
            </tbody>
          </table>
        </div>
      )}
      {enabled ? <PollingStatus {...inventory} /> : <p role="status">{t('ui.m246')}</p>}
      {inventory.data?.key === key && rows.length === 0 && <p>{t('ui.m247')}</p>}
      <DeviceInventoryTable rows={rows} alerts={isReplay ? [] : p.operations?.alerts} onNavigate={p.openPortMonitor} />
      {importOpen && !importDisabledReason && (
        <DeviceCsvImportDialog
          key={profileId}
          busy={p.busy}
          run={p.run}
          close={() => setImportOpen(false)}
          onImported={result => {
            setImportOpen(false);
            setNotice(t('ui.m248', { p0: importSummary(result) }));
            setRevision(value => value + 1);
          }}
        />
      )}
    </section>
  );
}
