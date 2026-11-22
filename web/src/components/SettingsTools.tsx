import { useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { deviceUrl } from '../deviceCsv';
import { asArray } from '../format';
import { t, useLang } from '../i18n';
import type { ReplaySession, SwitchGroup } from '../types';
import { GroupEditDialog } from './GroupEditDialog';
import { ImportReplay } from './ImportReplay';

export function SettingsTools({ theme, setTheme, notifications, setNotifications, expert = false, setExpert, busy, locked, replay, groups, profileId, run, refresh, setDialog, setReplay }: {
  theme: string; setTheme: (value: string) => void;
  notifications: boolean; setNotifications: (value: boolean) => void;
  expert?: boolean; setExpert: (enabled: boolean) => void;
  busy: boolean; locked: boolean; replay: boolean; groups: SwitchGroup[]; profileId?: number;
  run: (action: () => Promise<void>) => void; refresh: () => Promise<unknown>;
  setDialog: (value: string) => void; setReplay: Dispatch<SetStateAction<ReplaySession | null>>;
}) {
  const [language, setLanguage] = useLang();
  const [scope, setScope] = useState('*');
  const [order, setOrder] = useState(false);
  const [notice, setNotice] = useState('');
  const toggleNotifications = () => {
    if (notifications) { setNotifications(false); return; }
    if (typeof Notification === 'undefined') { setNotice(t('cleanup.notificationDenied')); return; }
    run(async () => {
      const result = await Notification.requestPermission();
      setNotifications(result === 'granted');
      setNotice(result === 'granted' ? '' : t('cleanup.notificationDenied'));
    });
  };
  const csv = deviceUrl({ scope: 'all', q: '', vlans: [], group: scope === '*' ? undefined : scope.slice(6) }, undefined, true);
  return <>
    <section className="settings-tools space-y-3">
      <h3>{t('cleanup.localSettings')}</h3>
      <label>{t('rack.language')} <select value={language} onChange={event => setLanguage(event.target.value === 'en' ? 'en' : 'ja')}>
        <option value="ja">JA</option><option value="en">English</option>
      </select></label>
      <button onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>{t(theme === 'dark' ? 'rack.light' : 'rack.dark')}</button>
      <label className="block"><input type="checkbox" checked={expert} onChange={event => setExpert(event.target.checked)} /> {t('expert.setting')}</label>
      <label className="block"><input type="checkbox" checked={notifications} disabled={busy} onChange={toggleNotifications} /> {t('cleanup.notifications')}</label>
      {notice && <p role="alert">⚠ {notice}</p>}
      <h3>{t('cleanup.transfers')}</h3>
      <button disabled={busy || locked} onClick={() => setDialog('export')}>{t('rack.export')}</button>
      <ImportReplay run={run} disabled={busy || locked} onImport={(id, timeline) => { setReplay({ id, timeline, states: [] }); setDialog(''); }} />
      {!replay && <div className="flex flex-wrap gap-2">
        <label>{t('cleanup.csvGroup')} <select value={scope} onChange={event => setScope(event.target.value)}>
          <option value="*">{t('rack.all')}</option>
          {asArray(groups).map(group => <option key={group.name} value={`group:${group.name}`}>{group.name || t('rack.ungrouped')}</option>)}
        </select></label>
        <a href={csv} download>{t('rack.csv')}</a>
      </div>}
      <button disabled={busy || locked} onClick={() => setOrder(true)}>{t('operations.order')}</button>
    </section>
    {order && !locked && <GroupEditDialog edit="order" groups={asArray(groups).map(group => group.name)} busy={busy} profileId={profileId}
      run={run} refresh={refresh} close={() => setOrder(false)} />}
  </>;
}
