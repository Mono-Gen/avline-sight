import { useEffect, useRef, useState } from 'react';
import type { DragEvent } from 'react';
import { api, message } from '../api';
import { t } from '../i18n';
import type { TranslationKey } from '../i18n';
import { Dialog } from './Dialog';

const maxConfigBytes = 4 * 1024 * 1024;
const importPath = '/api/profiles/import';
const fieldLabels: Record<string, TranslationKey> = {
  name: 'project.name',
  switches: 'configFile.switches',
  group_order: 'operations.order',
  trap_addr: 'configFile.trap',
  ports: 'configFile.ports',
  aliases: 'configFile.aliases',
  watchlist: 'configFile.watchlist',
  topology_layout: 'configFile.topology',
  'settings.setup_ping_interval_sec': 'cleanup.setupPing',
  'settings.rehearsal_ping_interval_sec': 'cleanup.rehearsalPing',
  'settings.thresholds': 'ui.m180',
  'settings.expert_display_default': 'configFile.expertDefault',
  'settings.views': 'ui.m182',
};

export interface ConfigPreview {
  profile_id: number;
  revision: string;
  switch_count: number;
  group_count: number;
  changes: { field: string; before: unknown; after: unknown }[];
}

type ImportState =
  | { phase: 'idle' }
  | { phase: 'previewing' }
  | { phase: 'ready' | 'applying'; file: File; preview: ConfigPreview }
  | { phase: 'error'; reason: string };

export function ConfigFilePicker({ disabled, onFiles }: {
  disabled: boolean;
  onFiles: (files: FileList) => void;
}) {
  const [dragging, setDragging] = useState(false);
  const dragOver = (event: DragEvent<HTMLFieldSetElement>) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = disabled ? 'none' : 'copy';
    if (!disabled) setDragging(true);
  };
  return <fieldset className={dragging && !disabled ? 'rounded border-2 border-dashed border-blue-500 p-4' : 'rounded border border-dashed p-4'}
    disabled={disabled} onDragOver={dragOver} onDragLeave={() => setDragging(false)}
    onDrop={event => {
      event.preventDefault();
      setDragging(false);
      if (!disabled) onFiles(event.dataTransfer.files);
    }}>
    <label className="block">
      {t('configFile.choose')}
      <input className="block max-w-full" type="file" accept=".json,application/json"
        onChange={event => {
          const files = event.currentTarget.files;
          if (!disabled && files?.length) onFiles(files);
          event.currentTarget.value = '';
        }} />
    </label>
    <p>{t('configFile.drop')}</p>
  </fieldset>;
}

function diffValue(value: unknown) {
  if (typeof value === 'boolean') return t(value ? 'configFile.on' : 'configFile.off');
  if (value == null || value === '') return '—';
  return typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value);
}

export function ConfigImportPreview({ preview }: { preview: ConfigPreview }) {
  return <section className="space-y-2">
    <p role="status">{t('configFile.summary', {
      switches: preview.switch_count, groups: preview.group_count, changes: preview.changes.length,
    })}</p>
    <p>{t('configFile.applyNote')}</p>
    {preview.switch_count > 0 && <p>{t('configFile.credentialsNote')}</p>}
    {preview.changes.length > 0 ? <>
      <h3 className="font-bold">{t('configFile.diff')}</h3>
      <dl className="space-y-3">
        {preview.changes.map(({ field, before, after }) => <div key={field}>
          <dt className="font-bold">{fieldLabels[field] ? t(fieldLabels[field]) : field}</dt>
          <dd className="grid gap-2 sm:grid-cols-2">
            <div className="min-w-0">
              <strong>{t('configFile.before')}</strong>
              <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all">{diffValue(before)}</pre>
            </div>
            <div className="min-w-0">
              <strong>{t('configFile.after')}</strong>
              <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all">{diffValue(after)}</pre>
            </div>
          </dd>
        </div>)}
      </dl>
    </> : <p>{t('configFile.same')}</p>}
  </section>;
}

export function ConfigImportDialog({ locked, close, onApplied }: {
  locked: boolean;
  close: () => void;
  onApplied: () => Promise<void>;
}) {
  const [state, setState] = useState<ImportState>({ phase: 'idle' });
  const action = useRef<AbortController>();
  useEffect(() => () => action.current?.abort(), []);
  const working = state.phase === 'previewing' || state.phase === 'applying';
  const load = async (files: FileList) => {
    if (locked || action.current) return;
    const file = files[0];
    if (files.length !== 1 || !file) {
      setState({ phase: 'error', reason: t('configFile.single') });
      return;
    }
    if (file.size > maxConfigBytes) {
      setState({ phase: 'error', reason: t('configFile.size') });
      return;
    }
    const controller = new AbortController();
    action.current = controller;
    setState({ phase: 'previewing' });
    try {
      const preview = await api<ConfigPreview>(`${importPath}/preview`, {
        method: 'POST', body: file, signal: controller.signal,
      });
      if (!controller.signal.aborted) setState({ phase: 'ready', file, preview });
    } catch (error) {
      if (!controller.signal.aborted) setState({ phase: 'error', reason: message(error) });
    } finally {
      action.current = undefined;
    }
  };
  const apply = async () => {
    if (locked || action.current || state.phase !== 'ready') return;
    const { file, preview } = state;
    const controller = new AbortController();
    action.current = controller;
    setState({ ...state, phase: 'applying' });
    try {
      await api(importPath, {
        method: 'POST', body: file, signal: controller.signal,
        headers: {
          'X-AVLine-Profile': String(preview.profile_id),
          'X-AVLine-Config-Revision': preview.revision,
        },
      });
      await onApplied();
    } catch (error) {
      if (!controller.signal.aborted) setState({ phase: 'error', reason: message(error) });
    } finally {
      action.current = undefined;
    }
  };
  const cancel = () => { if (state.phase !== 'applying') close(); };
  return <Dialog title={t('configFile.import')} close={cancel} closeDisabled={state.phase === 'applying'}>
    {locked && <p role="status">{t('configFile.locked')}</p>}
    <ConfigFilePicker disabled={locked || working} onFiles={load} />
    {working && <p role="status">{t(state.phase === 'applying' ? 'configFile.applying' : 'configFile.previewing')}</p>}
    {state.phase === 'error' && <p role="alert">⚠ {state.reason}</p>}
    {(state.phase === 'ready' || state.phase === 'applying') && <ConfigImportPreview preview={state.preview} />}
    <div className="flex gap-2">
      <button className="button-primary" disabled={locked || state.phase !== 'ready'} onClick={apply}>{t('configFile.apply')}</button>
      <button disabled={state.phase === 'applying'} onClick={cancel}>{t('checklist.cancel')}</button>
    </div>
  </Dialog>;
}
