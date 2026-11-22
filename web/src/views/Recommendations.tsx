import { api } from '../api';
import { PollingStatus } from '../components/PollingStatus';
import { asArray } from '../format';
import { eventDetails, eventTitle, isRecommendationEvent, t } from '../i18n';
import type { Audit, PortRecommendation } from '../types';
import { TOPOLOGY_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';
import { groupSwitches } from './props';

export function RecommendationTable({ rows, names }: {
  rows: PortRecommendation[]; names: ReadonlyMap<number, string>;
}) {
  if (!rows.length) return <p>{t('expert.noRecommendations')}</p>;
  return <div className="overflow-auto">
    <table className="w-full text-sm">
      <thead><tr><th>{t('ui.m018')}</th><th>{t('expert.port')}</th><th>{t('expert.field')}</th>
        <th>{t('expert.current')}</th><th>{t('expert.recommended')}</th><th>{t('expert.reference')}</th></tr></thead>
      <tbody>{rows.map((row, index) => {
        const mtu = row.code === 'QSYS_MTU' || row.code === 'UPLINK_MTU_MISMATCH';
        const recommended = row.code === 'QSYS_MTU' ? t('expert.qsysMtu', { frame: row.recommended })
          : row.code === 'UPLINK_MTU_MISMATCH' ? t('expert.peerMtu', { frame: row.recommended }) : row.recommended;
        const event = { code: row.code, detail: { pause: row.current, trust: row.current, max_frame: row.current, lpi: row.lpi } };
        const recommendation = isRecommendationEvent(row.code);
        const source = row.source && row.version
          ? t('expert.source', { name: row.source, version: row.version }) : t('expert.noSource');
        return <tr key={`${row.switch_id}/${row.port}/${row.code}/${index}`}>
          <td>{names.get(row.switch_id)}</td><td>{row.port}</td>
          <td>{recommendation ? eventTitle(event) : t(`expert.rules.${row.code}`)}</td>
          <td>{mtu ? `${row.current} B` : row.current}</td><td>{recommended}</td>
          <td>{recommendation && <p>{eventDetails(event)}</p>}{source}</td>
        </tr>;
      })}</tbody>
    </table>
  </div>;
}

export function Recommendations(props: ViewProps & { replayRows?: PortRecommendation[] }) {
  const polling = usePolling(signal => api<Audit>('/api/audit', { signal }), TOPOLOGY_REFRESH_MS,
    [props.status?.profile.id], !props.replay);
  const switches = new Map(groupSwitches(props).map(sw => [sw.id, sw.name]));
  const rows = props.replay ? props.replayRows : polling.data?.port_recommendations;
  return <section className="panel space-y-2" aria-labelledby="recommendations-title">
    <h2 id="recommendations-title">{t('expert.recommendations')}</h2>
    {!props.replay && <PollingStatus {...polling} />}
    {rows && <RecommendationTable rows={asArray(rows).filter(row => switches.has(row.switch_id))} names={switches} />}
  </section>;
}
