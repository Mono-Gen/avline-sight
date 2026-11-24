import { eventHeadline, eventHint, eventTitle, isRecommendationEvent, t } from '../i18n';
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

// Key for "the same kind of syslog": message with port names and numbers removed.
function repeatKey(event: Event) {
  if (event.category !== 'SYSLOG' || event.severity !== 'INFO') return '';
  return (syslogMessage(event.title) ?? event.title).replace(/\d+\/\d+\/\d+/g, '#').replace(/\d+/g, '#');
}

// Collapse consecutive INFO syslog lines of the same kind (LLDP keepalive and the like) into one row with a count.
export function collapseRepeated(rows: readonly Event[]) {
  const out: { event: Event; count: number; switches: string[] }[] = [];
  for (const event of rows) {
    const key = repeatKey(event);
    const last = out[out.length - 1];
    if (key && last && repeatKey(last.event) === key) {
      last.count++;
      if (event.switch_name && !last.switches.includes(event.switch_name)) last.switches.push(event.switch_name);
      continue;
    }
    out.push({ event, count: 1, switches: event.switch_name ? [event.switch_name] : [] });
  }
  return out;
}

export function LogEventRow({ event, ackable, disabled, onAcknowledge, expert, count = 1, switches, showAck = true, onPort, ongoing = false }: {
  event: Event; ackable: ReadonlySet<number>; disabled: boolean; onAcknowledge: (id: number) => void; expert?: boolean;
  count?: number; switches?: string[]; showAck?: boolean; onPort?: (event: Event) => void;
  // True only while the alert behind this row is still active (not merely unacknowledged).
  ongoing?: boolean;
}) {
  const syslog = event.category === 'SYSLOG' ? syslogMessage(event.title) : undefined;
  const switchLabel = switches && switches.length > 1 ? t('logs.andMore', { name: switches[0], count: switches.length - 1 }) : event.switch_name;
  return <tr className={ongoing ? 'log-row-ongoing' : undefined}>
    <td className="whitespace-nowrap"><time dateTime={event.ts}>{pcTimestamp(event.ts, true)}</time></td>
    {/* Only warnings and critical events get a coloured badge; INFO stays quiet. */}
    <td className="whitespace-nowrap">{event.severity === 'INFO' ? <span className="log-info">{t('ui.m007')}</span> : <Badge text={event.severity} />}</td>
    <td className="whitespace-nowrap">{categoryLabel(event.category)}</td>
    <td className="whitespace-nowrap" title={switches && switches.length > 1 ? switches.join(', ') : undefined}>{switchLabel}{onPort && event.switch_id > 0 && event.if_index > 0 &&
      <> <button type="button" className="log-port-link" data-action="open-port" onClick={() => onPort(event)}>{t('logs.openPort')}</button></>}</td>
    <td className="log-content">{count > 1 && <span className="log-repeat" title={t('logs.repeatedTitle', { count })}>{t('logs.repeated', { count })} </span>}
      {syslog !== undefined ? (expert ? <details className="log-message">
      <summary>{syslog}</summary>
      <pre>{event.title}</pre>
    </details> : syslog) : <><strong className="log-title">{expert || event.severity === 'INFO' ? eventTitle(event)
      : [eventHeadline(event), typeof event.detail?.name === 'string' ? event.detail.name : ''].filter(Boolean).join(' · ')}</strong>
      {expert && eventHint(event) && <span className="log-hint">{t('alertState.whatToDo')}: {eventHint(event)}</span>}
      {expert && <AlertTechnicalDetails event={event} counters={event.detail?.raw_counters}
        observed={event.code === 'PRE_SHOW_CHECKLIST_REMAINDER' ? event.title : undefined} />}</>}</td>
    {showAck && <td className="whitespace-nowrap">{event.acked && event.severity !== 'INFO' ? t('ui.m269') : canAcknowledgeEvent(event, ackable) ? <button
      className="button-primary" data-action="ack" disabled={disabled} onClick={() => onAcknowledge(event.id)}
    >{t('ui.m268')}</button> : '—'}</td>}
  </tr>;
}

