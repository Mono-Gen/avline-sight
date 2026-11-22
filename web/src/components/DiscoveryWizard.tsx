import { t } from '../i18n';
import { useEffect, useState } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import { Dialog } from './Common';
type Run = (f: () => Promise<void>) => void;
export function DiscoveryWizard({ close, run, busy }: {
  close: () => void;
  run: Run;
  busy: boolean;
}) {
  const [cidr, setCidr] = useState('');
  const [community, setCommunity] = useState('');
  const [found, setFound] = useState<{
    ip: string;
    port: number;
    sys_name: string;
    registered: boolean;
  }[]>([]);
  const [candidates, setCandidates] = useState<{
    ip: string;
    sys_name: string;
    via_switch: string;
    via_port: string;
  }[]>([]);
  useEffect(() => {
    api<typeof candidates>('/api/discovery/lldp-candidates').then(setCandidates).catch(() => setCandidates([]));
  }, []);
  return <Dialog title={t('ui.m118')} close={close}>
    <input placeholder="CIDR" value={cidr} onChange={e => setCidr(e.target.value)} />
    <input type="password" placeholder={t('ui.m119')} value={community} onChange={e => setCommunity(e.target.value)} />
    <button
      disabled={busy}
      onClick={() => run(async () => {
        const r = await api<{
          found: typeof found;
        }>('/api/discovery/scan', json('POST', {
          cidr,
          community
        }));
        setFound(r.found || []);
      })}
    >{t('ui.m120')}</button>
    {asArray(found).map(f => <div key={f.ip}>
      {f.ip}

      {f.sys_name}

      <button
        disabled={busy || f.registered}
        onClick={() => run(async () => {
          await api('/api/switches', json('POST', {
            name: f.sys_name || f.ip,
            ip: f.ip,
            snmp_port: f.port,
            community,
            enabled: true,
            group_tag: ''
          }));
          setFound(asArray(found).map(x => x.ip === f.ip ? {
            ...x,
            registered: true
          } : x));
        })}
      >{t('ui.m121')}</button>
    </div>)}
    <h3>{t('ui.m122')}</h3>
    {asArray(candidates).map(c => <div key={c.ip}>
      {c.ip}

      {c.sys_name}
      (
      {c.via_switch}

      {c.via_port}
      )
      <button
        disabled={busy}
        onClick={() => run(async () => {
          await api('/api/switches', json('POST', {
            name: c.sys_name || c.ip,
            ip: c.ip,
            snmp_port: 161,
            community,
            enabled: true,
            group_tag: ''
          }));
          setCandidates(asArray(candidates).filter(x => x.ip !== c.ip));
        })}
      >{t('ui.m121')}</button>
    </div>)}
  </Dialog>;
}
