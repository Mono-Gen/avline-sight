import { t } from '../i18n';
import { useState } from 'react';
import { api } from '../api';
import { Table } from '../components/Common';
import { PollingStatus } from '../components/PollingStatus';
import { asArray } from '../format';
import type { Port } from '../types';
import { STATE_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';
import { hasGroupFilter, inGroupScope, partPreset } from './props';

export function PortMatrix(p: ViewProps & { visibleSwitchIds: ReadonlySet<number> }) {
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
  const vlans = asArray(partPreset(p)?.vlans);
  const groupFiltered = hasGroupFilter(p);
  return <section className="panel">
    <div className="port-list-controls">
      {!p.replay && <PollingStatus {...result} />}
      <input type="search" aria-label={t('filters.portSearch')} placeholder={t('ui.m274')}
        value={q} onChange={event => setQ(event.target.value)} />
      <select aria-label={t('filters.portSort')} value={sort} onChange={event => setSort(event.target.value)}>
        <option value="err">{t('ui.m275')}</option>
        <option value="poe">{t('ui.m276')}</option>
        <option value="bw">{t('ui.m277')}</option>
      </select>
      <label>
        <input type="checkbox" checked={anomaly} onChange={event => setAnomaly(event.target.checked)} />
        {t('ui.m278')}
      </label>
    </div>
    <Table
      resetKey={sort}
      hide={['deviceDetails']}
      values={{
        oper: value => value === 'up' ? t('filters.linkConnected') : value === 'down' ? t('filters.linkDisconnected') : undefined,
        devices: (_value, row) => row.deviceDetails.length ? row.deviceDetails.map((device, index) =>
          <div key={`${device.mac}/${index}`}>
            <strong>{device.label || device.lldp_name || '—'}</strong>
            <small className="port-list-device-meta">{[device.mac, device.ip, device.vendor].filter(Boolean).join(' · ')}</small>
          </div>) : '—',
      }}
      rows={asArray(rows).filter(x => inGroupScope(x, p.visibleSwitchIds, groupFiltered) &&
        (!q || [x.switch_name, x.port.name, x.port.memo, x.port.pvid, ...asArray(x.port.devices).flatMap(d => [d.label, d.lldp_name, d.mac, d.ip, d.vendor])].join(' ').toLowerCase().includes(q.trim().toLowerCase())) &&
        (!anomaly || x.port.err_delta > 0 || x.port.discard_delta > 0) &&
        (!x.switch_id || !vlans.length || vlans.includes(x.port.pvid))).map(x => ({
        switch: x.switch_name,
        port: x.port.name,
        vlan: x.port.pvid,
        oper: x.port.oper,
        rx_bps: x.port.rx_bps,
        tx_bps: x.port.tx_bps,
        errors: x.port.err_delta,
        poe_w: x.port.poe?.power_w,
        devices: asArray(x.port.devices).map(device => device.label || device.lldp_name || device.mac).join(', '),
        deviceDetails: asArray(x.port.devices)
      }))}
    />
  </section>;
}
