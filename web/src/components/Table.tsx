import { eventHint, eventTitle, getLang, isRecommendationEvent, t } from '../i18n';
import React from 'react';
import { asArray, decimal, formatBps, formatTime } from '../format';
import { Badge } from './Badge';
import { columnLabels } from './columnLabels';
import { AlertTechnicalDetails } from './AlertTechnicalDetails';

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

export function Table({ rows, labels = {}, first = [], hide = [], values = {}, expert = false }: {
  rows: object[];
  labels?: Record<string, string>;
  first?: readonly string[];
  hide?: readonly string[];
  values?: Record<string, (value: unknown) => React.ReactNode>;
  expert?: boolean;
}) {
  const list = asArray(rows).filter(row => row && typeof row === 'object');
  if (!list.length) return <p>{t('ui.m195')}</p>;
  const keys = [...new Set([...first, ...list.flatMap(row => Object.keys(row))])].filter(key => !hide.includes(key));
  return (
    <div className="overflow-auto">
      <table className="w-full text-sm">
        <thead>
          <tr>{asArray(keys).map(key => <th key={key}>{labels[key] || columnLabels[key] || key}</th>)}</tr>
        </thead>
        <tbody>
          {asArray(list).map((row, index) => (
            <tr key={index}>
              {keys.map(key => {
                const record = row as Record<string, unknown>;
                const formatValue = Object.prototype.hasOwnProperty.call(values, key) ? values[key] : undefined;
                return <td key={key}>{formatValue?.(record[key]) ?? cellValue(key, record[key], record, expert)}</td>;
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
