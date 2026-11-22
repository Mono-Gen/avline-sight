import { asArray } from './format';
import type { Event, Overview } from './types';

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
  return (!filter.category || event.category.toLowerCase().includes(filter.category.toLowerCase())) &&
    (!filter.severity || event.severity.toLowerCase().includes(filter.severity.toLowerCase())) &&
    (!filter.switch_id || String(event.switch_id) === filter.switch_id) &&
    (!filter.q || JSON.stringify(event).toLowerCase().includes(filter.q.toLowerCase())) &&
    (!filter.from || Date.parse(event.ts) >= new Date(filter.from).getTime()) &&
    (!filter.to || Date.parse(event.ts) <= new Date(filter.to).getTime());
}

export function logRows(searchResults: Event[], liveEvents: Event[], filter: EventFilters): Event[] {
  const search = asArray(searchResults).slice(0, SEARCH_EVENT_LIMIT).filter(event => matchesEvent(event, filter));
  const live = asArray(liveEvents).slice(0, LIVE_EVENT_LIMIT).filter(event => matchesEvent(event, filter));
  
  return Array.from(new Map([...live, ...search].map(event => [event.id, event])).values())
    .sort((left, right) => Date.parse(right.ts) - Date.parse(left.ts));
}
