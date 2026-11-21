import React from 'react';
import { api, json } from '../api';
import { alertCount, asArray } from '../format';
import type { Overview, Profile, Settings, Status, SwitchState, Timeline } from '../types';
import { ImportReplay } from './ImportReplay';
interface HeaderProps {
  online: boolean;
  wall: boolean;
  setWall: (value: boolean) => void;
  theme: string;
  setTheme: (value: string) => void;
  muted: boolean;
  setMuted: (value: boolean) => void;
  status?: Status;
  busy: boolean;
  locked: boolean;
  profiles: Profile[];
  settings?: Settings;
  part: string;
  setPart: (value: string) => void;
  replay: {
    id: string;
    timeline: Timeline;
    states: SwitchState[];
  } | null;
  overview: Overview;
  marker: () => void;
  run: (action: () => Promise<void>) => void;
  refresh: () => Promise<void>;
  setDialog: (value: string) => void;
  setPage: (value: string) => void;
  setReplay: React.Dispatch<React.SetStateAction<{
    id: string;
    timeline: Timeline;
    states: SwitchState[];
  } | null>>;
}
export function AppHeader({
  online,
  wall,
  setWall,
  theme,
  setTheme,
  muted,
  setMuted,
  status,
  busy,
  locked,
  profiles,
  settings,
  part,
  setPart,
  replay,
  overview,
  marker,
  run,
  refresh,
  setDialog,
  setPage,
  setReplay
}: HeaderProps) {
  return (<header className="flex flex-wrap items-center gap-2 bg-slate-900 p-3 text-white">
    <strong className="text-xl">AVLine-Sight
    </strong>
    <span>
      {replay ? '⏪ オフライン再生' : online ? '🟢 LIVE' : '⚠ 接続待機'}
    </span>
    {!wall && <>
      <select
        aria-label="プロファイル"
        value={status?.profile.id || ''}
        disabled={busy || locked}
        onChange={e => run(async () => {
          await api(`/api/profiles/${e.target.value}/activate`, json('PUT'));
          await refresh();
        })}
      >
        {asArray(profiles).map(p => <option key={p.id} value={p.id}>
          {p.name}
        </option>)}
      </select>
      <select aria-label="担当パート" value={part} onChange={e => setPart(e.target.value)}>
        <option value="">全体
        </option>
        {asArray(settings?.views)?.map(v => <option key={v.id} value={v.id}>
          {v.name}
        </option>)}
      </select>
      <button disabled={busy || !!replay} onClick={marker}>★ Marker
      </button>
      <span>🔴
        {replay ? asArray(replay.states).reduce((n, s) => n + (s.alert_counts?.CRITICAL || 0), 0) : alertCount(overview.alerts, 'critical')}
        ⚠
        {replay ? asArray(replay.states).reduce((n, s) => n + (s.alert_counts?.WARNING || 0), 0) : alertCount(overview.alerts, 'warning')}
      </span>
      <button onClick={() => setMuted(!muted)}>
        {muted ? '🔇 消音' : '🔊 音あり'}
      </button>
      <button onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
        {theme === 'dark' ? '☀ ライト' : '☾ ダーク'}
      </button>
      <button
        disabled={busy || !!replay}
        onClick={() => run(async () => {
          const pin = status?.locked && status.pin_set ? prompt('PINを入力してください') : undefined;
          if (pin === null) return;
          await api('/api/mode/lock', json('POST', {
            locked: !status?.locked,
            pin
          }));
          await refresh();
        })}
      >
        {status?.locked ? '🔒 ロック中' : '🔓 ロック'}
      </button>
      <button disabled={busy || locked} onClick={() => setDialog('export')}>書出
      </button>
      <ImportReplay
        run={run}
        disabled={busy || locked}
        onImport={(id, t) => {
          setPage('俯瞰');
          setReplay({
            id,
            timeline: t,
            states: []
          });
        }}
      />
      <button disabled={locked} onClick={() => setDialog('settings')}>設定
      </button>
    </>}
    <button onClick={() => setWall(!wall)}>
      {wall ? '壁表示を終了' : '壁表示'}
    </button>
  </header>);
}
