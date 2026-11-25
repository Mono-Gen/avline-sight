import { t } from '../i18n';
import { useId, useState } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import type { SwitchConfig } from '../types';
import { Dialog } from './Common';
type Run = (f: () => Promise<void>) => void;
const fields = [['name', 'operations.fieldName'], ['ip', 'operations.fieldIp'], ['community', 'operations.fieldCommunity'], ['web_url', 'operations.fieldWebUrl']] as const;
export function SwitchForm({ value, close, run, busy, groups = [], onSaved, onDeleted }: {
  value?: SwitchConfig;
  close: () => void;
  run: Run;
  busy: boolean;
  groups?: string[];
  onSaved?: () => Promise<void>;
  // Given only where deleting belongs (the switch's own edit form), always behind a confirmation.
  onDeleted?: () => void;
}) {
  const listId = useId();
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
  const group = form.group_tag?.trim() || '';
  const invalidGroup = ['*', '.', '..'].includes(group);
  const missingCredentials = value?.needs_credentials && (!form.community || form.community === '****');
  return <Dialog title={value ? t('ui.m190') : t('ui.m191')} close={close}>
    {value?.needs_credentials && <p role="status">{t('configFile.credentialsNote')}</p>}
    {/* Same label-left rows as the settings page. */}
    <div className="dialog-form">
      {asArray(fields).map(([k, label]) => <div key={k} className="settings-row">
        <label className="settings-label" htmlFor={`${listId}-${k}`}>{t(label)}</label>
        <input id={`${listId}-${k}`}
          type={k === 'community' ? 'password' : 'text'}
          value={form[k] || ''}
          onChange={e => change(k, e.target.value)}
        />
      </div>)}
      <div className="settings-row">
        <label className="settings-label" htmlFor={`${listId}-group`}>{t('operations.memberGroup')}</label>
        <span>
          <input id={`${listId}-group`} list={listId} value={form.group_tag || ''} onChange={e => change('group_tag', e.target.value)} />
          <span className="settings-hint">{t('operations.memberGroupHelp')}</span>
        </span>
      </div>
      <datalist id={listId}>{groups.filter(Boolean).map(name => <option key={name} value={name} />)}</datalist>
      {invalidGroup && <p role="alert">⚠ {t('operations.invalidGroup')}</p>}
      <div className="settings-row">
        <label className="settings-label" htmlFor={`${listId}-port`}>{t('ui.m192')}</label>
        <input id={`${listId}-port`} type="number" className="settings-number" value={form.snmp_port} onChange={e => change('snmp_port', Number(e.target.value))} />
      </div>
      <div className="settings-row">
        <label className="settings-label" htmlFor={`${listId}-enabled`}>{t('ui.m009')}</label>
        <input id={`${listId}-enabled`} type="checkbox" checked={form.enabled} onChange={e => change('enabled', e.target.checked)} />
      </div>
    </div>
    <div className="settings-savebar">
    <button className="button-primary"
      disabled={busy || invalidGroup || missingCredentials}
      onClick={() => run(async () => {
        const request = json(value ? 'PUT' : 'POST', { ...form, group_tag: group });
        const headers = new Headers(request.headers);
        if (value) headers.set('X-AVLine-Profile', String(value.profile_id));
        await api(value ? `/api/switches/${value.id}` : '/api/switches', { ...request, headers });
        close();
        await onSaved?.();
      })}
    >{t('ui.m187')}</button>
    </div>
    {value && onDeleted && <div className="switch-form-danger">
      <button className="button-danger" data-action="delete-switch" disabled={busy} onClick={() => {
        if (window.confirm(t('rack.removeConfirm', { name: value.name }))) run(async () => {
          await api(`/api/switches/${value.id}`, json('DELETE'));
          close();
          onDeleted();
        });
      }}>{t('rack.remove')}</button>
    </div>}
  </Dialog>;
}
