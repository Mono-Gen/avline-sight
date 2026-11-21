import { useState } from 'react';
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
  cpu_pct: 'CPU使用率 (%)',
  mem_free_pct: '空きメモリ (%)',
  temp_c: '温度 (℃)',
  poe_pct: 'PoE使用率 (%)',
  util_pct: '帯域使用率 (%)',
  sfp_rx_dbm: 'SFP受信光 (dBm)',
  sfp_drop_db: 'SFP光低下量 (dB)',
  err_delta: 'エラー増分',
};
const commaList = (value: string) => asArray(value.split(',')).map(item => item.trim()).filter(Boolean);

export function SettingsDialog({ value, close, run, busy }: {
  value: Settings;
  close: () => void;
  run: (action: () => Promise<void>) => void;
  busy: boolean;
}) {
  const [thresholds, setThresholds] = useState({ ...value.thresholds });
  const [sound, setSound] = useState(value.alert_sound);
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
      setValidation('設定を保存できません（名前・数値・VLAN番号が不正です）。VLANは1〜4094で入力してください。');
      return;
    }
    setValidation('');
    run(async () => {
      await api('/api/settings', json('PUT', {
        thresholds,
        alert_sound: sound,
        views: parsed
      }));
      close();
    });
  };

  return (
    <Dialog title="設定" close={close}>
      <fieldset disabled={busy} className="space-y-4">
        <legend className="font-bold">警告しきい値</legend>
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
        <label className="block"><input type="checkbox" checked={sound} onChange={event => setSound(event.target.checked)} /> 重大警告の通知音</label>
        <h3 className="font-bold">担当パート</h3>
        {asArray(views).map((view, index) => (
          <div key={view.id} className="grid gap-2 rounded border p-2">
            <label>名前 <input value={view.name} onChange={event => changeView(index, 'name', event.target.value)} /></label>
            <label>VLAN（カンマ区切り） <input value={view.vlans} onChange={event => changeView(index, 'vlans', event.target.value)} /></label>
            <label>タグ（カンマ区切り） <input value={view.tags} onChange={event => changeView(index, 'tags', event.target.value)} /></label>
            <button onClick={() => setViews(old => asArray(old).filter((_, position) => position !== index))}>パートを削除</button>
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
        >パートを追加</button>
        {validation && <p role="alert">⚠ {validation}</p>}
        <button onClick={save}>保存</button>
      </fieldset>
    </Dialog>
  );
}
