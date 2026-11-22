import { eventHint, eventTitle, t } from '../i18n';
import { useState } from 'react';
import { api } from '../api';
import { acknowledgeEvents } from '../acknowledge';
import { confirmAck } from '../useOperations';
import { Badge } from '../components/Badge';
import { PollingStatus } from '../components/PollingStatus';
import type { EventFilters } from '../events';
import { logRows, SEARCH_EVENT_LIMIT } from '../events';
import { formatTime } from '../format';
import type { Event } from '../types';
import { usePolling } from '../usePolling';
import type { ViewProps } from './props';

const filterLabels = {
  get category() { return t('ui.m083'); },
  get severity() { return t('ui.m026'); },
  get switch_id() { return t('ui.m019'); },
  get q() { return t('ui.m264'); },
  get from() { return t('ui.m265'); },
  get to() { return t('ui.m266'); },
};
type Filters = EventFilters;

export function Logs(props: ViewProps) {
  const [filter, setFilter] = useState<Filters>({
    category: '',
    severity: '',
    switch_id: '',
    q: '',
    from: '',
    to: ''
  });
  const queryKey = JSON.stringify(filter);
  const result = usePolling(signal => {
    const params = new URLSearchParams({
      ...filter,
      from: filter.from ? new Date(filter.from).toISOString() : '',
      to: filter.to ? new Date(filter.to).toISOString() : '',
      limit: String(SEARCH_EVENT_LIMIT),
    });
    return api<Event[]>(`/api/events?${params}`, { signal });
  }, 0, [queryKey], true, 300);
  const refresh = result.refresh;
  const rows = logRows(result.data || [], props.events, filter);
  const acknowledge = (ids: number[]) => acknowledgeEvents(ids, refresh, saved => {
    const confirmed = new Set(saved);
    props.setEvents(old => old.map(event => confirmed.has(event.id) ? { ...event, acked: true } : event));
  });
  return (
    <section className="panel space-y-2">
      <PollingStatus {...result} />
      <div className="flex flex-wrap gap-2">
        {(Object.entries(filter) as [keyof Filters, string][]).map(([key, value]) => (
          <label key={key}>{filterLabels[key]}
            <input
              type={key === 'from' || key === 'to' ? 'datetime-local' : 'text'}
              value={value}
              onChange={event => setFilter({
                ...filter,
                [key]: event.target.value
              })}
            />
          </label>
        ))}
        <button
          disabled={props.busy || !!props.replay || !rows.some(event => !event.acked)}
          onClick={() => confirmAck(undefined, props.run, () => acknowledge(rows.filter(event => !event.acked).map(event => event.id)))}
        >{t('ui.m267')}</button>
        <a href="/api/export/csv?type=events">CSV</a>
        <a href="/api/export/json?type=events">JSON</a>
      </div>
      <div className="overflow-auto">
        <table>
          <thead><tr><th>{t('ui.m082')}</th><th>{t('ui.m026')}</th><th>{t('ui.m083')}</th><th>{t('ui.m018')}</th><th>{t('ui.m028')}</th><th>{t('ui.m268')}</th></tr></thead>
          <tbody>
            {rows.map(event => (
              <tr key={event.id}>
                <td>{formatTime(event.ts)}</td><td><Badge text={event.severity} /></td><td>{event.category}</td>
                <td>{event.switch_name}</td><td>{eventTitle(event)} {eventHint(event)}</td>
                <td>{event.acked ? t('ui.m269') : <button
                  disabled={props.busy || !!props.replay}
                  onClick={() => props.run(() => acknowledge([event.id]))}
                >{t('ui.m268')}</button>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
