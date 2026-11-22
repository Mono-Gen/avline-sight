import { useState } from 'react';
import { api } from '../api';
import { groupRequest } from '../groupRequest';
import { t } from '../i18n';
import { Dialog } from './Dialog';

export type GroupEdit = { name: string } | 'order';

const reserved = (value: string) => ['*', '.', '..'].includes(value);

export function GroupEditDialog({ edit, groups, busy, close, run, refresh, profileId }: {
  edit: GroupEdit; groups: string[]; busy: boolean; close: () => void;
  profileId?: number;
  run: (action: () => Promise<void>) => void; refresh: () => Promise<unknown>;
}) {
  const [names, setNames] = useState(groups.filter(item => item && !reserved(item)));
  const [name, setName] = useState(edit === 'order' ? '' : edit.name);
  const invalid = !name.trim() || reserved(name.trim());
  const move = (index: number, delta: number) => {
    const target = index + delta;
    if (target < 0 || target >= names.length) return;
    const next = [...names];
    [next[index], next[target]] = [next[target], next[index]];
    setNames(next);
  };
  const save = () => {
    const trimmed = name.trim();
    if (edit !== 'order') {
      if (invalid || trimmed === edit.name) return;
      const from = edit.name || t('rack.ungrouped');
      if (groups.includes(trimmed) && !window.confirm(t('operations.mergeConfirm', { from, to: trimmed }))) return;
    }
    run(async () => {
      if (edit === 'order') {
        await api('/api/groups/order', groupRequest(profileId, 'PUT', { names }));
      } else {
        const path = ['.', '..'].includes(edit.name) ? `?legacy_name=${encodeURIComponent(edit.name)}` : encodeURIComponent(edit.name);
        await api(`/api/groups/${path}`, groupRequest(profileId, 'PUT', { new_name: trimmed }));
      }
      await refresh();
      close();
    });
  };
  return <Dialog title={t(edit === 'order' ? 'operations.order' : 'operations.rename')} close={close}>
    <fieldset className="space-y-2" disabled={busy}>
      {edit === 'order' ? <ol className="space-y-1">
        {names.map((item, index) => <li className="flex items-center gap-2" key={item}>
          <span className="flex-1">{item}</span>
          <button disabled={index === 0} onClick={() => move(index, -1)} title={t('operations.up')}>↑</button>
          <button disabled={index === names.length - 1} onClick={() => move(index, 1)} title={t('operations.down')}>↓</button>
        </li>)}
        {groups.includes('') && <li>{t('rack.ungrouped')}</li>}
      </ol> : <label>{t('operations.newGroup')} <input value={name} onChange={event => setName(event.target.value)} /></label>}
      <button className="block" disabled={edit !== 'order' && (invalid || edit.name === name.trim())} onClick={save}>{t('operations.save')}</button>
    </fieldset>
  </Dialog>;
}
