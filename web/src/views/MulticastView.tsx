import { getLang, isRecommendationEvent, t } from '../i18n';
import { api } from '../api';
import { AvStatus } from '../components/AvStatus';
import { PollingStatus } from '../components/PollingStatus';
import { Table } from '../components/Table';
import { asArray } from '../format';
import type { Multicast, SwitchState } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';
import { hasGroupFilter, inGroupScope } from './props';
import { enabledStatus, findingColumns, localizedStatus, switchColumn } from './detailTables';

const hasLastQuerier = (address?: string) => !!address && address !== '0.0.0.0' && address !== '::';

export function VlanQuerierTable({ rows, networkRows = rows }: { rows: Multicast['vlans']; networkRows?: Multicast['vlans'] }) {
  // The querier's own "last querier" can be a stale value heard before the election (seen on M4250, 2026-11-27),
  // so it counts only while more than one switch claims to be the querier. Same rule as the server-side audit.
  const queriersByVlan = new Map<number, { switches: Set<number>; addresses: Set<string>; selfAddresses: Set<string> }>();
  for (const row of networkRows) {
    const queriers = queriersByVlan.get(row.vlan) ?? { switches: new Set<number>(), addresses: new Set<string>(), selfAddresses: new Set<string>() };
    if (row.querier_oper === 'querier') queriers.switches.add(row.switch_id);
    if (hasLastQuerier(row.last_querier)) (row.querier_oper === 'querier' ? queriers.selfAddresses : queriers.addresses).add(row.last_querier);
    queriersByVlan.set(row.vlan, queriers);
  }
  for (const queriers of queriersByVlan.values()) {
    if (queriers.switches.size > 1) queriers.selfAddresses.forEach(address => queriers.addresses.add(address));
  }
  const byVlan = new Map<number, (typeof rows)[number][]>();
  for (const row of rows) {
    const items = byVlan.get(row.vlan);
    if (items) items.push(row); else byVlan.set(row.vlan, [row]);
  }
  const summary = [...byVlan].sort(([a], [b]) => a - b).map(([vlan, members]) => {
    const queriers = queriersByVlan.get(vlan);
    const conflict = queriers && (queriers.switches.size > 1 || queriers.addresses.size > 1);
    const querierOK = conflict ? 0 : members.filter(row => row.querier_oper === 'querier' || hasLastQuerier(row.last_querier)).length;
    const snoopingOK = members.filter(row => row.snooping).length;
    return t('pages.vlanSummary', { vlan, querierOK, querierReview: members.length - querierOK,
      snoopingOK, snoopingReview: members.length - snoopingOK });
  });
  return <>
    {/* One line per VLAN so the summary can be scanned. */}
    {summary.length > 0 && <ul className="multicast-summary">{summary.map(line => <li key={line}>{line}</li>)}</ul>}
    <Table rows={rows} first={['switch_name', 'vlan', 'name', 'snooping', 'querier_oper', 'querier_addr']}
      hide={switchColumn.hide} values={{ snooping: enabledStatus, querier_admin: enabledStatus, querier_oper: localizedStatus }} />
  </>;
}

function replayMulticast(states: SwitchState[]): Multicast {
  return {
    vlans: states.flatMap(state => asArray(state.igmp.vlans).map(value => ({ ...value, switch_id: state.id, switch_name: state.name }))),
    groups: states.flatMap(state => asArray(state.igmp.groups).map(value => ({ ...value, switch_id: state.id, switch_name: state.name }))),
    mrouters: states.flatMap(state => asArray(state.igmp.mrouters).map(value => ({ ...value, switch_id: state.id, switch_name: state.name }))),
    gptp: states.flatMap(state => state.igmp.gptp ? [{ ...state.igmp.gptp, switch_id: state.id, switch_name: state.name }] : []),
    findings: [],
  };
}

export function MulticastView(props: ViewProps) {
  const switches = asArray(props.overview.switches);
  const switchKey = switches.map(item => item.id).join(',');
  const result = usePolling(async signal => {
    
    const [multicast, states] = await Promise.all([
      api<Multicast>('/api/multicast', { signal }),
      Promise.all(switches.map(item => api<SwitchState>(`/api/switches/${item.id}/state`, { signal }))),
    ]);
    return { multicast, states };
  }, STATE_REFRESH_MS, [switchKey], !props.replay);
  const states = props.replay ? asArray(props.replay) : asArray(result.data?.states);
  const data = props.replay ? replayMulticast(states) : result.data?.multicast;
  const preset = asArray(props.settings?.views).find(view => view.id === props.part);
  const vlans = asArray(preset?.vlans);
  const tags = asArray(preset?.tags);
  const visibleStates = states.filter(state => (props.group === undefined || state.group_tag === props.group) &&
    (!tags.length || tags.includes(state.group_tag)));
  const filtered = hasGroupFilter(props);
  const visibleIds = new Set(visibleStates.map(state => state.id));
  const visible = (row: { switch_id?: number }) => inGroupScope(row, visibleIds, filtered);
  const filter = <T extends {
    vlan: number;
    switch_id: number
  }>(rows: T[] | undefined) => asArray(rows).filter(row =>
    (!vlans.length || vlans.includes(row.vlan)) && visible(row));
  return (
    <div className="panel space-y-4">
      {!props.replay && <PollingStatus {...result} />}
      <h2>{t('ui.m270')}</h2>
      <VlanQuerierTable rows={filter(data?.vlans)} networkRows={asArray(data?.vlans)} />
      <h2>{t('ui.m271')}</h2>
      {/* スイッチがMACしか返さないときは、既知の用途から推定したIPに「推定」と添える */}
      <Table {...switchColumn} rows={filter(data?.groups).map(({ purpose_ja, purpose_en, ip_guessed, ...row }) => ({
        ...row, ip: row.ip ? `${row.ip}${ip_guessed ? t('mcast.guessed') : ''}` : '',
        purpose: (getLang() === 'en' ? purpose_en : purpose_ja) || '' }))}
        first={['switch_name', 'vlan', 'group', 'ip', 'protocol', 'purpose']} />
      {asArray(data?.groups).some(row => row.ip_guessed) && <p className="chart-help">{t('mcast.guessedHelp')}</p>}
      <h2>{t('ui.m272')}</h2>
      <Table {...switchColumn} rows={filter(data?.mrouters)} first={['switch_name', 'vlan', 'port']} />
      <h2>gPTP</h2>
      <Table {...switchColumn} rows={asArray(data?.gptp).filter(visible)} />
      <h2>{t('ui.m273')}</h2>
      {visibleStates.map(state => <AvStatus key={state.id} state={state} />)}
      <h2>{t('ui.m253')}</h2>
      <Table {...findingColumns} expert={props.expert} rows={asArray(data?.findings).filter(row =>
        visible(row) && (props.expert || !isRecommendationEvent(row.code)))} />
    </div>
  );
}
