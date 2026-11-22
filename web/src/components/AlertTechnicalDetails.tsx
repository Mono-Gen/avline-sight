import { eventDetails, eventTitle, isRecommendationEvent, t } from '../i18n';
import type { LocalizedEvent } from '../i18n';

export function AlertTechnicalDetails({ event, counters, observed }: {
  event: LocalizedEvent; counters?: unknown; observed?: string;
}) {
  const description = eventDetails(event);
  const rows = Object.entries(counters && typeof counters === 'object' ? counters : {})
    .filter(([, value]) => typeof value === 'string' && /^\d+$/.test(value))
    .sort(([a], [b]) => a.localeCompare(b));
  const observation = observed === eventTitle(event) ? undefined : observed;
  if (!description && !rows.length && !observation) return null;
  return <details>
    <summary>{t('expert.technical')}</summary>
    {description && <p>{description}</p>}
    {observation && <p>{observation}</p>}
    {isRecommendationEvent(event.code) && <p>{t('expert.noSource')}</p>}
    <code>{event.code}</code>
    {rows.length > 0 && <div className="overflow-auto">
      <table className="text-sm">
        <thead><tr><th>{t('expert.mibName')}</th><th>{t('expert.rawCounter')}</th></tr></thead>
        <tbody>{rows.map(([name, value]) => <tr key={name}><td><code>{name}</code></td><td className="tabular-nums">{value}</td></tr>)}</tbody>
      </table>
    </div>}
  </details>;
}
