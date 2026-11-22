import { t } from '../i18n';
import type { TranslationKey } from '../i18n';

export const switchColumn = { first: ['switch_name'], hide: ['switch_id'] };
export const findingColumns = {
  first: ['switch_name', 'title', 'severity', 'port', 'hint', 'code'],
  hide: switchColumn.hide,
  values: {
    switch_name: (value: unknown) => value === '' ? '—' : undefined,
  },
};

export function localizedStatus(value: unknown) {
  const keys: Record<string, TranslationKey> = {
    querier: 'pages.querier', 'non-querier': 'pages.nonQuerier',
    disabled: 'pages.disabled', enabled: 'pages.enabled', on: 'pages.enabled', off: 'pages.disabled',
    operational: 'pages.operational', failed: 'pages.failed', notpresent: 'pages.notpresent',
    powering: 'pages.powering', nopower: 'pages.nopower', notpowering: 'pages.notpowering',
    incompatible: 'pages.incompatible', low: 'pages.low', normal: 'pages.normal',
    warning: 'pages.warning', critical: 'pages.critical', shutdown: 'pages.shutdown',
    notoperational: 'pages.notoperational', up: 'pages.up', down: 'pages.down',
    unknown: 'pages.unknown', true: 'ui.m193', false: 'ui.m194',
  };
  const normalized = typeof value === 'string' ? value.toLowerCase() : '';
  const key = Object.prototype.hasOwnProperty.call(keys, normalized) ? keys[normalized] : undefined;
  return key ? t(key) : value === '' ? '—' : undefined;
}

export function enabledStatus(value: unknown) {
  return typeof value === 'boolean' ? t(value ? 'pages.enabled' : 'pages.disabled') : undefined;
}
