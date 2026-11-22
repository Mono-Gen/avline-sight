import { t } from '../i18n';
import { api } from '../api';
import { Table } from '../components/Common';
import { PollingStatus } from '../components/PollingStatus';
import { asArray } from '../format';
import type { DhcpData } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';

export function DhcpView() {
  const result = usePolling(signal => api<DhcpData>('/api/dhcp', { signal }), STATE_REFRESH_MS);
  const data = result.data;
  return <section className="panel space-y-4">
    <PollingStatus {...result} />
    {asArray((data?.servers || [])).filter(s => s.server_on).length > 1 && <p>{t('ui.m249')}</p>}
    <h2>{t('ui.m250')}</h2>
    <Table rows={data?.servers || []} />
    <h2>{t('ui.m251')}</h2>
    <Table rows={data?.leases || []} />
    <h2>{t('ui.m252')}</h2>
    <Table rows={data?.conflicts || []} />
    <h2>{t('ui.m253')}</h2>
    <Table rows={data?.findings || []} />
  </section>;
}
