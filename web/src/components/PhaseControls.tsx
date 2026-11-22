import { api, json } from '../api';
import { t } from '../i18n';
import { pcTimestamp, usePcTime } from '../pcTime';
import type { Phase } from '../types';

const phases: Phase[] = ['setup', 'rehearsal', 'show', 'teardown'];

export function changePhase(phase: Phase, run: (action: () => Promise<void>) => void, onPhase: (phase: Phase) => void) {
  if (!window.confirm(t('rack.phaseConfirm', { phase: t(`rack.${phase}`), effect: t(`rack.${phase}Effect`) }))) return;
  run(async () => {
    const result = await api<{ phase: Phase }>('/api/phase', json('PUT', { phase }));
    onPhase(result.phase);
  });
}

export function PhaseControls({ phase, locked, busy, wall, run, onPhase }: {
  phase?: Phase; locked: boolean; busy: boolean; wall: boolean;
  run: (action: () => Promise<void>) => void; onPhase: (phase: Phase) => void;
}) {
  return <div className="phase-control">
    <span className={`phase-pill phase-${phase || 'unknown'}`}>{t('rack.phase')} · {phase ? t(`rack.${phase}`) : '—'}</span>
    {!wall && !locked && phase && <select data-action="phase" aria-label={t('rack.phase')} value={phase} disabled={busy}
      onChange={event => changePhase(event.target.value as Phase, run, onPhase)}>
      {phases.map(value => <option key={value} value={value}>{t(`rack.${value}`)}</option>)}
    </select>}
  </div>;
}

export function PcClock() {
  const now = usePcTime();
  return <div className="pc-clock"><time dateTime={new Date(now).toISOString()}>{pcTimestamp(now, true)}</time><small>{t('operations.pcTime')}</small></div>;
}
