import { eventHint, eventTitle, isRecommendationEvent, t } from '../i18n';
import { useEffect, useRef, useState } from 'react';
import { api } from '../api';
import { acknowledgeEvents } from '../acknowledge';
import { confirmAck } from '../useOperations';
import { Badge } from '../components/Badge';
import { AlertTechnicalDetails } from '../components/AlertTechnicalDetails';
import { PollingStatus } from '../components/PollingStatus';
import { ShowReportButton } from '../components/ShowReportButton';
import type { EventFilters } from '../events';
import { acknowledgeableEventIds, canAcknowledgeEvent, logRows, SEARCH_EVENT_LIMIT, syslogMessage } from '../events';
import { asArray } from '../format';
import { parseJstDateTime, pcTimestamp } from '../pcTime';
import type { Event } from '../types';
import { usePolling } from '../usePolling';
import type { LiveMessage } from '../ws';
import type { ViewProps } from './props';
import { groupSwitches, hasGroupFilter, inGroupScope } from './props';

const categories = ['ANOMALY', 'CONNECT', 'TRAP', 'SYSLOG', 'BASELINE', 'CONFIG', 'MARKER', 'SYSTEM'];
const categoryLabel = (category: string) => category === 'CONFIG' ? t('configChanges.title') : category;
const severities = { CRITICAL: 'ui.m005', WARNING: 'ui.m006', INFO: 'ui.m007' } as const;

export function LogEventRow({ event, ackable, disabled, onAcknowledge, expert }: {
  event: Event; ackable: ReadonlySet<number>; disabled: boolean; onAcknowledge: (id: number) => void; expert?: boolean;
}) {
  const syslog = event.category === 'SYSLOG' ? syslogMessage(event.title) : undefined;
  return <tr>
    <td className="whitespace-nowrap"><time dateTime={event.ts}>{pcTimestamp(event.ts, true)}</time></td>
    <td><Badge text={event.severity} /></td><td>{categoryLabel(event.category)}</td>
    <td>{event.switch_name}</td>
    <td>{syslog !== undefined ? (expert ? <details className="log-message">
      <summary>{syslog}</summary>
      <pre>{event.title}</pre>
    </details> : syslog) : <>{eventTitle(event)} {eventHint(event)}
      {expert && <AlertTechnicalDetails event={event} counters={event.detail?.raw_counters}
        observed={event.code === 'PRE_SHOW_CHECKLIST_REMAINDER' ? event.title : undefined} />}</>}</td>
    <td className="whitespace-nowrap">{event.acked && event.severity !== 'INFO' ? t('ui.m269') : canAcknowledgeEvent(event, ackable) ? <button
      className="button-primary" data-action="ack" disabled={disabled} onClick={() => onAcknowledge(event.id)}
    >{t('ui.m268')}</button> : '—'}</td>
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
  const visibleIds = new Set(groupMembers.map(item => item.id));
  const filter = groupFiltered && !visibleIds.has(Number(localFilter.switch_id)) ? { ...localFilter, switch_id: '' } : localFilter;
  const ackable = acknowledgeableEventIds(asArray(props.operations?.alerts));
  const queryKey = JSON.stringify([filter, filter.unackedOnly ? [...ackable].sort((a, b) => a - b) : []]);
  const alertRevision = useRef({ observed: 0, requested: 0 });
  const result = usePolling(signal => {
    alertRevision.current.requested = alertRevision.current.observed;
    const { issuesOnly, unackedOnly, ...fields } = filter;
    const params = new URLSearchParams({
      ...fields,
      from: filter.from ? parseJstDateTime(filter.from).toISOString() : '',
      to: filter.to ? parseJstDateTime(filter.to).toISOString() : '',
      limit: String(SEARCH_EVENT_LIMIT),
    });
    if (issuesOnly) params.set('issues_only', '1');
    if (unackedOnly) params.set('unacked_only', '1');
    return api<Event[]>(`/api/events?${params}`, { signal });
  }, 0, [queryKey], true, 300);
  const refreshSearch = result.refresh;
  useEffect(() => {
    let mounted = true;
    const onLiveControl = (event: globalThis.Event) => {
      if ((event as CustomEvent<LiveMessage>).detail.type !== 'alerts_changed') return;
      const version = ++alertRevision.current.observed;
      const refreshIfStale = () => {
        if (mounted && alertRevision.current.requested < version) {
          return refreshSearch();
        }
      };
      void refreshSearch().then(refreshIfStale, refreshIfStale).catch(() => undefined);
    };
    window.addEventListener('live-control', onLiveControl);
    return () => {
      mounted = false;
      window.removeEventListener('live-control', onLiveControl);
    };
  }, [refreshSearch]);
  const rows = logRows(result.data || [], props.events, filter, ackable)
    .filter(event => inGroupScope(event, visibleIds, groupFiltered) && (props.expert || !isRecommendationEvent(event.code)));
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
          {categories.map(category => <option key={category} value={category}>{categoryLabel(category)}</option>)}
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
        {!props.replay && <ShowReportButton run={props.run} busy={props.busy} from={filter.from} to={filter.to} />}
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
              <LogEventRow key={event.id} event={event} ackable={ackable} disabled={disabled} expert={props.expert}
                onAcknowledge={id => props.run(() => acknowledge([id]))} />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
