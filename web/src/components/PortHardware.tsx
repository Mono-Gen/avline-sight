import React from 'react';
import { asArray, decimal } from '../format';
import type { Port } from '../types';

function Details({ title, rows }: {
  title: string;
  rows: [string, React.ReactNode][];
}) {
  return (
    <section>
      <h3 className="font-bold">{title}</h3>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
        {asArray(rows).map(([label, value]) => (
          <React.Fragment key={label}>
            <dt>{label}</dt><dd>{value ?? '—'}</dd>
          </React.Fragment>
        ))}
      </dl>
    </section>
  );
}

export function PortHardware({ port }: {
  port: Port;
}) {
  const { poe, sfp, cable } = port;
  return (
    <div className="grid gap-4 md:grid-cols-3">
      {poe && <Details
        title="PoE"
        rows={[
          ['検出状態', poe.detect],
          ['電力', `${decimal(poe.power_w)} W`],
          ['クラス', poe.class],
          ['電流', `${decimal(poe.current_ma)} mA`],
          ['電圧', `${decimal(poe.volts)} V`],
          ['障害', poe.fault || 'なし'],
          ['給電拒否回数', poe.denied_cnt],
          ['過負荷回数', poe.overload_cnt],
          ['短絡回数', poe.short_cnt],
        ]}
      />}
      {sfp && <Details
        title="SFP 光モジュール"
        rows={[
          ['受信光', `${decimal(sfp.rx_dbm, 2)} dBm`],
          ['送信光', `${decimal(sfp.tx_dbm, 2)} dBm`],
          ['温度', `${decimal(sfp.temp_c)} ℃`],
          ['電圧', `${decimal(sfp.volts, 2)} V`],
          ['バイアス電流', `${decimal(sfp.bias_ma)} mA`],
          ['メーカー', sfp.vendor],
          ['型番', sfp.part_no],
          ['対応状態', sfp.supported],
          ['光信号喪失', sfp.los ? '⚠ あり' : '✓ なし'],
          ['送信障害', sfp.tx_fault ? '⚠ あり' : '✓ なし'],
        ]}
      />}
      {cable && <Details
        title="ケーブル診断"
        rows={[
          ['状態', cable.status],
          ['最短長', `${cable.min_len_m} m`],
          ['最長長', `${cable.max_len_m} m`],
          ['障害位置', `${cable.fail_loc_m} m`],
        ]}
      />}
    </div>
  );
}
