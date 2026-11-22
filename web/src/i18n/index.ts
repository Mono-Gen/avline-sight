import { useSyncExternalStore } from 'react';
import { ja } from './ja';
import { en } from './en';

export type Language = 'ja' | 'en';
export type Widen<T> = { [K in keyof T]: T[K] extends string ? string : Widen<T[K]> };
export type Dictionary = Widen<typeof ja>;
type Paths<T> = { [K in keyof T & string]: T[K] extends string ? K : `${K}.${Paths<T[K]>}` }[keyof T & string];
export type TranslationKey = Paths<typeof ja>;

function storedLanguage(): Language {
  try {
    return typeof localStorage !== 'undefined' && localStorage.getItem('language') === 'en' ? 'en' : 'ja';
  } catch {
    return 'ja';
  }
}
let language = storedLanguage();
const listeners = new Set<() => void>();
export const getLang = () => language;
export function setLang(value: Language) {
  if (value !== 'ja' && value !== 'en') return;
  language = value;
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem('language', value);
  } catch {
    language = value;
  }
  if (typeof document !== 'undefined') document.documentElement.lang = value;
  listeners.forEach(listener => listener());
}
function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}
export function useLang(): [Language, typeof setLang] {
  return [useSyncExternalStore(subscribe, getLang, () => 'ja'), setLang];
}
export function t(key: TranslationKey, params: Record<string, unknown> = {}): string {
  let value: unknown = language === 'en' ? en : ja;
  for (const part of key.split('.')) {
    value = value && typeof value === 'object' ? (value as Record<string, unknown>)[part] : undefined;
  }
  return typeof value === 'string'
    ? value.replace(/\{(\w+)\}/g, (_, name: string) => params[name] == null ? '' : String(params[name]))
    : key;
}
interface LocalizedEvent {
  code: string;
  title?: string;
  hint?: string;
  detail?: Record<string, unknown>;
}
export function eventTitle(event: LocalizedEvent): string {
  if (!(event.code in ja.events)) return event.title || event.code;
  return t(`events.${event.code}.title` as TranslationKey, event.detail);
}
export function eventHint(event: LocalizedEvent): string {
  if (!(event.code in ja.events)) return event.hint || '';
  return t(`events.${event.code}.hint` as TranslationKey, event.detail);
}
