import { useState } from 'react';
import { api, json, message } from '../api';
import { pingLabel } from '../deviceCsv';
import { t } from '../i18n';
import { pcTimestamp } from '../pcTime';
import type { PingStatus } from '../types';

interface PingReply { status: PingStatus; rtt_ms: number; output: string; }
type PingState = { running: true } | { status: PingStatus; rtt: number; at: string; text: string };

// One device's Ping. The server records the result too, so the Ping column of Devices shows it afterwards.
// showResult: the answer next to the button (port detail); off where the row already shows the recorded result.
export function PingButton({ ip, disabled, showResult = true, onDone }: {
  ip: string; disabled?: boolean; showResult?: boolean; onDone?: () => void;
}) {
  const [state, setState] = useState<PingState>();
  const running = !!state && 'running' in state;
  const send = async () => {
    setState({ running: true });
    try {
      const reply = await api<PingReply>('/api/diagnostics/ping', json('POST', { ip }));
      setState({ status: reply.status, rtt: reply.rtt_ms, at: new Date().toISOString(), text: reply.output });
      // Only a sent Ping is recorded on the server; after an error there is nothing new to show.
      onDone?.();
    } catch (error) {
      setState({ status: 'error', rtt: 0, at: new Date().toISOString(), text: message(error) });
    }
  };
  const done = state && !('running' in state) ? state : undefined;
  return <span className="ping-button">
    <button type="button" data-action="ping-one" disabled={disabled || running}
      onClick={event => { event.stopPropagation(); void send(); }}>{running ? t('pingOutcome.pinging') : 'Ping'}</button>
    {/* An error is always shown here: it is not recorded anywhere else. */}
    {done && (showResult || done.status === 'error') && <span className={done.status === 'ok' ? 'text-green-700 dark:text-green-400' : 'text-amber-700 dark:text-amber-300'} title={done.text}>
      {pingLabel(done.status, done.rtt)} <small>{pcTimestamp(done.at)}</small>
    </span>}
  </span>;
}
