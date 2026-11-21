import { useEffect, useState } from 'react';
import { api } from '../api';
import { Table } from '../components/Common';
import { asArray } from '../format';
import type { Port } from '../types';
import type { ViewProps } from './props';

export function PortMatrix(p: ViewProps) {
  const [rows, setRows] = useState<{
    switch_id: number;
    switch_name: string;
    port: Port;
  }[]>([]);
  const [q, setQ] = useState('');
  const [sort, setSort] = useState('err');
  const [anomaly, setAnomaly] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => {
      api<typeof rows>(`/api/ports/search?q=${encodeURIComponent(q)}&sort=${sort}`).then(setRows).catch(() => setRows([]));
    }, 300);
    return () => clearTimeout(t);
  }, [q, sort]);
  const preset = asArray(p.settings?.views)?.find(v => v.id === p.part);
  return <section className="panel">
    <input placeholder="機器名 / MAC / IP / VLAN" value={q} onChange={e => setQ(e.target.value)} />
    <select value={sort} onChange={e => setSort(e.target.value)}>
      <option value="err">エラー順
      </option>
      <option value="poe">PoE順
      </option>
      <option value="bw">帯域順
      </option>
    </select>
    <label>
      <input type="checkbox" checked={anomaly} onChange={e => setAnomaly(e.target.checked)} />
      異常のみ
    </label>
    <Table
      rows={asArray((rows || [])).filter(x => (!anomaly || x.port.err_delta > 0 || x.port.discard_delta > 0) && (!preset?.vlans?.length || asArray(preset.vlans).includes(x.port.pvid))).map(x => ({
        switch: x.switch_name,
        port: x.port.name,
        vlan: x.port.pvid,
        oper: x.port.oper,
        rx_bps: x.port.rx_bps,
        tx_bps: x.port.tx_bps,
        errors: x.port.err_delta,
        poe_w: x.port.poe?.power_w,
        devices: asArray((x.port.devices || [])).map(d => `${d.label || d.lldp_name || ''} ${d.mac} ${d.ip || ''} ${d.vendor || ''}`).join(', ')
      }))}
    />
  </section>;
}
