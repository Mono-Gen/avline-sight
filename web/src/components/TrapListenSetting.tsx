import { useState } from 'react';
import { api, json } from '../api';
import { t } from '../i18n';
import type { Status } from '../types';

// "host:port" with a port 1-65535; the host may be an IPv4 address or 0.0.0.0. The server reports any bind failure itself.
export function validTrapAddress(value: string) {
  const match = /^(\d{1,3}(?:\.\d{1,3}){3}):(\d{1,5})$/.exec(value.trim());
  if (!match) return false;
  const port = Number(match[2]);
  return match[1].split('.').every(part => Number(part) <= 255) && port >= 1 && port <= 65535;
}

// A loopback address only takes traps sent from this PC, so switches can never reach it.
export function loopbackOnly(value: string) {
  return /^127\./.test(value.trim());
}

// Where this PC listens for SNMP traps. One setting for the whole app, not per switch:
// each switch sends to it, and the destination on the switch is set on the switch itself.
export function TrapListenSetting({ trap, busy, locked, run, onChanged, onHelp }: {
  trap?: Status['trap']; busy: boolean; locked: boolean;
  run: (action: () => Promise<void>) => void; onChanged: () => Promise<unknown>; onHelp: () => void;
}) {
  const [address, setAddress] = useState(trap?.addr || '0.0.0.0:162');
  const [result, setResult] = useState<Status['trap']>();
  const current = result ?? trap;
  const valid = validTrapAddress(address);
  return <section className="trap-setting space-y-2" aria-label={t('trapSetting.title')}>
    <h3>{t('trapSetting.title')}</h3>
    <p>{current?.ok ? t('trapSetting.listening', { address: current.addr }) : t('trapSetting.stopped', { address: current?.addr || '—' })}</p>
    {current?.error && <p role="alert">⚠ {current.error}</p>}
    {current?.ok && loopbackOnly(current.addr) && <p role="alert">⚠ {t('trapSetting.loopback')}</p>}
    <label className="flex flex-wrap items-center gap-2">{t('trapSetting.address')}
      <input value={address} disabled={busy || locked} onChange={event => setAddress(event.target.value)} placeholder="0.0.0.0:162" />
    </label>
    {!valid && <p role="alert">⚠ {t('trapSetting.invalid')}</p>}
    {valid && loopbackOnly(address) && address.trim() !== current?.addr && <p role="alert">⚠ {t('trapSetting.loopback')}</p>}
    <p className="muted">{t('trapSetting.hint')}</p>
    <div className="flex flex-wrap gap-2">
      <button className="button-primary" disabled={busy || locked || !valid || (!!current?.ok && address.trim() === current.addr)}
        onClick={() => run(async () => {
          const status = await api<Status['trap']>('/api/system/network-bind', json('PUT', { trap_addr: address.trim() }));
          setResult(status);
          await onChanged();
        })}>{t('trapSetting.apply')}</button>
      <button type="button" onClick={onHelp}>{t('trapSetting.help')}</button>
    </div>
  </section>;
}
