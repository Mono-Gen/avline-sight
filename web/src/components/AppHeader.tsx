import type { ReactNode } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import type { Phase, Profile, Settings, Status } from '../types';
import { t } from '../i18n';
import { PcClock, PhaseControls } from './PhaseControls';
interface HeaderProps {
  online: boolean; phase?: Phase; onPhase: (phase: Phase) => void; onTrapHelp: () => void;
  wall: boolean; setWall: (value: boolean) => void; status?: Status; busy: boolean; locked: boolean;
  profiles: Profile[]; settings?: Settings; part: string; setPart: (value: string) => void;
  replay: unknown; marker: () => void; summary?: ReactNode;
  run: (action: () => Promise<void>) => void; refresh: () => Promise<void>; setDialog: (value: string) => void;
}
export function AppHeader({ online, phase, onPhase, onTrapHelp, wall, setWall, status, busy, locked,
  profiles, settings, part, setPart, replay, marker, run, refresh, setDialog, summary }: HeaderProps) {
  const phaseControl = <PhaseControls {...{ phase, locked, busy, wall, run, onPhase }} />;
  if (wall) return <header className="wall-header">
    {summary}{phaseControl}<PcClock />
    <button className="wall-exit" aria-label={t('rack.exitWall')} onClick={() => setWall(false)}>×</button>
  </header>;
  return <header className="app-header">
    <div className="header-controls">
      <strong className="text-xl">AVLine-Sight</strong>
      <span>{replay ? t('rack.replay') : online ? '● LIVE' : t('rack.connecting')}</span>
      {phaseControl}
      {status?.trap && <button onClick={onTrapHelp} title={[status.trap.addr, status.trap.error].filter(Boolean).join(' · ')}
        className={status.trap.ok ? 'trap-ready' : 'trap-failed'}>{t(status.trap.ok ? 'rack.trapOK' : 'rack.trapFail')} ⓘ</button>}
      <select aria-label={t('rack.profile')} value={status?.profile.id || ''} disabled={busy || locked} onChange={event => {
        const id = event.target.value;
        run(async () => { await api(`/api/profiles/${id}/activate`, json('PUT')); await refresh(); });
      }}>{asArray(profiles).map(profile => <option key={profile.id} value={profile.id}>{profile.name}</option>)}</select>
      <select aria-label={t('rack.part')} value={part} onChange={event => setPart(event.target.value)}>
        <option value="">{t('rack.all')}</option>
        {asArray(settings?.views).map(view => <option key={view.id} value={view.id}>{view.name}</option>)}
      </select>
      <button disabled={busy || locked} onClick={marker}>★ {t('rack.marker')}</button>
      <button disabled={busy || !!replay || !status} onClick={() => run(async () => {
        const pin = status?.locked && status.pin_set ? prompt(t('rack.pin')) : undefined;
        if (pin === null) return;
        await api('/api/mode/lock', json('POST', { locked: !status?.locked, pin })); await refresh();
      })}>{t(status?.locked ? 'rack.locked' : 'rack.lock')}</button>
      <button disabled={busy || !status} onClick={() => setDialog('settings')}>{t('rack.settings')}</button>
      <button onClick={() => setWall(true)}>{t('rack.wall')}</button>
      <PcClock />
    </div>
    {summary}
  </header>;
}
