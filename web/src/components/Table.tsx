import { eventHint, eventTitle, getLang, isRecommendationEvent, t } from '../i18n';
import React, { useState } from 'react';
import { asArray, decimal, formatBps, formatTime } from '../format';
import { Badge } from './Badge';
import { columnLabels } from './columnLabels';
import { AlertTechnicalDetails } from './AlertTechnicalDetails';

function isEmpty(value: unknown) {
  return value == null || (typeof value === 'string' && !value.trim()) || (Array.isArray(value) && !value.length);
}

function compareValues(left: unknown, right: unknown, direction: 'ascending' | 'descending', collator: Intl.Collator) {
  const leftEmpty = isEmpty(left);
  const rightEmpty = isEmpty(right);
  if (leftEmpty || rightEmpty) return Number(leftEmpty) - Number(rightEmpty);
  let result: number;
  if (typeof left === 'number' && typeof right === 'number') result = left - right;
  else if (typeof left === 'number') result = -1;
  else if (typeof right === 'number') result = 1;
  else result = collator.compare(String(left), String(right));
  return direction === 'ascending' ? result : -result;
}

// Sort by the text shown on screen (event titles and hints use their displayed wording, not raw values).
function sortValue(key: string, row: Record<string, unknown>) {
  const value = row[key];
  if ((key === 'title' || key === 'hint') && typeof row.code === 'string') {
    const detail = row.detail && typeof row.detail === 'object' ? row.detail as Record<string, unknown> : undefined;
    const text = typeof value === 'string' ? value : undefined;
    return key === 'hint' ? eventHint({ code: row.code, hint: text, detail }) : eventTitle({ code: row.code, title: text, detail });
  }
  return value;
}

function cellValue(key: string, value: unknown, row: Record<string, unknown>, expert: boolean): React.ReactNode {
  if ((key === 'title' || key === 'hint') && typeof row.code === 'string') {
    const detail = row.detail && typeof row.detail === 'object' ? row.detail as Record<string, unknown> : undefined;
    const text = typeof value === 'string' ? value : undefined;
    if (key === 'hint') return eventHint({ code: row.code, hint: text, detail });
    const event = { code: row.code, title: text, detail };
    return <>{eventTitle(event)}{expert && <AlertTechnicalDetails event={event}
      observed={isRecommendationEvent(event.code) ? undefined : text} />}</>;
  }
  if (value == null) return '—';
  if (key === 'severity') return <Badge text={String(value)} />;
  if (typeof value === 'string' && (key === 'ts' || key.endsWith('_ts') || key === 'cleared_at' || key === 'acked_at')) return formatTime(value);
  if (typeof value === 'boolean') return value ? t('ui.m193') : t('ui.m194');
  if (typeof value === 'number') {
    if (key === 'rx_bps' || key === 'tx_bps') return formatBps(value);
    if (key.endsWith('_dbm')) return decimal(value, 2);
    if (key.endsWith('_w') || key.endsWith('_pct') || key.startsWith('cpu_')) return decimal(value);
    return value.toLocaleString(getLang() === 'ja' ? 'ja-JP' : 'en-US');
  }
  if (Array.isArray(value)) return asArray(value).map(item => typeof item === 'object' ? JSON.stringify(item) : String(item)).join(' / ');
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

export function Table<Row extends object>({ rows, labels = {}, first = [], hide = [], values = {}, expert = false, resetKey }: {
  rows: Row[];
  labels?: Record<string, string>;
  first?: readonly string[];
  hide?: readonly string[];
  values?: Record<string, (value: unknown, row: Row) => React.ReactNode>;
  expert?: boolean;
  // Header sorting is dropped when the screen's own sort order changes.
  resetKey?: unknown;
}) {
  const [stored, setSort] = useState<{ key: string; direction: 'ascending' | 'descending'; resetKey?: unknown }>();
  const sort = stored && stored.resetKey === resetKey ? stored : undefined;
  const list = asArray(rows).filter(row => row && typeof row === 'object') as (Row & Record<string, unknown>)[];
  if (!list.length) return <p>{t('ui.m195')}</p>;
  const keys = [...new Set([...first, ...list.flatMap(row => Object.keys(row))])].filter(key => !hide.includes(key));
  const entries = list.map((row, index) => ({ row, index }));
  if (sort && keys.includes(sort.key)) {
    const collator = new Intl.Collator(getLang(), { numeric: true });
    entries.sort((left, right) => compareValues(sortValue(sort.key, left.row), sortValue(sort.key, right.row), sort.direction, collator));
  }
  return (
    <div className="overflow-auto">
      <table className="w-full text-sm">
        <thead>
          <tr>{keys.map(key => {
            const direction = sort?.key === key ? sort.direction : undefined;
            return <th key={key} scope="col" aria-sort={direction}>
              <button type="button" className="border-0 bg-transparent p-0 text-left text-inherit"
                onClick={() => setSort(current => {
                  const active = current && current.resetKey === resetKey ? current : undefined;
                  return active?.key !== key ? { key, direction: 'ascending', resetKey }
                    : active.direction === 'ascending' ? { key, direction: 'descending', resetKey } : undefined;
                })}>
                {labels[key] || columnLabels[key] || key}{direction && <span aria-hidden="true"> {direction === 'ascending' ? '▲' : '▼'}</span>}
              </button>
            </th>;
          })}</tr>
        </thead>
        <tbody>
          {entries.map(({ row, index }) => (
            <tr key={index}>
              {keys.map(key => {
                const formatValue = Object.prototype.hasOwnProperty.call(values, key) ? values[key] : undefined;
                return <td key={key}>{formatValue?.(row[key], row) ?? cellValue(key, row[key], row, expert)}</td>;
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
