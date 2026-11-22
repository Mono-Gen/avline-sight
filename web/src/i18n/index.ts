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
function translation(key: TranslationKey): string {
  let value: unknown = language === 'en' ? en : ja;
  for (const part of key.split('.')) {
    value = value && typeof value === 'object' ? (value as Record<string, unknown>)[part] : undefined;
  }
  return typeof value === 'string' ? value : key;
}
export function t(key: TranslationKey, params: Record<string, unknown> = {}): string {
  return translation(key).replace(/\{(\w+)\}/g, (_, name: string) => params[name] == null ? '' : String(params[name]));
}
interface LocalizedEvent {
  code: string;
  title?: string;
  hint?: string;
  detail?: Record<string, unknown>;
}
export function eventTitle(event: LocalizedEvent): string {
  if (!Object.prototype.hasOwnProperty.call(ja.events, event.code)) return event.title || event.code;
  const key = `events.${event.code}.title` as TranslationKey;
  const variables = [...translation(key).matchAll(/\{(\w+)\}/g)];
  if (variables.some(([, name]) => event.detail?.[name] == null) && event.title) return event.title;
  return t(key, event.detail);
}
export function eventHint(event: LocalizedEvent): string {
  if (!Object.prototype.hasOwnProperty.call(ja.events, event.code)) return event.hint || '';
  return t(`events.${event.code}.hint` as TranslationKey, event.detail) || event.hint || '';
}
