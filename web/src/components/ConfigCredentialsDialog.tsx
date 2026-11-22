import { useState } from 'react';
import { api } from '../api';
import { asArray } from '../format';
import { t } from '../i18n';
import type { SwitchConfig } from '../types';
import { usePolling } from '../usePolling';
import { Dialog } from './Dialog';
import { SwitchForm } from './SwitchForm';

export function ConfigCredentialsDialog({ profileId, busy, locked, close, run, refresh }: {
  profileId?: number;
  busy: boolean;
  locked: boolean;
  close: () => void;
  run: (action: () => Promise<void>) => void;
  refresh: () => Promise<unknown>;
}) {
  const [edit, setEdit] = useState<SwitchConfig>();
  const list = usePolling(signal => api<SwitchConfig[]>('/api/switches', { signal }), 0, [profileId]);
  if (edit && !locked && edit.profile_id === profileId) {
    return <SwitchForm value={edit} busy={busy} run={run} close={() => setEdit(undefined)}
      onSaved={async () => {
        await Promise.all([list.refresh(), refresh()]);
      }} />;
  }
  const pending = asArray(list.data).filter(sw => sw.needs_credentials);
  return <Dialog title={t('configFile.credentialsTitle')} close={close}>
    <p>{t('configFile.credentialsNote')}</p>
    {list.pending && <p role="status">{t('rack.busy')}</p>}
    {list.error && <p role="alert">⚠ {list.error}</p>}
    {list.data && pending.length === 0 && <p role="status">{t('configFile.credentialsComplete')}</p>}
    <ul className="space-y-2">
      {pending.map(sw => <li key={sw.id} className="flex flex-wrap gap-2">
        <span>{sw.name} ({sw.ip})</span>
        <button disabled={busy || locked} onClick={() => setEdit(sw)}>{t('configFile.enterCredentials')}</button>
      </li>)}
    </ul>
  </Dialog>;
}
