import { useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { deviceUrl } from '../deviceCsv';
import { asArray } from '../format';
import { t, useLang } from '../i18n';
import type { ReplaySession, SwitchGroup } from '../types';
import { GroupEditDialog } from './GroupEditDialog';
import { ImportReplay } from './ImportReplay';
import { ShowReportButton } from './ShowReportButton';

type Run = (action: () => Promise<void>) => void;

// Language, theme and desktop notifications: kept in this browser only and applied at once.
export function BrowserDisplaySettings({ theme, setTheme, notifications, setNotifications, busy, run }: {
  theme: string; setTheme: (value: string) => void;
  notifications: boolean; setNotifications: (value: boolean) => void;
  busy: boolean; run: Run;
}) {
  const [language, setLanguage] = useLang();
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
  const scope = <span className="settings-scope settings-scope-browser">{t('settingsPage.scopeBrowser')}</span>;
  return <>
    <div className="settings-row"><span className="settings-label">{t('rack.language')} {scope}</span>
      <select aria-label={t('rack.language')} value={language} onChange={event => setLanguage(event.target.value === 'en' ? 'en' : 'ja')}>
        <option value="ja">{t('settingsPage.langJa')}</option><option value="en">English</option>
      </select></div>
    <div className="settings-row"><span className="settings-label">{t('settingsPage.theme')} {scope}</span>
      <select aria-label={t('settingsPage.theme')} value={theme === 'dark' ? 'dark' : 'light'} onChange={event => setTheme(event.target.value)}>
        <option value="dark">{t('settingsPage.dark')}</option><option value="light">{t('settingsPage.light')}</option>
      </select></div>
    <div className="settings-row"><span className="settings-label">{t('settingsPage.notifications')} {scope}</span>
      <label><input type="checkbox" checked={notifications} disabled={busy} onChange={toggleNotifications} /> {t('cleanup.notifications')}</label></div>
    {notice && <p role="alert">⚠ {notice}</p>}
  </>;
}

// Files in and out of this project: settings file, logs and the show report, device CSV, offline replay.
export function DataTransfers({ busy, locked, replay, groups, profileId, run, setDialog, setReplay }: {
  busy: boolean; locked: boolean; replay: boolean; groups: SwitchGroup[]; profileId?: number;
  run: Run; setDialog: (value: string) => void; setReplay: Dispatch<SetStateAction<ReplaySession | null>>;
}) {
  const [scope, setScope] = useState('*');
  const csv = deviceUrl({ scope: 'all', q: '', vlans: [], group: scope === '*' ? undefined : scope.slice(6) }, undefined, true);
  return <div className="settings-tiles">
    {!replay && profileId !== undefined && <div className="settings-tile">
      <h3>{t('settingsPage.configFile')}</h3><p>{t('settingsPage.configFileHelp')}</p>
      <a href={`/api/profiles/${profileId}/export`} download="avline-sight.config.json">{t('configFile.export')}</a>
      <button disabled={busy || locked} onClick={() => setDialog('config-import')}>{t('configFile.import')}</button>
    </div>}
    <div className="settings-tile">
      <h3>{t('settingsPage.logs')}</h3><p>{t('settingsPage.logsHelp')}</p>
      <button disabled={busy || locked} onClick={() => setDialog('export')}>{t('rack.export')}</button>
      {!replay && <ShowReportButton run={run} busy={busy} />}
    </div>
    {!replay && <div className="settings-tile">
      <h3>{t('settingsPage.devices')}</h3><p>{t('settingsPage.devicesHelp')}</p>
      <label>{t('cleanup.csvGroup')} <select value={scope} onChange={event => setScope(event.target.value)}>
        <option value="*">{t('rack.all')}</option>
        {asArray(groups).map(group => <option key={group.name} value={`group:${group.name}`}>{group.name || t('rack.ungrouped')}</option>)}
      </select></label>
      <a href={csv} download>{t('rack.csv')}</a>
    </div>}
    <div className="settings-tile">
      <h3>{t('settingsPage.replay')}</h3><p>{t('settingsPage.replayHelp')}</p>
      <ImportReplay run={run} disabled={busy || locked} onImport={(id, timeline) => setReplay({ id, timeline, states: [] })} />
    </div>
  </div>;
}

// Order of the switch groups in the port monitor.
export function GroupOrder({ busy, locked, groups, profileId, run, refresh }: {
  busy: boolean; locked: boolean; groups: SwitchGroup[]; profileId?: number; run: Run; refresh: () => Promise<unknown>;
}) {
  const [order, setOrder] = useState(false);
  return <>
    <button disabled={busy || locked} onClick={() => setOrder(true)}>{t('operations.order')}</button>
    {order && !locked && <GroupEditDialog edit="order" groups={asArray(groups).map(group => group.name)} busy={busy} profileId={profileId}
      run={run} refresh={refresh} close={() => setOrder(false)} />}
  </>;
}
