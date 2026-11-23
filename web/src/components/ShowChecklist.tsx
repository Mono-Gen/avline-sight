import { api } from '../api';
import { t } from '../i18n';
import type { TranslationKey } from '../i18n';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import { Dialog } from './Dialog';
import { PollingStatus } from './PollingStatus';

export interface ChecklistTarget {
  switch_id?: number;
  if_index?: number;
  mac?: string;
  label?: string;
  destination?: 'logs' | 'settings' | 'trap';
}

type State = 'pass' | 'review' | 'pending';
interface Finding {
  state: State;
  reason: string;
  action?: string;
  target: ChecklistTarget;
}
interface Item {
  id: string;
  state: State;
  reason: string;
  action: string;
  optional: boolean;
  findings?: Finding[];
}

const stateSymbols: Record<State, string> = { pass: '✓', review: '!', pending: '…' };

function ChecklistRows({ items, onTarget }: { items: Item[]; onTarget: (target: ChecklistTarget) => void }) {
  return <ul className="divide-y divide-slate-300 dark:divide-slate-700">
    {items.map(item => <li key={item.id} className="space-y-1 py-2">
      <div className="flex flex-wrap items-baseline gap-x-2">
        <strong className={item.state === 'pass' ? 'text-green-700 dark:text-green-300' :
          item.state === 'review' ? 'text-amber-800 dark:text-amber-300' : 'text-slate-600 dark:text-slate-300'}>
          {stateSymbols[item.state]} {t(`checklist.${item.state}`)}
        </strong>
        <strong>{t(`checklist.${item.id}` as TranslationKey)}</strong>
      </div>
      {!item.findings?.length && <p>{item.reason}</p>}
      {item.findings?.map(({ state, target, reason, action }, index) => {
        const description = [target.label, reason].filter(Boolean).join(' · ');
        return <div key={index} className="flex flex-wrap items-baseline gap-x-2">
          {state === 'review' && (target.mac || target.switch_id || target.destination) ?
            <button type="button" onClick={() => onTarget(target)}>{description} ↗</button> :
            <span>{description}</span>}
          <span>{action ?? item.action}</span>
        </div>;
      })}
    </li>)}
  </ul>;
}

export function ShowChecklist({ gate, busy, locked, actionError, close, onTarget, onStart }: {
  gate: boolean;
  busy: boolean;
  locked: boolean;
  actionError?: string;
  close: () => void;
  onTarget: (target: ChecklistTarget) => void;
  onStart: (confirmChecklist: boolean) => void;
}) {
  const result = usePolling(signal => api<{ items: Item[] }>('/api/checklist/pre-show', { signal }), STATE_REFRESH_MS);
  const items = result.data?.items ?? [];
  const optional = items.filter(item => item.optional);
  const remaining = items.some(item => item.state !== 'pass');

  return <Dialog title={t('checklist.title')} close={() => { if (!busy) close(); }}>
    <PollingStatus {...result} />
    {gate && actionError && actionError !== result.error &&
      <p role="alert" className="text-red-700 dark:text-red-300">⚠ {actionError}</p>}
    <ChecklistRows items={items.filter(item => !item.optional)} onTarget={onTarget} />
    {optional.length > 0 && <details>
      <summary>{t('checklist.other')} ({optional.filter(item => item.state !== 'pass').length})</summary>
      <ChecklistRows items={optional} onTarget={onTarget} />
    </details>}
    {gate && <div className="flex flex-wrap gap-2">
      <button type="button" disabled={busy} onClick={close}>{t('checklist.cancel')}</button>
      <button type="button" className="button-primary" disabled={busy || locked || !items.length || !!result.error}
        onClick={() => onStart(remaining)}>
        {t(remaining ? 'checklist.startWithRemaining' : 'checklist.start')}
      </button>
    </div>}
  </Dialog>;
}

