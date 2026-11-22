import { eventHint, eventTitle, t } from '../i18n';
import { useState } from 'react';
import { api } from '../api';
import { acknowledgeEvents } from '../acknowledge';
import { confirmAck } from '../useOperations';
import { Badge } from '../components/Badge';
import { PollingStatus } from '../components/PollingStatus';
import type { EventFilters } from '../events';
import { acknowledgeableEventIds, canAcknowledgeEvent, logRows, SEARCH_EVENT_LIMIT, syslogMessage } from '../events';
import { asArray } from '../format';
import { pcTimestamp } from '../pcTime';
import type { Event } from '../types';
import { usePolling } from '../usePolling';
import type { ViewProps } from './props';
import { groupSwitches, hasGroupFilter } from './props';

const categories = ['ANOMALY', 'CONNECT', 'TRAP', 'SYSLOG', 'BASELINE', 'MARKER', 'SYSTEM'];
const severities = { CRITICAL: 'ui.m005', WARNING: 'ui.m006', INFO: 'ui.m007' } as const;

export function LogEventRow({ event, ackable, disabled, onAcknowledge }: {
  event: Event; ackable: ReadonlySet<number>; disabled: boolean; onAcknowledge: (id: number) => void;
}) {
  const syslog = event.category === 'SYSLOG';
  return <tr>
    <td className="whitespace-nowrap"><time dateTime={event.ts}>{pcTimestamp(event.ts, true)}</time></td>
    <td><Badge text={event.severity} /></td><td>{event.category}</td>
    <td>{event.switch_name}</td>
    <td>{syslog ? <details className="log-message">
      <summary>{syslogMessage(event.title)}</summary>
      <pre>{event.title}</pre>
    </details> : <>{eventTitle(event)} {eventHint(event)}</>}</td>
    <td className="whitespace-nowrap">{event.acked && event.severity !== 'INFO' ? t('ui.m269') : canAcknowledgeEvent(event, ackable) && <button
      className="button-primary" data-action="ack" disabled={disabled} onClick={() => onAcknowledge(event.id)}
    >{t('ui.m268')}</button>}</td>
  </tr>;
}

