import { useEffect, useState } from 'react';
import { api } from '../api';
import { Table } from '../components/Common';
import { asArray } from '../format';
import type { DhcpData } from '../types';

export function DhcpView() {
  const [data, setData] = useState<DhcpData>();
  useEffect(() => {
    api<DhcpData>('/api/dhcp').then(setData).catch(() => setData(undefined));
  }, []);
  return <section className="panel space-y-4">
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
