import { t } from '../i18n';
import { useState } from 'react';
import type { ReactNode } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import type { Settings } from '../types';
import { Dialog } from './Dialog';

type ViewDraft = {
  id: string;
  name: string;
  vlans: string;
  tags: string;
  categories: string[];
};
const thresholdLabels: Record<string, string> = {
  get cpu_pct() { return t('ui.m066'); },
  get mem_free_pct() { return t('ui.m050'); },
  get temp_c() { return t('ui.m056'); },
  get poe_pct() { return t('ui.m173'); },
  get util_pct() { return t('ui.m174'); },
  get sfp_rx_dbm() { return t('ui.m175'); },
  get sfp_drop_db() { return t('ui.m176'); },
  get err_delta() { return t('ui.m177'); },
};
const commaList = (value: string) => asArray(value.split(',')).map(item => item.trim()).filter(Boolean);

export function SettingsDialog({ value, close, run, busy, webAddr, locked = false, tools }: {
  value: Settings;
  locked?: boolean;
  tools?: ReactNode;
  webAddr?: string;
  close: () => void;
  run: (action: () => Promise<void>) => void;
  busy: boolean;
}) {
  const [thresholds, setThresholds] = useState({ ...value.thresholds });
  const [setupPing, setSetupPing] = useState(value.setup_ping_interval_sec ?? 60);
  const [rehearsalPing, setRehearsalPing] = useState(value.rehearsal_ping_interval_sec ?? 60);
  const [views, setViews] = useState<ViewDraft[]>(() => asArray(value.views).map(view => ({
    id: view.id,
    name: view.name,
    vlans: asArray(view.vlans).join(', '),
    tags: asArray(view.tags).join(', '),
    categories: asArray(view.categories),
  })));
  const [validation, setValidation] = useState('');
  const changeView = (index: number, key: 'name' | 'vlans' | 'tags', text: string) => {
    setViews(old => asArray(old).map((view, position) => position === index ? {
      ...view,
      [key]: text
    } : view));
  };
  const save = () => {
    const parsed = asArray(views).map(view => ({
      ...view,
      tags: commaList(view.tags),
      vlans: asArray(commaList(view.vlans)).map(Number),
    }));
    if (asArray(Object.values(thresholds)).some(number => !Number.isFinite(number)) ||
      asArray(parsed).some(view => !view.name.trim() || asArray(view.vlans).some(vlan => !Number.isInteger(vlan) || vlan < 1 || vlan > 4094))) {
      setValidation(t('ui.m178'));
      return;
    }
    if ([setupPing, rehearsalPing].some(seconds => !Number.isInteger(seconds) || seconds < 30 || seconds > 3600)) {
      setValidation(t('cleanup.pingInvalid')); return;
    }
    setValidation('');
    run(async () => {
      await api('/api/settings', json('PUT', {
        thresholds,
        setup_ping_interval_sec: setupPing,
        rehearsal_ping_interval_sec: rehearsalPing,
        views: parsed
      }));
      close();
    });
  };

  return (
    <Dialog title={t('ui.m179')} close={close}>
      {webAddr?.startsWith('127.0.0.1:') && <p role="status">ⓘ {t('rack.loopback')}</p>}
      {tools}
      <fieldset disabled={busy || locked} className="space-y-4">
        <legend className="font-bold">{t('ui.m180')}</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          {asArray(Object.entries(thresholds)).map(([key, number]) => (
            <label key={key} className="flex justify-between gap-2">
              {thresholdLabels[key] || key}
              <input
                type="number"
                step="any"
                className="w-28"
                value={Number.isFinite(number) ? number : ''}
                onChange={event => setThresholds({
                  ...thresholds,
                  [key]: event.target.value === '' ? NaN : Number(event.target.value)
                })}
              />
            </label>
          ))}
        </div>
        <label className="block">{t('cleanup.setupPing')} <input type="number" min="30" max="3600" step="1" value={Number.isFinite(setupPing) ? setupPing : ''} onChange={event => setSetupPing(event.target.value === '' ? NaN : Number(event.target.value))} /></label>
        <label className="block">{t('cleanup.rehearsalPing')} <input type="number" min="30" max="3600" step="1" value={Number.isFinite(rehearsalPing) ? rehearsalPing : ''} onChange={event => setRehearsalPing(event.target.value === '' ? NaN : Number(event.target.value))} /></label>
        <p>{t('cleanup.pingBounds')}</p>
        <h3 className="font-bold">{t('ui.m182')}</h3>
        {asArray(views).map((view, index) => (
          <div key={view.id} className="grid gap-2 rounded border p-2">
            <label>{t('ui.m017')}<input value={view.name} onChange={event => changeView(index, 'name', event.target.value)} /></label>
            <label>{t('ui.m183')}<input value={view.vlans} onChange={event => changeView(index, 'vlans', event.target.value)} /></label>
            <label>{t('ui.m184')}<input value={view.tags} onChange={event => changeView(index, 'tags', event.target.value)} /></label>
            <button onClick={() => setViews(old => asArray(old).filter((_, position) => position !== index))}>{t('ui.m185')}</button>
          </div>
        ))}
        <button
          onClick={() => setViews(old => [
            ...asArray(old),
            {
              id: `view-${Date.now()}-${old.length}`,
              name: '',
              vlans: '',
              tags: '',
              categories: [],
            }
          ])}
        >{t('ui.m186')}</button>
        {validation && <p role="alert">⚠ {validation}</p>}
        <button onClick={save}>{t('ui.m187')}</button>
      </fieldset>
    </Dialog>
  );
}
