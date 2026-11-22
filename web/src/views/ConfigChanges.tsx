import { api } from '../api';
import { PollingStatus } from '../components/PollingStatus';
import { SEARCH_EVENT_LIMIT } from '../events';
import { asArray } from '../format';
import { eventHint, eventTitle, t } from '../i18n';
import { AlertTechnicalDetails } from '../components/AlertTechnicalDetails';
import { pcTimestamp } from '../pcTime';
import type { Event } from '../types';
import { TOPOLOGY_REFRESH_MS, usePolling } from '../usePolling';
import type { BaselineDiff } from './detailTables';
import { baselineDiffItem, localizedStatus } from './detailTables';
import type { ViewProps } from './props';
import { groupSwitches, hasGroupFilter, inGroupScope } from './props';

interface Baseline {
  id: number;
  name: string;
  captured_ts: string;
}

export function ConfigChanges(p: ViewProps) {
  const profile = p.status?.profile.id;
  const phase = p.operations?.phase;
  const comparison = usePolling(async signal => {
    const baselines = await api<Baseline[]>('/api/baselines', { signal });
    const latest = asArray(baselines)[0];
    if (!latest) return { latest, diffs: [] as BaselineDiff[] };
    const result = await api<{ diffs: BaselineDiff[] }>(`/api/baselines/${latest.id}/diff?read_only=1`, { signal });
    return { latest, diffs: asArray(result.diffs) };
  }, TOPOLOGY_REFRESH_MS, [profile, phase]);
  const history = usePolling(signal => api<Event[]>(`/api/events?category=CONFIG&severity=INFO&active_profile=1&limit=${SEARCH_EVENT_LIMIT}`, { signal }),
    TOPOLOGY_REFRESH_MS, [profile]);
  const switches = groupSwitches(p);
  const visibleIds = new Set(switches.map(sw => sw.id));
  const filtered = hasGroupFilter(p);
  const latest = comparison.data?.latest;
  const diffs = asArray(comparison.data?.diffs).filter(row => !filtered || visibleIds.has(row.switch_id));
  const events = asArray(history.data).filter(event => inGroupScope(event, visibleIds, filtered)).slice(0, 30);
  return <section className="panel space-y-4">
    <section>
      <h2>{t('configChanges.latestBaseline')}</h2>
      <PollingStatus {...comparison} />
      {comparison.data && (latest ? <>
        <p>{t('configChanges.comparing', { name: latest.name, time: pcTimestamp(latest.captured_ts, true) })}</p>
        {diffs.length ? <ul className="list-disc pl-5">{diffs.map((row, index) => <li key={index}>
          {t('configChanges.difference', {
            switchName: row.switch_name, item: baselineDiffItem(row),
            before: row.config_field === 'alias' ? row.before : localizedStatus(row.before) ?? row.before,
            after: row.config_field === 'alias' ? row.after : localizedStatus(row.after) ?? row.after,
          })}
        </li>)}</ul> : <p>{t('configChanges.noDiff')}</p>}
      </> : <p>{t('configChanges.noBaseline')}</p>)}
    </section>
    <section>
      <h2>{t('configChanges.recent')}</h2>
      <PollingStatus {...history} />
      {history.data && (events.length ? <ul className="list-disc pl-5">{events.map(event => <li key={event.id}>
        <time dateTime={event.ts}>{pcTimestamp(event.ts, true)}</time> · {event.switch_name} · {eventTitle(event)} {eventHint(event)}
        {p.expert && <AlertTechnicalDetails event={event} />}
      </li>)}</ul> : <p>{t('configChanges.noEvents')}</p>)}
    </section>
  </section>;
}