export function Logs(props: ViewProps) {
  const [localFilter, setFilter] = useState<EventFilters>({
    category: '',
    severity: '',
    switch_id: '',
    q: '',
    from: '',
    to: ''
  });
  const groupFiltered = hasGroupFilter(props);
  const groupMembers = groupSwitches(props);
  const visibleIds = new Set(groupMembers.map(item => String(item.id)));
  const filter = groupFiltered && !visibleIds.has(localFilter.switch_id) ? { ...localFilter, switch_id: '' } : localFilter;
  const ackable = acknowledgeableEventIds(asArray(props.operations?.alerts));
  const queryKey = JSON.stringify([filter, filter.unackedOnly ? [...ackable].sort((a, b) => a - b) : []]);
  const result = usePolling(signal => {
    const { issuesOnly, unackedOnly, ...fields } = filter;
    const params = new URLSearchParams({
      ...fields,
      from: filter.from ? new Date(filter.from).toISOString() : '',
      to: filter.to ? new Date(filter.to).toISOString() : '',
      limit: String(SEARCH_EVENT_LIMIT),
    });
    if (issuesOnly) params.set('issues_only', '1');
    if (unackedOnly) params.set('unacked_only', '1');
    return api<Event[]>(`/api/events?${params}`, { signal });
  }, 0, [queryKey], true, 300);
  const rows = logRows(result.data || [], props.events, filter, ackable)
    .filter(event => !groupFiltered || visibleIds.has(String(event.switch_id)));
  const switches = new Map(groupMembers.map(item => [String(item.id), item.name]));
  for (const event of [...asArray(result.data), ...asArray(props.events)]) {
    const id = String(event.switch_id);
    if (!groupFiltered && event.switch_id > 0 && event.switch_name && !switches.has(id)) switches.set(id, event.switch_name);
  }
  const [selectedSwitchName, setSelectedSwitchName] = useState('');
  if (filter.switch_id && !switches.has(filter.switch_id)) switches.set(filter.switch_id, selectedSwitchName);
  const changeFilter = (key: keyof EventFilters, value: string | boolean) => setFilter(previous => ({ ...previous, [key]: value }));
  const ackIds = rows.filter(event => canAcknowledgeEvent(event, ackable)).map(event => event.id);
  const disabled = props.busy || props.locked || !!props.replay;
  const refresh = () => Promise.all([result.refresh(), props.operations?.refresh()]);
  const acknowledge = (ids: number[]) => acknowledgeEvents(ids, refresh, saved => {
    const confirmed = new Set(saved);
    props.setEvents(old => old.map(event => confirmed.has(event.id) ? { ...event, acked: true } : event));
  });
  return (
    <section className="panel space-y-2">
      <PollingStatus {...result} />
      <div className="flex flex-wrap gap-2">
        <label>{t('ui.m083')} <select value={filter.category} onChange={event => changeFilter('category', event.target.value)}>
          <option value="">{t('rack.all')}</option>
          {categories.map(category => <option key={category}>{category}</option>)}
        </select></label>
        <label>{t('ui.m026')} <select value={filter.severity} onChange={event => changeFilter('severity', event.target.value)}>
          <option value="">{t('rack.all')}</option>
          {Object.entries(severities).map(([severity, label]) => <option key={severity} value={severity}>{t(label)}</option>)}
        </select></label>
        <label>{t('ui.m018')} <select value={filter.switch_id} onChange={event => {
          setSelectedSwitchName(switches.get(event.target.value) || '');
          changeFilter('switch_id', event.target.value);
        }}>
          <option value="">{t('rack.all')}</option>
          {[...switches].sort((a, b) => a[1].localeCompare(b[1])).map(([id, name]) => <option key={id} value={id}>{name}</option>)}
        </select></label>
        {(['q', 'from', 'to'] as const).map(key => (
          <label key={key}>{t(key === 'q' ? 'ui.m264' : key === 'from' ? 'ui.m265' : 'ui.m266')}
            <input
              type={key === 'from' || key === 'to' ? 'datetime-local' : 'text'}
              value={filter[key]}
              onChange={event => changeFilter(key, event.target.value)}
            />
          </label>
        ))}
        <button aria-pressed={!!filter.issuesOnly} onClick={() => changeFilter('issuesOnly', !filter.issuesOnly)}>{t('logs.issuesOnly')}</button>
        <button aria-pressed={!!filter.unackedOnly} onClick={() => changeFilter('unackedOnly', !filter.unackedOnly)}>{t('logs.unackedOnly')}</button>
        <button aria-pressed={filter.category === 'MARKER'} onClick={() => changeFilter('category', filter.category === 'MARKER' ? '' : 'MARKER')}>{t('logs.marker')}</button>
        <a href="/api/export/csv?type=events">CSV</a>
        <a href="/api/export/json?type=events">JSON</a>
      </div>
      <div className="operation-area" role="group" aria-label={t('rack.actions')}>
        <button className="button-primary" data-action="ack-all" disabled={disabled || !ackIds.length}
          onClick={() => confirmAck(undefined, props.run, () => acknowledge(ackIds))}>{t('ui.m267')}</button>
      </div>
      <div className="overflow-auto">
        <table>
          <thead><tr><th className="whitespace-nowrap">{t('logs.receivedAt')}</th><th>{t('ui.m026')}</th><th>{t('ui.m083')}</th><th>{t('ui.m018')}</th><th>{t('ui.m028')}</th><th className="whitespace-nowrap">{t('ui.m268')}</th></tr></thead>
          <tbody>
            {rows.map(event => (
              <LogEventRow key={event.id} event={event} ackable={ackable} disabled={disabled}
                onAcknowledge={id => props.run(() => acknowledge([id]))} />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
