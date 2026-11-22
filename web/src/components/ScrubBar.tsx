import { useState } from 'react';
import { api, json } from '../api';
import { asArray, formatTime } from '../format';
import type { Timeline } from '../types';
type Run = (f: () => Promise<void>) => void;
const KEYBOARD_STEP_MS = 1000;

export function ScrubBar({ sid, timeline, ts, requestedTs, busy, onSnapshot, onClose, run }: {
  sid: string;
  timeline: Timeline;
  ts?: string;
  requestedTs?: string;
  busy: boolean;
  onSnapshot: (ts: string) => void;
  onClose: () => void;
  run: Run;
}) {
  const [draft, setDraft] = useState<number>();
  const min = Date.parse(timeline.from);
  const max = Date.parse(timeline.to);
  // 取得時刻と移動先の間に記録がなくても、次のキー操作は移動先から続ける。
  const value = draft ?? (requestedTs ? Date.parse(requestedTs) : min);
  const seek = (requested: number) => {
    // 取得できた時刻を一覧や書出と共有し、失敗時も表示だけが先に進まないようにする。
    setDraft(undefined);
    if (Number.isFinite(requested)) onSnapshot(new Date(requested).toISOString());
  };
  return <div className="bg-amber-100 p-3 text-black">
    <strong>⏪ オフライン再生
    </strong>

    <button
      disabled={busy}
      onClick={() => run(async () => {
        await api(`/api/replay/${encodeURIComponent(sid)}`, json('DELETE'));
        onClose();
      })}
    >LIVEへ戻る
    </button>
    <div>
      <input
        aria-label="再生の移動先"
        type="range"
        min={min}
        max={max}
        value={value}
        disabled={busy}
        onChange={e => setDraft(e.currentTarget.valueAsNumber)}
        onKeyDown={e => {
          const forward = e.key === 'ArrowRight' || e.key === 'ArrowUp';
          const backward = e.key === 'ArrowLeft' || e.key === 'ArrowDown';
          if (!forward && !backward) return;
          e.preventDefault();
          const increment = forward ? KEYBOARD_STEP_MS : -KEYBOARD_STEP_MS;
          setDraft(previous => Math.min(max, Math.max(min, (previous ?? value) + increment)));
        }}
        onPointerUp={e => seek(e.currentTarget.valueAsNumber)}
        onPointerCancel={() => setDraft(undefined)}
        onKeyUp={() => { if (draft !== undefined) seek(draft); }}
        onBlur={() => setDraft(undefined)}
      />

      <span>表示中: {ts ? formatTime(ts) : '取得待ち'}</span>
    </div>
    <div>
      {asArray((timeline.markers || [])).map((m, i) => <button
        key={i}
        title={m.title}
        disabled={busy}
        onClick={() => {
          setDraft(undefined);
          onSnapshot(m.ts);
        }}
      >★
        {m.title}
      </button>)}
    </div>
  </div>;
}
