import { useState } from 'react';
import { api, json } from '../api';
import { Badge } from '../components/Badge';
import { PollingStatus } from '../components/PollingStatus';
import type { EventFilters } from '../events';
import { logRows, SEARCH_EVENT_LIMIT } from '../events';
import { formatTime } from '../format';
import type { Event } from '../types';
import { usePolling } from '../usePolling';
import type { ViewProps } from './props';

const filterLabels = {
  category: '分類',
  severity: '重要度',
  switch_id: 'スイッチID',
  q: 'テキスト',
  from: '開始日時',
  to: '終了日時',
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
          disabled={props.busy || props.locked}
          onClick={() => props.run(async () => {
            await api('/api/events/ack-all', json('POST'));
            await refresh();
          })}
        >すべて確認</button>
        <a href="/api/export/csv?type=events">CSV</a>
        <a href="/api/export/json?type=events">JSON</a>
      </div>
      <div className="overflow-auto">
        <table>
          <thead><tr><th>時刻</th><th>重要度</th><th>分類</th><th>スイッチ</th><th>内容</th><th>確認</th></tr></thead>
          <tbody>
            {rows.map(event => (
              <tr key={event.id}>
                <td>{formatTime(event.ts)}</td><td><Badge text={event.severity} /></td><td>{event.category}</td>
                <td>{event.switch_name}</td><td>{event.title} {event.hint}</td>
                <td>{event.acked ? '確認済' : <button
                  disabled={props.busy || props.locked}
                  onClick={() => props.run(async () => {
                    await api(`/api/events/${event.id}/ack`, json('POST'));
                    await refresh();
                  })}
                >確認</button>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
