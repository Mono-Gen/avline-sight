import { t } from '../i18n';
import React from 'react';
import { asArray, decimal, formatBps } from '../format';
import { Badge } from './Badge';
import { columnLabels } from './columnLabels';

function cellValue(key: string, value: unknown): React.ReactNode {
  if (value == null) return '—';
  if (key === 'severity') return <Badge text={String(value)} />;
  if (typeof value === 'boolean') return value ? t('ui.m193') : t('ui.m194');
  if (typeof value === 'number') {
    if (key === 'rx_bps' || key === 'tx_bps') return formatBps(value);
    if (key.endsWith('_dbm')) return decimal(value, 2);
    if (key.endsWith('_w') || key.endsWith('_pct') || key.startsWith('cpu_')) return decimal(value);
    return value.toLocaleString('ja-JP');
  }
  if (Array.isArray(value)) return asArray(value).map(item => typeof item === 'object' ? JSON.stringify(item) : String(item)).join(' / ');
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

export function Table({ rows, labels = {} }: {
  rows: object[];
  labels?: Record<string, string>;
}) {
  const list = asArray(rows).filter(row => typeof row === 'object');
  if (!list.length) return <p>{t('ui.m195')}</p>;
  const keys = Array.from(new Set(asArray(list).flatMap(row => Object.keys(row))));
  return (
    <div className="overflow-auto">
      <table className="w-full text-sm">
        <thead>
          <tr>{asArray(keys).map(key => <th key={key}>{labels[key] || columnLabels[key] || key}</th>)}</tr>
        </thead>
        <tbody>
          {asArray(list).map((row, index) => (
            <tr key={index}>
              {asArray(keys).map(key => <td key={key}>{cellValue(key, (row as Record<string, unknown>)[key])}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
