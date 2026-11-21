import { useEffect, useState } from 'react';
import { api, json } from '../api';
import { Badge } from '../components/Badge';
import { asArray, formatTime } from '../format';
import type { Event } from '../types';
import type { ViewProps } from './props';

const filterLabels = {
  category: '分類',
  severity: '重要度',
  switch_id: 'スイッチID',
  q: 'テキスト',
  from: '開始日時',
  to: '終了日時',
};
type Filters = Record<keyof typeof filterLabels, string>;

function matches(event: Event, filter: Filters): boolean {
  return (!filter.category || event.category.toLowerCase().includes(filter.category.toLowerCase())) &&
    (!filter.severity || event.severity.toLowerCase().includes(filter.severity.toLowerCase())) &&
    (!filter.switch_id || String(event.switch_id) === filter.switch_id) &&
    (!filter.q || JSON.stringify(event).toLowerCase().includes(filter.q.toLowerCase())) &&
    (!filter.from || Date.parse(event.ts) >= new Date(filter.from).getTime()) &&
    (!filter.to || Date.parse(event.ts) <= new Date(filter.to).getTime());
}

export function Logs(props: ViewProps) {
  const [filter, setFilter] = useState<Filters>({
    category: '',
    severity: '',
    switch_id: '',
    q: '',
    from: '',
    to: ''
  });
  const refresh = async () => {
    const params = new URLSearchParams({
      ...filter,
      from: filter.from ? new Date(filter.from).toISOString() : '',
      to: filter.to ? new Date(filter.to).toISOString() : '',
      limit: '500',
    });
    const fetched = await api<Event[]>(`/api/events?${params}`);
    props.setEvents(old => {
      // GET待機中に届いたイベントを、後着の履歴応答で失わないよう統合する。
      const merged = [...asArray(old), ...asArray(fetched)];
      return Array.from(new Map(merged.map(event => [event.id, event])).values())
        .sort((left, right) => Date.parse(right.ts) - Date.parse(left.ts)).slice(0, 2000);
    });
  };
  useEffect(() => {
    const timer = setTimeout(() => {
      void refresh().catch(() => undefined);
    }, 300);
    return () => clearTimeout(timer);
  }, [JSON.stringify(filter)]);
  const rows = asArray(props.events).filter(event => matches(event, filter));
  return (
    <section className="panel space-y-2">
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
