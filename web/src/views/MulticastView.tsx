import { useEffect, useState } from 'react';
import { api } from '../api';
import { Table } from '../components/Common';
import { asArray } from '../format';
import type { Multicast } from '../types';
import type { ViewProps } from './props';

export function MulticastView(p: ViewProps) {
  const [data, setData] = useState<Multicast>();
  useEffect(() => {
    if (!p.replay) api<Multicast>('/api/multicast').then(setData).catch(() => setData(undefined));
  }, [p.replay]);
  const dataReplay = p.replay ? {
    vlans: asArray(p.replay).flatMap(s => asArray((s.igmp.vlans || [])).map(v => ({
      ...v,
      switch_id: s.id,
      switch_name: s.name
    }))),
    groups: asArray(p.replay).flatMap(s => asArray((s.igmp.groups || [])).map(v => ({
      ...v,
      switch_id: s.id,
      switch_name: s.name
    }))),
    mrouters: asArray(p.replay).flatMap(s => asArray((s.igmp.mrouters || [])).map(v => ({
      ...v,
      switch_id: s.id,
      switch_name: s.name
    }))),
    gptp: asArray(p.replay).flatMap(s => s.igmp.gptp ? [{
      ...s.igmp.gptp,
      switch_id: s.id,
      switch_name: s.name
    }] : []),
    findings: []
  } : data;
  const preset = asArray(p.settings?.views)?.find(v => v.id === p.part);
  const filt = <T extends {
    vlan: number;
  }>(a: T[] | undefined) => asArray((a || [])).filter(x => !preset?.vlans?.length || asArray(preset.vlans).includes(x.vlan));
  return <div className="panel space-y-4">
    <h2>VLAN クエリア
    </h2>
    <Table rows={filt(dataReplay?.vlans)} />
    <h2>グループ × ポート
    </h2>
    <Table rows={filt(dataReplay?.groups)} />
    <h2>マルチキャストルーター
    </h2>
    <Table rows={filt(dataReplay?.mrouters)} />
    <h2>gPTP
    </h2>
    <Table rows={dataReplay?.gptp || []} />
    <h2>検出事項
    </h2>
    <Table rows={dataReplay?.findings || []} />
  </div>;
}
