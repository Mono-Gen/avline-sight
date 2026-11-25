import { Children } from 'react';
import type { ReactNode } from 'react';
import { api, json } from '../api';
import { t } from '../i18n';
import type { Profile, Status } from '../types';

export function AppSidebar({ children, index, profiles, status, busy, locked, replay, expert, setExpert, openSettings, run, refresh }: {
  children: ReactNode;
  // Switch index inserted right below the first page button (port monitor), plan section 21.
  index?: ReactNode;
  profiles: Profile[];
  status?: Status;
  busy: boolean;
  locked: boolean;
  replay: boolean;
  expert: boolean;
  setExpert: (value: boolean) => void;
  openSettings: () => void;
  run: (action: () => Promise<void>) => void;
  refresh: () => Promise<void>;
}) {
  return <aside className="app-nav">
    <nav aria-label={t('project.navigation')}>{index ? Children.toArray(children).flatMap((child, i) => i === 0 ? [child, index] : [child]) : children}</nav>
    <div className="app-nav-tools">
      <label>{t('rack.profile')}
        <select value={status?.profile.id || ''} disabled={busy || locked} onChange={event => {
          const id = event.target.value;
          run(async () => { await api(`/api/profiles/${id}/activate`, json('PUT')); await refresh(); });
        }}>{profiles.map(profile => <option key={profile.id} value={profile.id}>{profile.name}</option>)}</select>
      </label>
      <button disabled={busy || replay || !status} onClick={() => run(async () => {
        const pin = status?.locked && status.pin_set ? prompt(t('rack.pin')) : undefined;
        if (pin === null) return;
        await api('/api/mode/lock', json('POST', { locked: !status?.locked, pin }));
        await refresh();
      })}>{t(locked ? 'rack.locked' : 'rack.lock')}</button>
      <button disabled={busy || !status} onClick={openSettings}>{t('rack.settings')}</button>
      <label className="app-nav-expert">{t('expert.setting')}
        <input type="checkbox" role="switch" checked={expert} onChange={event => setExpert(event.target.checked)} />
      </label>
    </div>
  </aside>;
}
