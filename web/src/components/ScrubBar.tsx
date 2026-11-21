import { useState } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import type { Timeline } from '../types';
type Run = (f: () => Promise<void>) => void;
export function ScrubBar({ sid, timeline, onSnapshot, onClose, run }: {
  sid: string;
  timeline: Timeline;
  onSnapshot: (ts: string) => void;
  onClose: () => void;
  run: Run;
}) {
  const [value, setValue] = useState(new Date(timeline.from).getTime());
  const min = new Date(timeline.from).getTime(), max = new Date(timeline.to).getTime();
  return <div className="bg-amber-100 p-3 text-black">
    <strong>⏪ オフライン再生
    </strong>

    <button
      onClick={() => run(async () => {
        await api(`/api/replay/${encodeURIComponent(sid)}`, json('DELETE'));
        onClose();
      })}
    >LIVEへ戻る
    </button>
    <div>
      <input
        aria-label="再生時刻"
        type="range"
        min={min}
        max={max}
        value={value}
        onChange={e => setValue(Number(e.target.value))}
        onMouseUp={() => onSnapshot(new Date(value).toISOString())}
        onKeyUp={() => onSnapshot(new Date(value).toISOString())}
      />

      {new Date(value).toLocaleString()}
    </div>
    <div>
      {asArray((timeline.markers || [])).map((m, i) => <button
        key={i}
        title={m.title}
        onClick={() => {
          const v = new Date(m.ts).getTime();
          setValue(v);
          onSnapshot(m.ts);
        }}
      >★
        {m.title}
      </button>)}
    </div>
  </div>;
}
