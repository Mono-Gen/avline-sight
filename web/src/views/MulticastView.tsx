import { t } from '../i18n';
import { api } from '../api';
import { AvStatus } from '../components/AvStatus';
import { PollingStatus } from '../components/PollingStatus';
import { Table } from '../components/Table';
import { asArray } from '../format';
import type { Multicast, SwitchState } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';

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
  const visibleStates = states.filter(state => !tags.length || tags.includes(state.group_tag));
  const visibleIds = new Set(visibleStates.map(state => state.id));
  const filter = <T extends {
    vlan: number;
    switch_id: number
  }>(rows: T[] | undefined) => asArray(rows).filter(row =>
    (!vlans.length || vlans.includes(row.vlan)) && (!tags.length || visibleIds.has(row.switch_id)));
  return (
    <div className="panel space-y-4">
      {!props.replay && <PollingStatus {...result} />}
      <h2>{t('ui.m270')}</h2>
      <Table rows={filter(data?.vlans)} />
      <h2>{t('ui.m271')}</h2>
      <Table rows={filter(data?.groups)} />
      <h2>{t('ui.m272')}</h2>
      <Table rows={filter(data?.mrouters)} />
      <h2>gPTP</h2>
      <Table rows={asArray(data?.gptp).filter(row => !tags.length || visibleIds.has(row.switch_id))} />
      <h2>{t('ui.m273')}</h2>
      {visibleStates.map(state => <AvStatus key={state.id} state={state} />)}
      <h2>{t('ui.m253')}</h2>
      <Table rows={asArray(data?.findings).filter(row => !tags.length || visibleIds.has(row.switch_id))} />
    </div>
  );
}
