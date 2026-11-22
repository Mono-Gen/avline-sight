import { t } from '../i18n';
import { useState } from 'react';
import { api } from '../api';
import { Table } from '../components/Common';
import { PollingStatus } from '../components/PollingStatus';
import { asArray } from '../format';
import type { Port } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';
import { groupSwitches, hasGroupFilter } from './props';

export function PortMatrix(p: ViewProps) {
  const [q, setQ] = useState('');
  const [sort, setSort] = useState('err');
  const [anomaly, setAnomaly] = useState(false);
  const result = usePolling(signal => api<{
    switch_id: number;
    switch_name: string;
    port: Port
  }[]>(
    `/api/ports/search?q=${encodeURIComponent(q)}&sort=${sort}`, { signal }),
    STATE_REFRESH_MS, [q, sort], !p.replay, 300);
  const rows = p.replay ? asArray(p.replay).flatMap(sw => asArray(sw.ports).map(port => ({ switch_id: sw.id, switch_name: sw.name, port }))) : result.data;
  const preset = asArray(p.settings?.views)?.find(v => v.id === p.part);
  const groupFiltered = hasGroupFilter(p);
  const visible = new Set(groupSwitches(p).map(sw => sw.id));
  return <section className="panel">
    {!p.replay && <PollingStatus {...result} />}
    <input placeholder={t('ui.m274')} value={q} onChange={e => setQ(e.target.value)} />
    <select value={sort} onChange={e => setSort(e.target.value)}>
      <option value="err">{t('ui.m275')}</option>
      <option value="poe">{t('ui.m276')}</option>
      <option value="bw">{t('ui.m277')}</option>
    </select>
    <label>
      <input type="checkbox" checked={anomaly} onChange={e => setAnomaly(e.target.checked)} />
      {t('ui.m278')}</label>
    <Table
      rows={asArray(rows).filter(x => (!groupFiltered || visible.has(x.switch_id)) &&
        (!q || [x.switch_name, x.port.name, x.port.memo, ...asArray(x.port.devices).flatMap(d => [d.label, d.mac, d.ip])].join(' ').toLowerCase().includes(q.toLowerCase())) &&
        (!anomaly || x.port.err_delta > 0 || x.port.discard_delta > 0) &&
        (!preset?.vlans?.length || asArray(preset.vlans).includes(x.port.pvid))).map(x => ({
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