type Quick = 'all' | 'issues' | 'unacked' | 'marker';
type Period = 'all' | 'hour' | 'today' | 'show' | 'custom';

// datetime-local input value (YYYY-MM-DDTHH:MM:SS in JST, as shown everywhere else) for a Date.
// Seconds are kept so "during the show" does not pull in events from the minute before the show started.
function jstInput(date: Date) {
  return new Date(date.getTime() + 9 * 3600_000).toISOString().slice(0, 19);
}

export function Logs(props: ViewProps) {
  const [localFilter, setFilter] = useState<EventFilters>({
    category: '',
    severity: '',
    switch_id: '',
    q: '',
    from: '',
    to: '',
    // Default view: warnings and above.
    issuesOnly: true,
  });
  const [period, setPeriod] = useState<Period>('all');
  const groupFiltered = hasGroupFilter(props);
  const groupMembers = groupSwitches(props);
  const visibleIds = new Set(groupMembers.map(item => item.id));
  const filter = groupFiltered && !visibleIds.has(Number(localFilter.switch_id)) ? { ...localFilter, switch_id: '' } : localFilter;
  const ackable = acknowledgeableEventIds(asArray(props.operations?.alerts));
  const queryKey = JSON.stringify([filter, filter.unackedOnly ? [...ackable].sort((a, b) => a - b) : []]);
  // Show start is looked up first and on its own (see showStart below).
  const inShow = (props.operations?.phase || props.status?.phase) === 'show';
  const phaseEvents = usePolling(signal => api<Event[]>(`/api/events?category=SYSTEM&limit=${SEARCH_EVENT_LIMIT}`, { signal }),
    0, [inShow], inShow && !props.replay, 300);
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
  const quick: Quick = filter.category === 'MARKER' ? 'marker' : filter.unackedOnly ? 'unacked' : filter.issuesOnly ? 'issues' : 'all';
  // Quick filters replace the severity and category choices so the two levels never contradict each other.
  const setQuick = (value: Quick) => setFilter(previous => ({
    ...previous,
    issuesOnly: value === 'issues',
    unackedOnly: value === 'unacked',
    severity: value === 'all' ? previous.severity : '',
    category: value === 'marker' ? 'MARKER' : previous.category === 'MARKER' ? '' : previous.category,
  }));
  // Choosing INFO or the marker category in "more filters" leaves the issues-only and unacknowledged quick views.
  const changeDetail = (key: 'category' | 'severity', value: string) => setFilter(previous => ({
    ...previous,
    [key]: value,
    ...((key === 'severity' && value === 'INFO') || (key === 'category' && value === 'MARKER')
      ? { issuesOnly: false, unackedOnly: false } : {}),
  }));
  // "During the show": from the latest change to the show phase, while the show phase is current.
  // Looked up on its own so the quick filters and a page reload do not hide it.
  const showStart = inShow
    ? [...asArray(phaseEvents.data), ...asArray(props.events)]
      .filter(event => event.code === 'PHASE_CHANGED' && event.detail?.phase === 'show')
      .map(event => event.ts).sort().pop()
    : undefined;
  const choosePeriod = (value: Period) => {
    setPeriod(value);
    const now = new Date();
    const from = value === 'hour' ? jstInput(new Date(now.getTime() - 3600_000))
      : value === 'today' ? `${jstInput(now).slice(0, 10)}T00:00`
        : value === 'show' && showStart ? jstInput(new Date(showStart)) : '';
    if (value !== 'custom') setFilter(previous => ({ ...previous, from, to: '' }));
  };
  const grouped = collapseRepeated(rows);
  const activeIds = new Set(asArray(props.operations?.alerts).filter(alert => alert.active).map(alert => alert.event_id));
  const showAck = rows.some(event => canAcknowledgeEvent(event, ackable) || (event.acked && event.severity !== 'INFO'));
  const openPort = props.openPortMonitor
    ? (event: Event) => props.openPortMonitor?.({ switchId: event.switch_id, ifIndex: event.if_index })
    : undefined;
  return (
    <section className="panel space-y-2 log-screen">
      <PollingStatus {...result} />
      <div className="log-quick" role="group" aria-label={t('logs.quick')}>
        {(['all', 'issues', 'unacked', 'marker'] as const).map(value => <button key={value} data-quick={value}
          aria-pressed={quick === value} onClick={() => setQuick(value)}>
          {t(value === 'all' ? 'logs.quickAll' : value === 'issues' ? 'logs.quickIssues' : value === 'unacked' ? 'logs.quickUnacked' : 'logs.quickMarker')}
        </button>)}
        <span className="log-quick-spacer" />
        {ackIds.length > 0 && <button className="log-ack-all" data-action="ack-all" disabled={disabled}
          onClick={() => confirmAck(undefined, props.run, () => acknowledge(ackIds))}>{t('ui.m267')}</button>}
        <details className="log-export">
          <summary>{t('logs.export')} ▾</summary>
          <div className="log-export-items">
            <a href="/api/export/csv?type=events">CSV</a>
            <a href="/api/export/json?type=events">JSON</a>
            {/* A preset period has no end; the report then runs up to now. */}
            {!props.replay && <ShowReportButton run={props.run} busy={props.busy} from={filter.from}
              to={filter.to || (filter.from ? jstInput(new Date()) : '')} />}
          </div>
        </details>
      </div>
      <details className="log-more">
        <summary>{t('logs.more')}</summary>
        <div className="flex flex-wrap items-center gap-2">
          <label>{t('ui.m083')} <select value={filter.category} onChange={event => changeDetail('category', event.target.value)}>
            <option value="">{t('rack.all')}</option>
            {categories.map(category => <option key={category} value={category}>{categoryLabel(category)}</option>)}
          </select></label>
          <label>{t('ui.m026')} <select value={filter.severity} onChange={event => changeDetail('severity', event.target.value)}>
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
          <label>{t('ui.m264')} <input type="text" value={filter.q} onChange={event => changeFilter('q', event.target.value)} /></label>
          <label>{t('logs.period')} <select data-action="period" value={period} onChange={event => choosePeriod(event.target.value as Period)}>
            <option value="all">{t('logs.periodAll')}</option>
            <option value="hour">{t('logs.periodHour')}</option>
            <option value="today">{t('logs.periodToday')}</option>
            <option value="show" disabled={!showStart}>{t('logs.periodShow')}</option>
            <option value="custom">{t('logs.periodCustom')}</option>
          </select></label>
          {period === 'custom' && (['from', 'to'] as const).map(key => (
            <label key={key}>{t(key === 'from' ? 'ui.m265' : 'ui.m266')}
              <input type="datetime-local" value={filter[key]} onChange={event => changeFilter(key, event.target.value)} />
            </label>
          ))}
        </div>
      </details>
      <div className="overflow-auto">
        <table className="log-table">
          <thead><tr><th className="whitespace-nowrap">{t('logs.receivedAt')}</th><th className="whitespace-nowrap">{t('ui.m026')}</th><th className="whitespace-nowrap">{t('ui.m083')}</th><th className="whitespace-nowrap">{t('ui.m018')}</th><th className="log-content">{t('ui.m028')}</th>
            {showAck && <th className="whitespace-nowrap">{t('ui.m268')}</th>}</tr></thead>
          <tbody>
            {grouped.map(({ event, count, switches: names }) => (
              <LogEventRow key={event.id} event={event} ackable={ackable} disabled={disabled} expert={props.expert}
                count={count} switches={names} showAck={showAck} onPort={openPort} ongoing={activeIds.has(event.id)}
                onAcknowledge={id => props.run(() => acknowledge([id]))} />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
