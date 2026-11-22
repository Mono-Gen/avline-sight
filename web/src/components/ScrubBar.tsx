import { t } from '../i18n';
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
  
  const value = draft ?? (requestedTs ? Date.parse(requestedTs) : min);
  const seek = (requested: number) => {
    
    setDraft(undefined);
    if (Number.isFinite(requested)) onSnapshot(new Date(requested).toISOString());
  };
  return <div className="bg-amber-100 p-3 text-black">
    <strong>{t('ui.m169')}</strong>

    <button
      disabled={busy}
      onClick={() => run(async () => {
        await api(`/api/replay/${encodeURIComponent(sid)}`, json('DELETE'));
        onClose();
      })}
    >{t('ui.m170')}</button>
    <div>
      <input
        aria-label={t('ui.m171')}
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

      <span>{t('ui.m172')}{ts ? formatTime(ts) : t('ui.m147')}</span>
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
