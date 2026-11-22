import { useSyncExternalStore } from 'react';
import { getLang } from './i18n';

let now = Date.now();
let timer: ReturnType<typeof setInterval> | undefined;
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  if (timer === undefined) {
    now = Date.now();
    timer = setInterval(() => {
      now = Date.now();
      for (const notify of listeners) notify();
    }, 1000);
  }
  return () => {
    listeners.delete(listener);
    if (!listeners.size) {
      clearInterval(timer);
      timer = undefined;
    }
  };
}

export const usePcTime = () => useSyncExternalStore(subscribe, () => now, () => now);

const clocks = {
  ja: new Intl.DateTimeFormat('ja-JP', { hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23', timeZoneName: 'short' }),
  en: new Intl.DateTimeFormat('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23', timeZoneName: 'short' }),
};
const two = (part: number) => String(part).padStart(2, '0');
const day = (date: Date) => `${date.getFullYear()}-${two(date.getMonth() + 1)}-${two(date.getDate())}`;

export function pcTimestamp(value: string | number, alwaysDate = false, today = Date.now()) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime()) || date.getFullYear() < 2) return '—';
  const parts = clocks[getLang()].formatToParts(date);
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find(item => item.type === type)?.value || '';
  const time = `${part('hour')}:${part('minute')}:${part('second')} ${part('timeZoneName')}`;
  return alwaysDate || day(date) !== day(new Date(today)) ? `${day(date)} ${time}` : time;
}
