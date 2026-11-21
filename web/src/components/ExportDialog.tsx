import { useEffect, useState } from 'react';
import { api, download, json, response } from '../api';
import { asArray, formatTime } from '../format';
import type { Event } from '../types';
import { Dialog } from './Common';
type Run = (f: () => Promise<void>) => void;
export function ExportDialog({ close, run, busy }: {
  close: () => void;
  run: Run;
  busy: boolean;
}) {
  const [markers, setMarkers] = useState<Event[]>([]);
  const [marker, setMarker] = useState('');
  const [minutes, setMinutes] = useState(5);
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  useEffect(() => {
    api<Event[]>('/api/events?category=MARKER').then(setMarkers).catch(() => setMarkers([]));
  }, []);
  return <Dialog title="ログパックを書き出す" close={close}>
    <select value={marker} onChange={e => setMarker(e.target.value)}>
      <option value="">期間指定（空欄は直近1時間）
      </option>
      {asArray(markers).map(m => <option value={m.id} key={m.id}>
        {formatTime(m.ts)}

        {m.title}
      </option>)}
    </select>
    {marker ? <select value={minutes} onChange={e => setMinutes(Number(e.target.value))}>
      {[3, 5, 10].map(n => <option key={n} value={n}>前後
        {n}
        分
      </option>)}
    </select> : <div>
      <input type="datetime-local" value={from} onChange={e => setFrom(e.target.value)} />
      ～
      <input type="datetime-local" value={to} onChange={e => setTo(e.target.value)} />
    </div>}
    <button
      disabled={busy}
      onClick={() => run(async () => {
        const end = to ? new Date(to) : new Date();
        const start = from ? new Date(from) : new Date(end.getTime() - 3600000);
        const body = marker ? {
          marker_id: Number(marker),
          minutes
        } : {
          from: start.toISOString(),
          to: end.toISOString()
        };
        const r = await response('/api/export/pack', json('POST', body));
        download(await r.blob(), 'export.avsight');
        close();
      })}
    >書き出す
    </button>
  </Dialog>;
}
