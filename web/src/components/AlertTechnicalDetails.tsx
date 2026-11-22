import { t } from '../i18n';

export function AlertTechnicalDetails({ counters }: { counters: unknown }) {
  if (!counters || typeof counters !== 'object') return null;
  const rows = Object.entries(counters)
    .filter(([, value]) => typeof value === 'string' && /^\d+$/.test(value))
    .sort(([a], [b]) => a.localeCompare(b));
  if (!rows.length) return null;
  return <details>
    <summary>{t('expert.technical')}</summary>
    <div className="overflow-auto">
      <table className="text-sm">
        <thead><tr><th>{t('expert.mibName')}</th><th>{t('expert.rawCounter')}</th></tr></thead>
        <tbody>{rows.map(([name, value]) => <tr key={name}><td><code>{name}</code></td><td className="tabular-nums">{value}</td></tr>)}</tbody>
      </table>
    </div>
  </details>;
}
