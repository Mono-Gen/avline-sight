import { t } from '../i18n';
import { useEffect, useRef, useState } from 'react';
import { api, json, message } from '../api';
import { decodeDeviceCsv, importSummary } from '../deviceCsv';
import { asArray } from '../format';
import type { DeviceAliasImportMode, DeviceAliasImportResponse, DeviceAliasImportRow } from '../types';
import { Dialog } from './Dialog';

const MAX_CSV_BYTES = 2 * 1024 * 1024;
const DISPLAY_LIMIT = 500;
const IMPORT_URL = '/api/device-aliases/import';

interface Preview {
  file: File;
  mode: DeviceAliasImportMode;
  csv: string;
  encoding: string;
  response: DeviceAliasImportResponse;
}

function actionLabel(row: DeviceAliasImportRow): string {
  if (row.action === 'update') return t('ui.m092', { p0: row.old_label || t('ui.m091'), p1: row.label });
  return row.action === 'add' ? t('ui.m093') : t('ui.m094');
}

function PreviewTables({ response }: { response: DeviceAliasImportResponse }) {
  const errors = asArray(response.errors);
  const rows = asArray(response.rows);
  const shownErrors = errors.slice(0, DISPLAY_LIMIT);
  const shownRows = rows.slice(0, DISPLAY_LIMIT - shownErrors.length);
  return (
    <div className="space-y-2">
      <p>{importSummary(response)}</p>
      {errors.length > 0 && (
        <div className="overflow-auto">
          <h3 className="font-bold">{t('ui.m095')}{errors.length}{t('ui.m096')}</h3>
          <table className="w-full text-sm">
            <thead><tr><th scope="col">{t('ui.m097')}</th><th scope="col">{t('ui.m098')}</th></tr></thead>
            <tbody>
              {shownErrors.map((error, index) => <tr key={index}><td>{error.line}</td><td>{error.reason}</td></tr>)}
            </tbody>
          </table>
        </div>
      )}
      <div className="overflow-auto">
        <h3 className="font-bold">{t('ui.m099')}{rows.length}{t('ui.m096')}</h3>
        <table className="w-full text-sm">
          <thead>
            <tr>
              {[t('ui.m097'), 'MAC', t('ui.m100'), t('ui.m101'), t('ui.m102')].map(label => <th key={label} scope="col">{label}</th>)}
            </tr>
          </thead>
          <tbody>
            {shownRows.map(row => (
              <tr key={row.line}>
                <td>{row.line}</td><td>{row.mac}</td><td>{row.label}</td><td>{row.note}</td><td>{actionLabel(row)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {errors.length + rows.length > DISPLAY_LIMIT && <p>{t('ui.m103')}{DISPLAY_LIMIT}{t('ui.m104')}</p>}
    </div>
  );
}

export function DeviceCsvImportDialog({ close, onImported, run, busy }: {
  close: () => void;
  onImported: (response: DeviceAliasImportResponse) => void;
  run: (action: () => Promise<void>) => void;
  busy: boolean;
}) {
  const [file, setFile] = useState<File>();
  const [mode, setMode] = useState<DeviceAliasImportMode>('merge');
  const [preview, setPreview] = useState<Preview>();
  const [encoding, setEncoding] = useState('');
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);
  const committing = useRef(false);

  useEffect(() => {
    if (!file) return;
    const request = new AbortController();
    setPending(true);
    setError('');
    const loadPreview = async () => {
      if (file.size > MAX_CSV_BYTES + 3) {
        throw new Error(t('ui.m105'));
      }
      const decoded = decodeDeviceCsv(new Uint8Array(await file.arrayBuffer()));
      if (request.signal.aborted) return;
      setEncoding(decoded.encoding);
      const response = await api<DeviceAliasImportResponse>(IMPORT_URL, {
        ...json('POST', { csv: decoded.csv, mode, commit: false }),
        signal: request.signal,
      });
      if (!request.signal.aborted) setPreview({ file, mode, ...decoded, response });
    };
    void loadPreview().catch(reason => {
      if (!request.signal.aborted) setError(message(reason));
    }).finally(() => {
      if (!request.signal.aborted) setPending(false);
    });
    return () => request.abort();
  }, [file, mode]);

  const current = preview?.file === file && preview?.mode === mode ? preview : undefined;
  const ready = current && Array.isArray(current.response.rows) && current.response.rows.length > 0 &&
    Array.isArray(current.response.errors) && current.response.errors.length === 0;
  const commit = () => {
    if (!current || !ready || busy || pending || committing.current) return;
    const deleted = current.response.result.deleted;
    if (mode === 'replace' && deleted > 0 && !window.confirm(t('ui.m106', { p0: deleted }))) return;
    run(async () => {
      committing.current = true;
      setError('');
      try {
        const result = await api<DeviceAliasImportResponse>(IMPORT_URL, json('POST', { csv: current.csv, mode, commit: true }));
        if (!result.committed) throw new Error(t('ui.m107'));
        onImported(result);
      } catch (reason) {
        setError(message(reason));
      } finally {
        committing.current = false;
      }
    });
  };

  return (
    <Dialog title={t('ui.m108')} close={() => { if (!committing.current) close(); }}>
      <label className="block">{t('ui.m109')}<input
        type="file"
        accept=".csv,text/csv"
        disabled={busy}
        onChange={event => {
          const selected = event.target.files?.[0];
          event.target.value = '';
          if (!selected) return;
          setPreview(undefined);
          setEncoding('');
          setFile(selected);
        }}
      /></label>
      {file && <p>{file.name}{encoding && ` · ${encoding}`}</p>}
      <fieldset disabled={busy} className="flex flex-wrap gap-3">
        <legend>{t('ui.m110')}</legend>
        <label><input type="radio" name="device-import-mode" checked={mode === 'merge'} onChange={() => {
          setPreview(undefined);
          setMode('merge');
        }} /> {t('ui.m111')}</label>
        <label><input type="radio" name="device-import-mode" checked={mode === 'replace'} onChange={() => {
          setPreview(undefined);
          setMode('replace');
        }} /> {t('ui.m112')}</label>
      </fieldset>
      {mode === 'replace' && <p className="text-amber-800 dark:text-amber-300">{t('ui.m113')}</p>}
      {pending && <p role="status">{t('ui.m114')}</p>}
      {committing.current && <p role="status">{t('ui.m115')}</p>}
      {error && <p role="alert" className="text-red-700 dark:text-red-300">⚠ {error}</p>}
      {current && <PreviewTables response={current.response} />}
      <button disabled={!ready || busy || pending} onClick={commit}>{t('ui.m116')}</button>
    </Dialog>
  );
}
