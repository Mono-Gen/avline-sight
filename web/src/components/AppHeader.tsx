import { useState } from 'react';
import type { ReactNode, RefObject } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import type { Phase, Profile, Settings, Status } from '../types';
import { t } from '../i18n';
import { PcClock, PhaseControls } from './PhaseControls';
import { jstTime } from '../pcTime';
interface HeaderProps {
  online: boolean; phase?: Phase; onPhase: (phase: Phase) => void; onTrapHelp: () => void;
  wall: boolean; setWall: (value: boolean) => void; status?: Status; busy: boolean; locked: boolean;
  profiles: Profile[]; settings?: Settings; groupSelection: string; setGroupSelection: (value: string) => void; groupNames?: string[];
  replay: unknown; markerInput: RefObject<HTMLInputElement>; summary?: ReactNode;
  updatedAt?: number | string;
  run: (action: () => Promise<void>) => void; refresh: () => Promise<void>; setDialog: (value: string) => void;
}
export function AppHeader({ online, phase, onPhase, onTrapHelp, wall, setWall, status, busy, locked,
  profiles, settings, groupSelection, setGroupSelection, groupNames = [], replay, markerInput, run, refresh, setDialog, summary, updatedAt }: HeaderProps) {
  const [note, setNote] = useState('');
  const phaseControl = <PhaseControls {...{ phase, locked, busy, wall, run, onPhase }} />;
  const clock = <div className="header-time"><PcClock /><small>{t('rack.lastUpdated', { at: jstTime(updatedAt ?? NaN) })}</small></div>;
  const groupFilter = <label>{t('pages.groupLabel')} <select aria-label={t('pages.group')} value={groupSelection}
    onChange={event => setGroupSelection(event.target.value)}>
    <option value="">{t('rack.all')}</option>
    {groupNames.map(name => <option key={name} value={`group:${name}`}>{name || t('rack.ungrouped')}</option>)}
    {asArray(settings?.views).length > 0 && <optgroup label={t('rack.part')}>
      {asArray(settings?.views).map(view => <option key={view.id} value={`view:${view.id}`}>{view.name}</option>)}
    </optgroup>}
  </select></label>;
  if (wall) return <header className="wall-header">
    {summary}{phaseControl}{groupFilter}{clock}
    <button className="wall-exit" onClick={() => setWall(false)}>{t('wall.exit')}</button>
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
      {groupFilter}
      <form className="header-marker" onSubmit={event => {
        event.preventDefault();
        if (busy || locked) return;
        run(async () => {
          await api('/api/markers', json('POST', { note }));
          setNote('');
        });
      }}>
        <input ref={markerInput} aria-label={t('pages.marker')} placeholder={t('pages.marker')}
          value={note} disabled={busy || locked} onChange={event => setNote(event.target.value)} />
        <button type="submit" disabled={busy || locked}>{t('pages.record')}</button>
      </form>
      <button disabled={busy || !!replay || !status} onClick={() => run(async () => {
        const pin = status?.locked && status.pin_set ? prompt(t('rack.pin')) : undefined;
        if (pin === null) return;
        await api('/api/mode/lock', json('POST', { locked: !status?.locked, pin })); await refresh();
      })}>{t(locked ? 'rack.locked' : 'rack.lock')}</button>
      <button disabled={busy || !status} onClick={() => setDialog('settings')}>{t('rack.settings')}</button>
      <button onClick={() => setWall(true)}>{t('rack.wall')}</button>
      {clock}
    </div>
    {summary}
  </header>;
}
