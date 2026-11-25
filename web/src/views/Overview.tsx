import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertBand } from '../components/AlertBand';
import { DiscoveryWizard, LabPanel, SwitchForm } from '../components/Dialogs';
import { GroupActions, PingOutcomeView } from '../components/GroupActions';
import type { PingOutcome } from '../components/GroupActions';
import { GroupEditDialog } from '../components/GroupEditDialog';
import type { GroupEdit } from '../components/GroupEditDialog';
import { PortDetail } from '../components/PortDetail';
import { PortLegend, RackRow } from '../components/RackRow';
import { api } from '../api';
import { asArray } from '../format';
import { t } from '../i18n';
import { sortedAlerts, uplinkPorts, withoutUnusedLags } from '../rackModel';
import type { Alert, InventoryRow, Port, SwitchConfig, SwitchSummary } from '../types';
import { useRackData } from '../useRackData';
import { registeredSummary, replaySummary } from './rackSummaries';
import type { PortMonitorTarget, SwitchFilter, ViewProps } from './props';
import { hasGroupFilter, inGroupScope, matchesSwitch, partPreset } from './props';
import type { Operations } from '../useOperations';
import { PortMatrix } from './PortMatrix';
import { WallSwitchTile } from '../components/WallSwitchTile';
import { hasCriticalActive, isPortAlert, matchingPortAlerts, replayIssueCount, summarizeAlerts } from '../wallAlerts';
import { ErrorSymbol } from '../components/ErrorSymbol';
import { SwitchFilters } from '../components/SwitchFilters';
import { MonitorKpis } from '../components/MonitorKpis';
import { SwitchPorts } from './SwitchPorts';
import { emptySwitchFilter } from './props';

const collapseKey = 'rack-collapsed';
function readCollapsed() {
  try {
    return new Set(asArray<string[]>(JSON.parse(localStorage.getItem(collapseKey) || '[]')).filter(name => typeof name === 'string'));
  } catch {
    return new Set<string>();
  }
}

type Sample = { rx: number; tx: number };
function useUplinkSamples(ports: { switch_id: number; port: Port }[] | undefined, uplinks: Set<string>) {
  const [samples, setSamples] = useState<Record<string, Sample[]>>({});
  const uplinkKey = [...uplinks].sort().join('\n');
  const previousPorts = useRef(ports);
  useEffect(() => {
    const newSnapshot = previousPorts.current !== ports;
    previousPorts.current = ports;
    setSamples(previous => {
      const next: Record<string, Sample[]> = {};
      for (const { switch_id, port } of asArray(ports)) {
        const key = `${switch_id}/${port.name}`;
        if (!uplinks.has(key)) continue;
        next[key] = !newSnapshot && previous[key] ? previous[key] :
          [...(previous[key] || []).slice(-23), { rx: port.rx_bps || 0, tx: port.tx_bps || 0 }];
      }
      return next;
    });
  }, [ports, uplinkKey]);
  return samples;
}

function indexBySwitch<T extends { switch_id?: number }>(rows: T[]) {
  const index = new Map<number, T[]>();
  for (const row of rows) {
    if (row.switch_id === undefined) continue;
    if (!index.has(row.switch_id)) index.set(row.switch_id, []);
    index.get(row.switch_id)?.push(row);
  }
  return index;
}

type OverviewProps = ViewProps & {
  wall?: boolean; operations?: Operations; onTrapHelp?: () => void;
  replayAlerts?: Alert[];
  target?: PortMonitorTarget; onClosePortDetail?: () => void;
  onFilterChange?: (filter: SwitchFilter) => void;
  // Switch chosen in the index (0 or unset = all switches); single-switch view of plan section 21.
  focus?: number; onFocus?: (switchId: number) => void;
};
const noDevices: InventoryRow[] = [];

