import React from 'react';
import { api, json } from '../api';
import { alertCount, asArray } from '../format';
import type { Overview, Phase, Profile, ReplaySession, Settings, Status } from '../types';
import { t, useLang } from '../i18n';
import { ImportReplay } from './ImportReplay';
import { PcClock, PhaseControls } from './PhaseControls';
interface HeaderProps {
  online: boolean;
  phase?: Phase;
  onPhase: (phase: Phase) => void;
  onTrapHelp: () => void;
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
  replay: ReplaySession | null;
  overview: Overview;
  marker: () => void;
  run: (action: () => Promise<void>) => void;
  refresh: () => Promise<void>;
  setDialog: (value: string) => void;
  setPage: (value: 'overview') => void;
  setReplay: React.Dispatch<React.SetStateAction<ReplaySession | null>>;
}
export function AppHeader({
  online,
  phase,
  onPhase,
  onTrapHelp,
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
  const [lang, setLang] = useLang();
  const phaseControl = <PhaseControls {...{ phase, locked, busy, wall, run, onPhase }} />;
  if (wall) return <header className="wall-header">
    {phaseControl}<PcClock />
    <button className="wall-exit" aria-label={t('rack.exitWall')} onClick={() => setWall(false)}>×</button>
  </header>;
  return (<header className="flex flex-wrap items-center gap-2 bg-slate-900 p-3 text-white">
    <strong className="text-xl">AVLine-Sight
    </strong>
    <span>
      {replay ? t('rack.replay') : online ? '● LIVE' : t('rack.connecting')}
    </span>
      {phaseControl}
      {status?.trap && <button onClick={onTrapHelp} title={[status.trap.addr, status.trap.error].filter(Boolean).join(' · ')}
        className={status.trap.ok ? 'trap-ready' : 'trap-failed'}>{t(status.trap.ok ? 'rack.trapOK' : 'rack.trapFail')} ⓘ</button>}
      <select
        aria-label={t('rack.profile')}
        value={status?.profile.id || ''}
        disabled={busy || locked}
        onChange={e => {
          const id = e.target.value;
          run(async () => {
            await api(`/api/profiles/${id}/activate`, json('PUT'));
            await refresh();
          });
        }}
      >
        {asArray(profiles).map(p => <option key={p.id} value={p.id}>
          {p.name}
        </option>)}
      </select>
      <select aria-label={t('rack.part')} value={part} onChange={e => setPart(e.target.value)}>
        <option value="">{t('rack.all')}
        </option>
        {asArray(settings?.views)?.map(v => <option key={v.id} value={v.id}>
          {v.name}
        </option>)}
      </select>
      <button disabled={busy || locked} onClick={marker}>★ Marker
      </button>
      <span>🔴
        {replay ? asArray(replay.states).reduce((n, s) => n + (s.alert_counts?.CRITICAL || 0), 0) : alertCount(overview.alerts, 'critical')}
        ⚠
        {replay ? asArray(replay.states).reduce((n, s) => n + (s.alert_counts?.WARNING || 0), 0) : alertCount(overview.alerts, 'warning')}
      </span>
      <button onClick={() => setMuted(!muted)}>
        {t(muted ? 'rack.mute' : 'rack.sound')}
      </button>
      <button onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
        {t(theme === 'dark' ? 'rack.light' : 'rack.dark')}
      </button>
      <button
        disabled={busy || !!replay || !status}
        onClick={() => run(async () => {
          const pin = status?.locked && status.pin_set ? prompt(t('rack.pin')) : undefined;
          if (pin === null) return;
          await api('/api/mode/lock', json('POST', {
            locked: !status?.locked,
            pin
          }));
          await refresh();
        })}
      >
        {t(status?.locked ? 'rack.locked' : 'rack.lock')}
      </button>
      <button disabled={busy || locked} onClick={() => setDialog('export')}>{t('rack.export')}
      </button>
      <ImportReplay
        run={run}
        disabled={busy || locked}
        onImport={(id, t) => {
          setPage('overview');
          setReplay({
            id,
            timeline: t,
            states: []
          });
        }}
      />
      <button disabled={locked} onClick={() => setDialog('settings')}>{t('rack.settings')}
      </button>
    <select aria-label={t('rack.language')} value={lang} onChange={event => setLang(event.target.value === 'en' ? 'en' : 'ja')}>
      <option value="ja">JA</option><option value="en">EN</option>
    </select>
    <button onClick={() => setWall(true)}>{t('rack.wall')}</button>
    <PcClock />
  </header>);
}
