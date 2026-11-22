import { asArray } from './format';
import type { Alert, Event, Overview } from './types';
import { summarizeAlerts } from './wallAlerts';

export const LIVE_EVENT_LIMIT = 2000;
export const SEARCH_EVENT_LIMIT = 2000;
const RECENT_EVENT_LIMIT = 30;

export interface EventFilters {
  category: string;
  severity: string;
  switch_id: string;
  q: string;
  from: string;
  to: string;
  issuesOnly?: boolean;
  unackedOnly?: boolean;
}

export function mergeOverviewTick(previous: Overview, incoming: Overview): Overview {
  return {
    ...incoming,
    
    recent_events: (Array.isArray(incoming.recent_events)
      ? asArray(incoming.recent_events)
      : asArray(previous.recent_events)).slice(0, RECENT_EVENT_LIMIT),
  };
}

export function pushRecentEvent(previous: Overview, event: Event): Overview {
  return {
    ...previous,
    recent_events: [event, ...asArray(previous.recent_events).filter(item => item.id !== event.id)].slice(0, RECENT_EVENT_LIMIT),
  };
}

export function matchesEvent(event: Event, filter: EventFilters): boolean {
  return (!filter.category || event.category === filter.category) &&
    (!filter.severity || event.severity === filter.severity) &&
    (!filter.issuesOnly || isIssueEvent(event)) &&
    (!filter.switch_id || String(event.switch_id) === filter.switch_id) &&
    (!filter.q || JSON.stringify(event).toLowerCase().includes(filter.q.toLowerCase())) &&
    (!filter.from || Date.parse(event.ts) >= new Date(filter.from).getTime()) &&
    (!filter.to || Date.parse(event.ts) <= new Date(filter.to).getTime());
}

export function logRows(searchResults: Event[], liveEvents: Event[], filter: EventFilters, ackable = new Set<number>()): Event[] {
  const search = asArray(searchResults).slice(0, SEARCH_EVENT_LIMIT).filter(event => matchesEvent(event, filter));
  const live = asArray(liveEvents).slice(0, LIVE_EVENT_LIMIT).filter(event => matchesEvent(event, filter));
  const merged = new Map(live.map(event => [event.id, event]));
  for (const event of search) {
    merged.set(event.id, { ...event, acked: event.acked || merged.get(event.id)?.acked === true });
  }
  return Array.from(merged.values())
    .filter(event => !filter.unackedOnly || canAcknowledgeEvent(event, ackable))
    .sort((left, right) => Date.parse(right.ts) - Date.parse(left.ts));
}

function isIssueEvent(event: Event) {
  return event.severity === 'CRITICAL' || event.severity === 'WARNING';
}

export function acknowledgeableEventIds(alerts: Alert[]) {
  return new Set(summarizeAlerts(alerts).items.filter(alert => !alert.acked).map(alert => alert.event_id));
}

export function canAcknowledgeEvent(event: Event, ids: ReadonlySet<number>) {
  return !event.acked && isIssueEvent(event) && ids.has(event.id);
}

const syslogHeader = /^(?:<\d{1,3}>\s*)?(?:(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) +\d{1,2} +\d{2}:\d{2}:\d{2}|(?:\d+ )?\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})) +\S+\s+/;

export function syslogMessage(raw: string) {
  return raw.replace(syslogHeader, '').replace(/^<\d{1,3}>\s*/, '');
}
