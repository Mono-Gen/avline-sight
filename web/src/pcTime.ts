import { useSyncExternalStore } from 'react';

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

export function parseJstDateTime(value: string) {
  return new Date(`${value}+09:00`);
}

const displayTimeZone = 'JST';
const clock = new Intl.DateTimeFormat('en-GB', {
  timeZone: 'Asia/Tokyo', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
});
const calendar = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Asia/Tokyo', year: 'numeric', month: '2-digit', day: '2-digit',
});

export function jstTime(value: string | number | Date, withZone = true) {
  const date = value instanceof Date ? value : new Date(value);
  if (!Number.isFinite(date.getTime()) || date.getUTCFullYear() < 2) return '—';
  return withZone ? `${clock.format(date)} ${displayTimeZone}` : clock.format(date);
}

function jstDay(value: string | number) {
  const parts = calendar.formatToParts(new Date(value));
  return ['year', 'month', 'day'].map(type => parts.find(part => part.type === type)?.value).join('-');
}

export function pcTimestamp(value: string | number, alwaysDate = false, today = Date.now()) {
  const time = jstTime(value);
  if (time === '—') return time;
  const day = jstDay(value);
  return alwaysDate || day !== jstDay(today) ? `${day} ${time}` : time;
}
