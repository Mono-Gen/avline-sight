import { t } from '../i18n';
import { useState } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import type { SwitchConfig } from '../types';
import { Dialog } from './Common';
type Run = (f: () => Promise<void>) => void;
export function SwitchForm({ value, close, run, busy }: {
  value?: SwitchConfig;
  close: () => void;
  run: Run;
  busy: boolean;
}) {
  const [form, setForm] = useState<Partial<SwitchConfig>>(value ? {
    ...value,
    community: ''
  } : {
    name: '',
    ip: '',
    group_tag: '',
    snmp_port: 161,
    community: '',
    enabled: true
  });
  const change = (key: string, val: unknown) => setForm({
    ...form,
    [key]: val
  });
  return <Dialog title={value ? t('ui.m190') : t('ui.m191')} close={close}>
    <div className="grid gap-2">
      {asArray((['name', 'ip', 'group_tag', 'community', 'web_url'] as const)).map(k => <label key={k}>
        {k}

        <input
          type={k === 'community' ? 'password' : 'text'}
          value={form[k] || ''}
          onChange={e => change(k, e.target.value)}
        />
      </label>)}
      <label>{t('ui.m192')}<input type="number" value={form.snmp_port} onChange={e => change('snmp_port', Number(e.target.value))} />
      </label>
      <label>
        <input type="checkbox" checked={form.enabled} onChange={e => change('enabled', e.target.checked)} />
        {t('ui.m009')}</label>
    </div>
    <button
      disabled={busy}
      onClick={() => run(async () => {
        await api(value ? `/api/switches/${value.id}` : '/api/switches', json(value ? 'PUT' : 'POST', form));
        close();
      })}
    >{t('ui.m187')}</button>
  </Dialog>;
}