export function OverviewView(props: OverviewProps) {
  const { wall = false, operations, onTrapHelp = () => undefined, replay, locked, busy, target } = props;
  const [layout, setLayout] = useState<'diagram' | 'list'>('diagram');
  const [edit, setEdit] = useState<SwitchConfig | null>();
  const [discover, setDiscover] = useState(false);
  const [groupEdit, setGroupEdit] = useState<GroupEdit>();
  const portTarget = target?.ifIndex || target?.portName ? target : undefined;
  const [selected, setSelected] = useState(portTarget);
  useEffect(() => { setSelected(portTarget); }, [portTarget, props.group, props.part, props.switchId, props.focus]);
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [notice, setNotice] = useState('');
  // "Ping all" result per group ("*" for all switches), kept next to that group's actions until closed.
  const [pings, setPings] = useState<Record<string, PingOutcome>>({});
  const pingFor = (key: string) => (outcome: PingOutcome | undefined) => setPings(previous => {
    const next = { ...previous };
    if (outcome) next[key] = outcome; else delete next[key];
    return next;
  });
  const pingResult = (key: string) => pings[key] && <PingOutcomeView outcome={pings[key]} close={() => pingFor(key)(undefined)} openDevices={props.openDevices} />;
  const [wallSwitch, setWallSwitch] = useState<number>();
  const rack = useRackData(props.status?.profile.id, replay, props.replayRef);
  const configs = rack.data?.configs;
  const summary = useMemo(() => {
    if (replay !== null) return asArray(replay).map(replaySummary);
    const items = asArray(props.overview.switches);
    if (!configs) return items;
    const byId = new Map(items.map(item => [item.id, item]));
    return configs.map(config => ({ ...(byId.get(config.id) || registeredSummary(config)), group_tag: config.group_tag }));
  }, [replay, props.overview.switches, configs]);
  const uplinks = useMemo(() => uplinkPorts(rack.topology, summary), [rack.topology, summary]);
  const samples = useUplinkSamples(rack.data?.ports, uplinks);
  // Uplink total for the number panels: sum of uplink ports in the existing snapshot, no extra polling.
  const uplinkTotal = useMemo(() => {
    let found = false; let rx = 0; let tx = 0;
    for (const { switch_id, port } of asArray(rack.data?.ports)) {
      if (!uplinks.has(`${switch_id}/${port.name}`)) continue;
      found = true; rx += port.rx_bps || 0; tx += port.tx_bps || 0;
    }
    return found ? { rx, tx } : undefined;
  }, [rack.data?.ports, uplinks]);
  const devices = rack.data?.devices || noDevices;
  const ports = indexBySwitch(asArray(rack.data?.ports));
  const bySwitch = indexBySwitch(devices);
  const knownGroups = new Map(asArray(operations?.groups).map(group => [group.name, group]));
  const preset = partPreset(props);
  const tags = asArray(preset?.tags);
  // While one switch is shown, filters do not apply: the alert band and number panels stay whole-project.
  const focusSummary = !wall && props.focus ? summary.find(item => item.id === props.focus) : undefined;
  const groupFiltered = hasGroupFilter(props) && !focusSummary;
  const visible = summary.filter(item => matchesSwitch(props, tags, item.id, item.group_tag));
  const visibleIds = new Set(visible.map(item => item.id));
  const targetSwitch = visible.find(item => item.id === target?.switchId);
  const focusedTarget = useRef<PortMonitorTarget>();
  useEffect(() => {
    if (!target || !targetSwitch || layout !== 'diagram' || collapsed.has(targetSwitch.group_tag) || focusedTarget.current === target) return;
    const row = document.getElementById(`rack-switch-${target.switchId}`);
    if (row) {
      row.focus();
      focusedTarget.current = target;
    }
  }, [target, targetSwitch, layout, collapsed]);
  useEffect(() => {
    if (!targetSwitch) return;
    setCollapsed(previous => {
      if (!previous.has(targetSwitch.group_tag)) return previous;
      const next = new Set(previous);
      next.delete(targetSwitch.group_tag);
      return next;
    });
  }, [targetSwitch?.id, targetSwitch?.group_tag]);
  const byId = new Map(summary.map(item => [item.id, item]));
  const live = replay === null;
  const allAlerts = asArray(live ? operations?.alerts : props.replayAlerts);
  const visibleAlert = (alert: Alert) => inGroupScope(alert, visibleIds, groupFiltered);
  const outsideCount = !groupFiltered ? 0 : !live && props.replayAlerts === undefined
    ? replayIssueCount(asArray(replay).filter(sw => sw.id && !visibleIds.has(sw.id)))
    : summarizeAlerts(allAlerts.filter(alert => !visibleAlert(alert))).count;
  const outsideNote = outsideCount > 0 && <p role="status">{t('cleanup.outsideGroup', { count: outsideCount })}</p>;
  const alerts = sortedAlerts(allAlerts.filter(visibleAlert).map(alert => {
      const portAlert = isPortAlert(alert);
      const port = portAlert ? ports.get(alert.switch_id)?.find(row => row.port.if_index === alert.if_index)?.port : undefined;
      const devices = bySwitch.get(alert.switch_id);
      const device = alert.mac ? devices?.find(row => row.mac === alert.mac) :
        portAlert ? devices?.find(row => row.if_index === alert.if_index && row.registered) || devices?.find(row => row.if_index === alert.if_index) : undefined;
      return {
        ...alert,
        switch_id: alert.switch_id ?? 0,
        switch_name: alert.switch_name || byId.get(alert.switch_id)?.name || '',
        port: portAlert ? alert.port || port?.name || `#${alert.if_index}` : '',
        device_name: alert.device_name || device?.label || device?.lldp_name || device?.mac ||
          asArray(port?.devices)[0]?.label || asArray(port?.devices)[0]?.lldp_name || asArray(port?.devices)[0]?.mac,
      };
    }));
  const byAlert = indexBySwitch(alerts);
  const groupRows = new Map<string, SwitchSummary[]>();
  for (const sw of visible) {
    if (!groupRows.has(sw.group_tag)) groupRows.set(sw.group_tag, []);
    groupRows.get(sw.group_tag)?.push(sw);
  }
  const groups = [...knownGroups.keys()].filter(name => groupRows.has(name));
  groups.push(...[...groupRows.keys()].filter(name => !knownGroups.has(name)).sort());
  if (groups.includes('')) groups.push(groups.splice(groups.indexOf(''), 1)[0]);
  const allGroupNames = [...new Set([...knownGroups.keys(), ...summary.map(item => item.group_tag)])];
  const saveCollapsed = (next: Set<string>) => {
    setCollapsed(next);
    try { localStorage.setItem(collapseKey, JSON.stringify([...next])); } catch { return; }
  };
  const refresh = async () => {
    await Promise.all([rack.refresh(), operations?.refresh()]);
  };
  const selectedSwitch = selected && visible.find(item => item.id === selected.switchId);
  const selectedPort = selected && ports.get(selected.switchId)?.find(row =>
    selected.ifIndex ? row.port.if_index === selected.ifIndex : row.port.name === selected.portName)?.port;
  const profileId = props.status?.profile.id;
  const actionProps = { locked, wall, busy, devices, run: props.run, refresh, notice: setNotice, replay: replay !== null, profileId,
    phase: operations?.phase || props.status?.phase };
  const wallSummary = summarizeAlerts(alerts);
  const wallTiles = [...visible];
  for (const alert of wallSummary.items) {
    if (!wallTiles.some(sw => sw.id === alert.switch_id)) wallTiles.push({
      ...registeredSummary({ id: alert.switch_id, name: alert.switch_id ? alert.switch_name || t('wall.global') : t('wall.global'), ip: '', group_tag: '' } as SwitchConfig),
      reachable: true,
    });
  }
  wallTiles.sort((a, b) => {
    const rank = (sw: SwitchSummary) => {
      const current = summarizeAlerts(byAlert.get(sw.id) || []);
      return current.active ? 0 : current.unacked ? 1 : !sw.reachable ? 2 : 3;
    };
    return rank(a) - rank(b);
  });
  const wallFilter = wallTiles.find(sw => sw.id === wallSwitch);
  if (wall) return <div className="wall-monitor">
    {wallFilter && <div className="wall-filter-note">{t('wall.filter', { name: wallFilter.name })} <button onClick={() => setWallSwitch(undefined)}>{t('wall.clearFilter')}</button></div>}
    {outsideNote}
    {/* Switch state first: it is what the wall is read for from a distance. */}
    <h2 className="wall-switches-heading">{t('wall.current')}</h2>
    <div className="wall-switches" style={{ '--switch-count': visible.length } as React.CSSProperties}>
      {!visible.length && <p>{rack.data ? t('rack.empty') : t('rack.loading')}</p>}
      {wallTiles.map(sw => <WallSwitchTile key={sw.id} summary={sw} alerts={byAlert.get(sw.id) || []} expert={props.expert}
        stale={!!rack.error} selected={wallFilter?.id === sw.id} onSelect={() => setWallSwitch(current => current === sw.id ? undefined : sw.id)} />)}
    </div>
    {(live || props.replayAlerts !== undefined) && <AlertBand alerts={wallFilter ? alerts.filter(alert => !alert.switch_id || alert.switch_id === wallFilter.id) : alerts}
      wall expert={props.expert} pending={live && !operations?.ready} busy={busy} locked={locked} ackMany={live ? operations?.ackMany : undefined}
      undoCount={live ? operations?.undoCount : undefined} undo={live ? operations?.undoAck : undefined} />}
  </div>;
  // Legacy replays carry no alert list, so fault counts are unknown rather than zero.
  const kpis = <MonitorKpis switches={summary} alerts={allAlerts} uplink={uplinkTotal} alertsKnown={live || props.replayAlerts !== undefined} />;
  const band = (live || props.replayAlerts !== undefined) && <AlertBand alerts={alerts} wall={wall} expert={props.expert} busy={busy} locked={locked} ack={live ? operations?.ack : undefined}
    ackMany={live && groupFiltered ? operations?.ackMany : undefined} undoCount={live ? operations?.undoCount : undefined}
    undo={live ? operations?.undoAck : undefined} pending={live && !operations?.ready && !operations?.alerts.length} />;
  // Edit form for one switch; deleting the switch shown alone returns to all switches.
  const switchForm = !wall && !locked && edit !== undefined && <SwitchForm value={edit || undefined} groups={allGroupNames} busy={busy} run={props.run}
    close={() => { setEdit(undefined); void refresh().catch(() => undefined); }}
    onDeleted={edit ? () => { if (focusSummary?.id === edit.id) props.onFocus?.(0); } : undefined} />;
  if (focusSummary) return <div className="rack-overview">
    {kpis}
    {band}
    {operations?.toast && <div className="rack-toast" role="status">✓ {operations.toast}</div>}
    {/* Breadcrumb: all > group > switch; the links return to all switches. */}
    <nav className="focus-crumb" aria-label={t('focus.breadcrumb')}>
      <button type="button" className="focus-crumb-link" onClick={() => props.onFocus?.(0)}>{t('focus.all')}</button>
      <span className="focus-crumb-sep" aria-hidden="true">＞</span>
      <button type="button" className="focus-crumb-link" onClick={() => {
        props.onFocus?.(0);
        props.onFilterChange?.({ group: focusSummary.group_tag, part: '' });
      }}>{focusSummary.group_tag || t('rack.ungrouped')}</button>
      <span className="focus-crumb-sep" aria-hidden="true">＞</span>
      <strong aria-current="page">{focusSummary.name}</strong>
    </nav>
    {rack.error && <p role="alert" className="rack-data-error">⚠ {rack.error}</p>}
    {/* Keyed by switch so port choice and form fields never carry over to another switch (a save would hit the wrong port). */}
    <SwitchPorts key={focusSummary.id} {...props} {...emptySwitchFilter} focusSummary={focusSummary} selected={focusSummary.id}
      thresholds={props.settings?.thresholds}
      select={id => props.onFocus?.(id)} initialPort={target?.switchId === focusSummary.id ? target : undefined}
      alerts={byAlert.get(focusSummary.id) || []} onClosePort={props.onClosePortDetail}
      onEdit={locked || replay ? undefined : () => props.run(async () => {
        const config = asArray(await api<SwitchConfig[]>('/api/switches')).find(item => item.id === focusSummary.id);
        if (config) setEdit(config);
      })} />
    {switchForm}
  </div>;
  return <div className="rack-overview">
    {!wall && kpis}
    {outsideNote}
    {band}
    {!wall && operations?.toast && <div className="rack-toast" role="status">✓ {operations.toast}</div>}
    {!wall && <div className="rack-toolbar">
      <div role="group" aria-label={t('cleanup.layout')}>
        <button aria-pressed={layout === 'diagram'} onClick={() => setLayout('diagram')}>{t('cleanup.diagram')}</button>
        <button aria-pressed={layout === 'list'} onClick={() => setLayout('list')}>{t('cleanup.list')}</button>
      </div>
      <SwitchFilters filter={{ group: props.group, part: props.part ?? '', switchId: props.switchId }}
        onChange={props.onFilterChange ?? (() => undefined)} switches={summary}
        groups={live ? allGroupNames : undefined} parts={props.settings?.views} />
    </div>}
    {!locked && <div className="operation-area" role="group" aria-label={t('rack.actions')}>
      {/* Bulk actions appear only when their target matches what is shown (never act on all or a whole group while filtered to a switch or part). */}
      {props.group === undefined && !preset?.tags?.length && !preset?.vlans?.length && props.switchId === undefined && <>
        <strong>{t('rack.all')}</strong><GroupActions {...actionProps} group="*" all onPing={pingFor('*')} />
      </>}
      {props.group !== undefined && props.switchId === undefined && layout === 'list' && <GroupActions {...actionProps} group={props.group} onPing={pingFor(props.group)} />}
      <button disabled={busy} onClick={() => setEdit(null)}>{t('rack.add')}</button>
      <button disabled={busy} onClick={() => setDiscover(true)}>{t('rack.discover')}</button>
    </div>}
    {!locked && !wall && (props.group === undefined ? pingResult('*') : layout === 'list' && pingResult(props.group))}
    {!wall && notice && <div role="status" className="panel flex items-center justify-between gap-2">{notice}<button onClick={() => setNotice('')}>{t('rack.close')}</button></div>}
    {rack.error && <p role="alert" className="rack-data-error">⚠ {rack.error}</p>}
    {layout === 'list' ? <PortMatrix {...props} visibleSwitchIds={visibleIds} /> : <><PortLegend /><div className="rack-groups">
      {!visible.length && <p>{rack.data ? t('rack.empty') : t('rack.loading')}</p>}
      {groups.map(name => {
        const rows = groupRows.get(name) || [];
        const isClosed = !wall && collapsed.has(name);
        const counts = summarizeAlerts(rows.flatMap(sw => byAlert.get(sw.id) || []));
        const collapse = () => {
          const next = new Set(collapsed);
          if (next.has(name)) next.delete(name); else next.add(name);
          saveCollapsed(next);
        };
        return <section className="rack-group" key={name}>
          <div className="rack-group-header">
            <h2>{name || t('rack.ungrouped')}</h2>
            <span className="rack-group-counts" aria-label={t('wall.counts', counts)}>
              <span className={hasCriticalActive(counts) ? 'rack-critical-count' : counts.active ? 'rack-caution-count' : counts.unacked ? 'rack-warning-count' : 'rack-count-zero'}>{t('pages.groupCounts', counts)}</span>
            </span>
            <button className="group-collapse" aria-label={t(isClosed ? 'rack.expand' : 'rack.collapse')} aria-expanded={!isClosed} onClick={collapse}>{isClosed ? '+' : '−'}</button>
          </div>
          {!locked && !isClosed && <div className="operation-area" role="group" aria-label={t('rack.actions')}>
            {props.switchId === undefined && <GroupActions {...actionProps} group={name} onPing={pingFor(name)} />}
            <button disabled={busy} aria-label={t('operations.rename')} onClick={() => setGroupEdit({ name })}>✎</button>
          </div>}
          {!locked && !isClosed && props.switchId === undefined && pingResult(name)}
          {!isClosed && rows.map(sw => <RackRow key={sw.id} summary={sw}
            ports={withoutUnusedLags((ports.get(sw.id) || []).map(row => row.port), byAlert.get(sw.id) || [])} devices={bySwitch.get(sw.id) || []} alerts={byAlert.get(sw.id) || []}
            definitions={rack.definitions} vlans={preset?.vlans}
            wall={wall} stale={!!rack.error} uplinks={uplinks} samples={samples} actions={props}
            onPort={port => setSelected({ switchId: sw.id, ifIndex: port.if_index })} onEdit={setEdit} />)}
        </section>;
      })}
    </div></>}
    {!wall && selected && selectedSwitch && selectedPort && <PortDetail key={`${selected.switchId}/${selectedPort.if_index}`}
      switchId={selected.switchId} switchName={selectedSwitch.name} port={selectedPort} expert={props.expert}
      devices={(bySwitch.get(selected.switchId) || []).filter(device => device.if_index === selectedPort.if_index)}
      alerts={matchingPortAlerts(byAlert.get(selected.switchId) || [], selected.switchId, selectedPort.if_index)}
      close={() => { setSelected(undefined); props.onClosePortDetail?.(); }} replay={replay !== null} />}
    {!wall && !locked && props.status?.lab && <LabPanel run={props.run} busy={busy} switches={asArray(props.overview.switches)} />}
    {switchForm}
    {!wall && !locked && discover && <DiscoveryWizard busy={busy} run={props.run}
      close={() => { setDiscover(false); void refresh().catch(() => undefined); }} />}
    {!wall && !locked && groupEdit && <GroupEditDialog edit={groupEdit} groups={allGroupNames} busy={busy} run={props.run} refresh={refresh} profileId={profileId}
      close={() => setGroupEdit(undefined)} />}
  </div>;
}
