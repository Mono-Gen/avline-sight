import { useSyncExternalStore } from 'react';
import { ja } from './ja';
import { en } from './en';
import type { EventCopy } from './eventJa';

export type Language = 'ja' | 'en';
export type Widen<T> = { [K in keyof T]: T[K] extends string ? string : Widen<T[K]> };
export type Dictionary = Omit<Widen<typeof ja>, 'events'> & { events: Record<keyof typeof ja.events, EventCopy> };
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
export interface LocalizedEvent {
  code: string;
  title?: string;
  hint?: string;
  detail?: Record<string, unknown>;
}

function copyFor(code: string): EventCopy | undefined {
  if (!Object.prototype.hasOwnProperty.call(ja.events, code)) return undefined;
  return (language === 'en' ? en : ja).events[code as keyof typeof ja.events];
}

function renderEventCopy(template: string, params: Record<string, unknown> = {}): string | undefined {
  if ([...template.matchAll(/\{(\w+)\}/g)].some(([, name]) => params[name] == null)) return undefined;
  return template.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name]));
}

const recommendationCodes = new Set([
  'EEE_ON_AV_PORT', 'FLOWCTRL_ON_AV_PORT', 'NDI4_NEEDS_FLOWCTRL', 'DSCP_TRUST', 'QSYS_MTU',
  'STORM_CTRL_UPLINK', 'BCAST_STORM_CTRL_UPLINK', 'MCAST_STORM_CTRL_UPLINK', 'UPLINK_MTU_MISMATCH',
  'STP_MSTP', 'FW_MISMATCH', 'STP_PRIORITY_DEFAULT', 'STP_EDGE_ROOT', 'QUERIER_ON_EDGE', 'DAISY_CHAIN', 'VLAN_CROWDED',
]);

export function isRecommendationEvent(code: string): boolean {
  return recommendationCodes.has(code);
}

export function eventTitle(event: LocalizedEvent): string {
  const copy = copyFor(event.code);
  if (!copy) return event.title || event.code;
  if (event.code === 'MARKER' && event.detail?.note == null && event.title) return event.title;
  const title = renderEventCopy(copy.title, eventParams(event)) ?? copy.fallbackTitle ?? event.title ?? event.code;
  const vlan = event.detail?.vlan;
  return !isRecommendationEvent(event.code) && typeof vlan === 'number' ? `VLAN ${vlan} · ${title}` : title;
}

function eventParams(event: LocalizedEvent): Record<string, unknown> | undefined {
  const detail = event.detail;
  if (event.code === 'PORT_CONFIG_CHANGED' && typeof detail?.field === 'string') {
    return { ...detail, field: configFieldName(detail.field) };
  }
  return detail;
}

export function configFieldName(field: string): string {
  return Object.prototype.hasOwnProperty.call(ja.configChanges.fields, field)
    ? t(`configChanges.fields.${field as keyof typeof ja.configChanges.fields}`)
    : field;
}
export function eventHint(event: LocalizedEvent): string {
  const copy = copyFor(event.code);
  return copy ? renderEventCopy(copy.hint, event.detail) ?? '' : event.hint || '';
}

export function eventDetails(event: LocalizedEvent): string {
  const template = copyFor(event.code)?.details;
  if (!template) return '';
  const detail = eventParams(event);
  if (event.code === 'PORT_CONFIG_CHANGED' && typeof detail?.field === 'string') {
    const change = renderEventCopy(translation('configChanges.change'), detail);
    if (change) return change;
  }
  return renderEventCopy(template, detail) ?? '';
}
