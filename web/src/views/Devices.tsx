import { t } from '../i18n';
import { useState } from 'react';
import { api, download, json } from '../api';
import { Badge } from '../components/Badge';
import { DeviceCsvImportDialog } from '../components/DeviceCsvImportDialog';
import { confirmBulkPing } from '../components/GroupActions';
import { PingButton } from '../components/PingButton';
import { PollingStatus } from '../components/PollingStatus';
import { pruneSwitchFilter, SwitchFilters } from '../components/SwitchFilters';
import { bulkPingTimeoutMs, deviceUrl, importSummary, MAX_BULK_PING, pingLabel, pingTargets, visibleDeviceCsv } from '../deviceCsv';
import type { DeviceScope } from '../deviceCsv';
import { asArray, decimal, formatTime } from '../format';
import { pcTimestamp } from '../pcTime';
import type { BulkPingResult, BulkPingResponse, InventoryRow } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import type { SwitchFilter, ViewProps } from './props';
import { emptySwitchFilter, hasGroupFilter, matchesSwitch, partPreset } from './props';
import { alertStates, alertTone, matchingDeviceAlerts, summarizeAlerts } from '../wallAlerts';
import type { Alert } from '../types';

const emptyCell = '—';

function lastSeen(value: string | undefined): string {
  
  return value && value !== '0001-01-01T00:00:00Z' ? formatTime(value) : t('ui.m217');
}

export function DeviceInventoryTable({ rows, alerts = [], onNavigate, pingDisabled = true, onPinged }: {
  rows: InventoryRow[]; alerts?: Alert[]; onNavigate?: ViewProps['openPortMonitor'];
  // Per-row Ping; disabled in replay, while locked, or when no callback refreshes the recorded result.
  pingDisabled?: boolean; onPinged?: () => void;
}) {
  const columns = [t('rack.connectionState'), t('pages.deviceName'), t('pages.switchPort'), t('pages.deviceIP'),
    'Ping', t('ui.m074'), t('pages.vlanPoe'), 'MAC'];
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
                {row.label ? <strong>{row.label}</strong> :
                  <span className="device-secondary">{row.lldp_name || emptyCell}</span>}
                {!row.seen && <span className="ml-2 whitespace-nowrap"><Badge text={t('ui.m218')} /></span>}
                {row.via_uplink && <span className="ml-2 whitespace-nowrap" title={t('ui.m222')}><Badge text={t('ui.m223')} /></span>}
              </td>
              <td className="whitespace-nowrap">
                {row.switch_name || emptyCell}
                {row.port && <> / {navigate ? <button onClick={event => { event.stopPropagation(); navigate(); }} aria-label={`${t('rack.nav.monitor')} · ${row.switch_name} ${row.port}`}>{row.port}</button> : row.port}</>}
                {!row.port && navigate && <button onClick={event => { event.stopPropagation(); navigate(); }} aria-label={`${t('rack.nav.monitor')} · ${row.switch_name}`}>↗</button>}
              </td>
              <td className="whitespace-nowrap">
                <div>{row.ip || emptyCell}
                  {row.ip && row.ip_stale && <span className="ml-2" title={t('ui.m219')}><Badge text={t('ui.m220')} /></span>}
                </div>
                <span className="device-secondary" title={t('rack.plannedIP')}>{row.planned_ip || emptyCell}</span>
                {row.ip_mismatch && <span className="text-amber-700 dark:text-amber-300" title={t('operations.mismatch')} aria-label={t('operations.mismatch')}> ▲</span>}
              </td>
              <td className="whitespace-nowrap">
                {row.ping ? pingLabel(row.ping.status, row.ping.rtt_ms) :
                  row.ping_ok == null ? emptyCell : pingLabel(row.ping_ok ? 'ok' : 'timeout', row.ping_rtt_ms ?? 0)}
                {(row.ping_ts || row.ping?.at) && <small className="device-secondary block" title={t('operations.pingAt')}>{pcTimestamp(row.ping_ts || row.ping?.at || '')}</small>}
                {row.ip && !pingDisabled && <PingButton key={row.ip} ip={row.ip} showResult={false} onDone={onPinged} />}
              </td>
              <td>{row.vendor || emptyCell}</td>
              <td className="whitespace-nowrap">{row.vlan || emptyCell} / {row.poe_w != null ? `${decimal(row.poe_w)} W` : emptyCell}</td>
              <td className="whitespace-nowrap font-mono" title={[
                row.lldp_name && row.label && `${t('ui.m213')} ${row.lldp_name}`,
                row.speed_mbps > 0 && `${row.speed_mbps} Mbps`, row.source,
                `${t('ui.m215')} ${lastSeen(row.last_seen)}`,
              ].filter(Boolean).join(' · ')}>{row.mac}</td>
            </tr>;
          })}
        </tbody>
      </table>
    </div>
  );
}

