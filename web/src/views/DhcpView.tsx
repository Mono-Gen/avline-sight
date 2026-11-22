import { t } from '../i18n';
import { api } from '../api';
import { Table } from '../components/Common';
import { PollingStatus } from '../components/PollingStatus';
import { asArray } from '../format';
import type { DhcpData } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import { enabledStatus, findingColumns, switchColumn } from './detailTables';
import { groupSwitches, hasGroupFilter, inGroupScope } from './props';
import type { ViewProps } from './props';

export function DhcpView(props: ViewProps) {
  const result = usePolling(signal => api<DhcpData>('/api/dhcp', { signal }), STATE_REFRESH_MS);
  const data = result.data;
  const filtered = hasGroupFilter(props);
  const visibleIds = new Set(groupSwitches(props).map(sw => sw.id));
  const filter = <T extends { switch_id?: number }>(rows: T[] | undefined) =>
    asArray(rows).filter(row => inGroupScope(row, visibleIds, filtered));
  const servers = filter(data?.servers);
  return <section className="panel space-y-4">
    <PollingStatus {...result} />
    {asArray(data?.servers).filter(s => s.server_on).length > 1 && <p>{t('ui.m249')}</p>}
    <h2>{t('ui.m250')}</h2>
    <Table {...switchColumn} first={['switch_name', 'server_on', 'snooping_on']} rows={servers}
      values={{ server_on: enabledStatus, snooping_on: enabledStatus }} />
    <h2>{t('ui.m251')}</h2>
    <Table {...switchColumn} first={['switch_name', 'ip', 'mac']} rows={filter(data?.leases)} />
    <h2>{t('ui.m252')}</h2>
    <Table {...switchColumn} first={['switch_name', 'ip']} rows={filter(data?.conflicts)} />
    <h2>{t('ui.m253')}</h2>
    <Table {...findingColumns} rows={filter(data?.findings)} />
  </section>;
}
