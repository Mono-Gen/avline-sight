import { t } from '../i18n';
import React from 'react';
import { asArray, decimal } from '../format';
import type { Port } from '../types';

function Details({ title, rows }: {
  title: string;
  rows: [string, React.ReactNode][];
}) {
  return (
    <section className="port-hardware-section">
      <h3 className="font-bold">{title}</h3>
      <dl className="port-hardware-fields">
        {asArray(rows).map(([label, value]) => (
          <React.Fragment key={label}>
            <dt className="whitespace-nowrap">{label}</dt><dd>{value ?? '—'}</dd>
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
    <div className="port-hardware grid gap-4">
      {poe && <Details
        title="PoE"
        rows={[
          [t('ui.m148'), poe.detect],
          [t('ui.m149'), `${decimal(poe.power_w)} W`],
          [t('ui.m150'), poe.class],
          [t('ui.m151'), `${decimal(poe.current_ma)} mA`],
          [t('ui.m152'), `${decimal(poe.volts)} V`],
          [t('ui.m153'), poe.fault || t('ui.m154')],
          [t('ui.m155'), poe.denied_cnt],
          [t('ui.m156'), poe.overload_cnt],
          [t('ui.m157'), poe.short_cnt],
        ]}
      />}
      {sfp && <Details
        title={t('ui.m158')}
        rows={[
          [t('ui.m159'), `${decimal(sfp.rx_dbm, 2)} dBm`],
          [t('ui.m160'), `${decimal(sfp.tx_dbm, 2)} dBm`],
          [t('ui.m161'), `${decimal(sfp.temp_c)} ℃`],
          [t('ui.m152'), `${decimal(sfp.volts, 2)} V`],
          [t('ui.m162'), `${decimal(sfp.bias_ma)} mA`],
          [t('ui.m074'), sfp.vendor],
          [t('ui.m075'), sfp.part_no],
          [t('ui.m076'), sfp.supported],
          [t('ui.m072'), sfp.los ? t('ui.m163') : t('ui.m164')],
          [t('ui.m073'), sfp.tx_fault ? t('ui.m163') : t('ui.m164')],
        ]}
      />}
      {cable && <Details
        title={t('ui.m165')}
        rows={[
          [t('ui.m063'), cable.status],
          [t('ui.m166'), `${cable.min_len_m} m`],
          [t('ui.m167'), `${cable.max_len_m} m`],
          [t('ui.m168'), `${cable.fail_loc_m} m`],
        ]}
      />}
    </div>
  );
}
