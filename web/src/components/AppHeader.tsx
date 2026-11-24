import { useState } from 'react';
import type { ReactNode, RefObject } from 'react';
import { api, json } from '../api';
import type { Phase, Status } from '../types';
import { t } from '../i18n';
import { PcClock, PhaseControls } from './PhaseControls';
import { jstTime } from '../pcTime';
import { Badge } from './Badge';
interface HeaderProps {
  online: boolean; phase?: Phase; onPhase: (phase: Phase) => void; onTrapHelp: () => void;
  wall: boolean; setWall: (value: boolean) => void; expert?: boolean; status?: Status; busy: boolean; locked: boolean;
  replay: unknown; markerInput: RefObject<HTMLInputElement>; summary?: ReactNode;
  updatedAt?: number | string;
  openChecklist?: (gate: boolean) => void;
  run: (action: () => Promise<void>) => void;
}
export function AppHeader({ online, phase, onPhase, onTrapHelp, wall, setWall, expert, status, busy, locked,
  replay, markerInput, run, summary, updatedAt, openChecklist }: HeaderProps) {
  const [note, setNote] = useState('');
  const phaseControl = <PhaseControls {...{ phase, locked, busy, wall, run, onPhase }}
    openChecklist={openChecklist && (() => openChecklist(true))} />;
  const expertBadge = expert && <Badge text={t('expert.badge')} />;
  const clock = <div className="header-time"><PcClock /><small>{t('rack.lastUpdated', { at: jstTime(updatedAt ?? NaN) })}</small></div>;
  if (wall) return <header className="wall-header">
    {summary}{phaseControl}{clock}{expertBadge}
    <button className="wall-exit" onClick={() => setWall(false)}>{t('wall.exit')}</button>
  </header>;
  return <header className="app-header">
    <div className="header-controls">
      <strong className="text-xl">AVLine-Sight</strong>
      <span>{replay ? t('rack.replay') : online ? '● LIVE' : t('rack.connecting')}</span>
      {status?.profile.name && <small className="header-project">{t('project.active', { name: status.profile.name })}</small>}
      {phaseControl}
      {!replay && (phase === 'setup' || phase === 'rehearsal') &&
        <button type="button" data-action="pre-show-checklist" disabled={busy} onClick={() => openChecklist?.(false)}>{t('checklist.title')}</button>}
      {status?.trap && <button onClick={onTrapHelp} title={[status.trap.addr, status.trap.error].filter(Boolean).join(' · ')}
        className={status.trap.ok ? 'trap-ready' : 'trap-failed'}>{t(status.trap.ok ? 'rack.trapOK' : 'rack.trapFail')} ⓘ</button>}
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
      {expertBadge}
      <button onClick={() => setWall(true)}>{t('rack.wall')}</button>
      {clock}
    </div>
    {summary}
  </header>;
}