export function Devices(p: ViewProps & { initialQuery?: string }) {
  const [scope, setScope] = useState<DeviceScope>('all');
  const [q, setQuery] = useState(p.initialQuery ?? '');
  const [importOpen, setImportOpen] = useState(false);
  const [notice, setNotice] = useState('');
  const [revision, setRevision] = useState(0);
  const [storedFilter, setSwitchFilter] = useState<SwitchFilter>(emptySwitchFilter);
  // Ignore filter choices that no longer exist so the list never stays empty on a stale selection.
  const switchFilter = pruneSwitchFilter(storedFilter, asArray(p.replay ?? p.overview?.switches),
    p.replay ? undefined : asArray(p.operations?.groups).map(group => group.name), p.settings?.views) ?? storedFilter;
  const preset = partPreset({ ...switchFilter, settings: p.settings });
  const filtered = hasGroupFilter(switchFilter);
  const tags = asArray(preset?.tags);
  const vlans = asArray(preset?.vlans);
  const isReplay = p.replay !== null;
  const replay = p.replayRef;
  const enabled = !isReplay || Boolean(replay);
  const filters = { scope: filtered ? 'all' as const : scope, q, vlans: [] };
  const url = enabled ? deviceUrl(filters, replay) : '';
  const profileId = p.status?.profile.id;
  const key = JSON.stringify([url, profileId, revision]);
  const inventory = usePolling(async signal => ({ key, rows: await api<InventoryRow[]>(url, { signal }) }),
    isReplay ? 0 : STATE_REFRESH_MS, [key], enabled, 300);
  const matchingRows = inventory.data?.key === key ? asArray(inventory.data.rows).filter(row =>
    matchesSwitch(switchFilter, tags, row.switch_id, row.group_tag) &&
    (!row.switch_id || !vlans.length || vlans.includes(row.vlan))) : [];
  const unnamedRows = matchingRows.filter(row => !row.label);
  const rows = scope === 'unnamed' ? unnamedRows : matchingRows;
  const phase = p.operations?.phase || p.status?.phase;
  const [manual, setManual] = useState('');
  const [noReply, setNoReply] = useState<BulkPingResult[]>([]);
  const [pinging, setPinging] = useState(0);
  // Ping target: shown devices, one group, or manual entry. The choice remembers the phase it was made in,
  // so entering the show phase (or learning the phase after mount) switches to manual entry.
  const [pingChoice, setPingChoice] = useState<{ target: string; phase?: string }>({ target: phase === 'show' ? 'manual' : 'visible', phase });
  const chosenTarget = phase === 'show' && pingChoice.phase !== 'show' ? 'manual' : pingChoice.target;
  const setPingTarget = (target: string) => setPingChoice({ target, phase });
  const groupTargets = [...new Set(rows.map(row => row.group_tag).filter(Boolean))]
    .map(name => ({ name, ips: pingTargets(rows.filter(row => row.group_tag === name)) }));
  const chosenGroup = chosenTarget.startsWith('group:') ? groupTargets.find(group => `group:${group.name}` === chosenTarget) : undefined;
  // A group that is no longer shown falls back to manual entry, never to "all shown devices".
  const pingTarget = chosenTarget.startsWith('group:') && !chosenGroup ? 'manual' : chosenTarget;
  const targets = chosenGroup ? chosenGroup.ips : pingTargets(rows);
  const manualMode = pingTarget === 'manual';
  const pingDisabledReason = isReplay ? t('ui.m224')
    : p.locked ? t('ui.m225')
    : targets.length === 0 ? t('ui.m226')
      : targets.length > MAX_BULK_PING ? t('ui.m227', { p0: MAX_BULK_PING }) : undefined;
  const manualDisabledReason = isReplay ? t('ui.m224')
    : p.locked ? t('ui.m225') : undefined;
  const runPing = (body: { ips?: string[]; manual?: string }, count: number) => p.run(async () => {
    setNotice('');
    setNoReply([]);
    // Positive: number of targets; -1: manual entry whose count the server decides.
    setPinging(count);
    try {
      const r = await api<BulkPingResponse>('/api/diagnostics/ping-bulk', json('POST', body),
        bulkPingTimeoutMs(body.ips ? body.ips.length : MAX_BULK_PING));
      // One summary line; list only the addresses that did not answer.
      setNoReply(asArray(r.results).filter(x => x.status !== 'ok'));
      setNotice(t('ping.summary', { ok: r.ok, ng: r.total - r.ok, sec: (r.elapsed_ms / 1000).toFixed(1) }));
      setRevision(value => value + 1);
    } finally {
      setPinging(0);
    }
  });
  const startPing = () => {
    if (manualMode) {
      const summary = t('ui.m241', { p0: MAX_BULK_PING, p1: manual.trim() });
      if (window.confirm(phase === 'show' ? `${t('pages.pingLoad')}
${summary}` : summary)) runPing({ manual }, -1);
    } else if (confirmBulkPing(targets.length, phase)) runPing({ ips: targets }, targets.length);
  };
  const runDisabledReason = manualMode ? manualDisabledReason : pingDisabledReason;
  const importDisabledReason = isReplay ? t('ui.m229')
    : p.locked ? t('ui.m230') : undefined;

  return (
    <section className="panel space-y-3">
      <SwitchFilters filter={switchFilter} onChange={setSwitchFilter} parts={p.settings?.views}
        switches={asArray(p.replay ?? p.overview?.switches)}
        groups={isReplay ? undefined : asArray(p.operations?.groups).map(group => group.name)} />
      <div className="flex flex-wrap items-end gap-3">
        <div role="group" aria-label={t('ui.m231')} className="flex gap-1">
          <button aria-pressed={scope === 'all'} onClick={() => setScope('all')}>{t('ui.m232')}</button>
          <button aria-pressed={scope === 'unnamed'} onClick={() => setScope('unnamed')}>{t('ui.m233')}</button>
        </div>
        <label>{t('ui.m234')}<input type="search" value={q} onChange={event => setQuery(event.target.value)} /></label>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        {enabled && (filtered ? <>
          <button disabled={inventory.data?.key !== key}
            onClick={() => download(visibleDeviceCsv(matchingRows), 'avline-sight-devices.csv')}>{t('ui.m235')}</button>
          <button disabled={inventory.data?.key !== key}
            onClick={() => download(visibleDeviceCsv(unnamedRows), 'avline-sight-devices-unnamed.csv')}>{t('ui.m236')}</button>
        </> : <>
          <a className="button-link" href={deviceUrl({ ...filters, scope: 'all' }, replay, true)} download>{t('ui.m235')}</a>
          <a className="button-link" href={deviceUrl({ ...filters, scope: 'unnamed' }, replay, true)} download>{t('ui.m236')}</a>
        </>)}
        <button disabled={Boolean(importDisabledReason) || p.busy} title={importDisabledReason} onClick={() => {
          setNotice('');
          setImportOpen(true);
        }}>{t('ui.m237')}</button>
        {importDisabledReason && <span className="text-sm text-slate-500 dark:text-slate-400">{importDisabledReason}</span>}
      </div>
      {!isReplay && (
        <div className="operation-area ping-row" role="group" aria-label={t('ping.target')}>
          <label>{t('ping.target')}
            <select data-action="ping-target" value={pingTarget} disabled={p.locked || p.busy} onChange={event => setPingTarget(event.target.value)}>
              <option value="visible">{t('ping.visible', { count: pingTargets(rows).length })}</option>
              {groupTargets.map(group => <option key={group.name} value={`group:${group.name}`}>{t('ping.group', { name: group.name, count: group.ips.length })}</option>)}
              <option value="manual">{t('ping.manual')}</option>
            </select>
          </label>
          {manualMode && <input aria-label={t('ping.ips')} value={manual}
            placeholder="192.168.1.10, 10.0.0.0/28" title="192.168.1.10, 192.168.1.20-40, 10.0.0.0/28"
            disabled={Boolean(manualDisabledReason)} onChange={event => setManual(event.target.value)} />}
          <button data-action="ping-run" disabled={Boolean(runDisabledReason) || p.busy || (manualMode && !manual.trim())}
            title={runDisabledReason} onClick={startPing}>
            {pinging < 0 ? t('ping.running') : pinging > 0 ? t('ping.runningCount', { count: pinging }) : t('ping.run')}
          </button>
          <button type="button" className="ping-about" aria-label={t('ping.about')} title={t('ui.m243')}
            onClick={() => window.alert(t('ui.m243'))}>ⓘ</button>
          {runDisabledReason && <span className="text-sm text-slate-500 dark:text-slate-400">{runDisabledReason}</span>}
        </div>
      )}
      {notice && <p role="status">✓ {notice}</p>}
      {noReply.length > 0 && (
        <div className="overflow-auto">
          <h3 className="font-bold">{t('ping.noReply', { count: noReply.length })}</h3>
          <table className="text-sm">
            <thead><tr><th scope="col">IP</th><th scope="col">{t('ui.m245')}</th><th scope="col">{t('ui.m085')}</th></tr></thead>
            <tbody>
              {noReply.map(x => <tr key={x.ip}><td className="font-mono">{x.ip}</td><td>{pingLabel(x.status, x.rtt_ms)}</td><td>{x.detail || emptyCell}</td></tr>)}
            </tbody>
          </table>
        </div>
      )}
      {enabled ? <PollingStatus {...inventory} /> : <p role="status">{t('ui.m246')}</p>}
      {inventory.data?.key === key && rows.length === 0 && <p>{t('ui.m247')}</p>}
      <DeviceInventoryTable rows={rows} alerts={isReplay ? [] : p.operations?.alerts} onNavigate={p.openPortMonitor}
        pingDisabled={isReplay || p.locked} onPinged={() => void inventory.refresh()} />
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
