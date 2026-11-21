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
    {asArray((data?.servers || [])).filter(s => s.server_on).length > 1 && <p>⚠ 複数のDHCPサーバーが稼働しています
    </p>}
    <h2>サーバー・Snooping
    </h2>
    <Table rows={data?.servers || []} />
    <h2>リース
    </h2>
    <Table rows={data?.leases || []} />
    <h2>IP衝突
    </h2>
    <Table rows={data?.conflicts || []} />
    <h2>検出事項
    </h2>
    <Table rows={data?.findings || []} />
  </section>;
}
