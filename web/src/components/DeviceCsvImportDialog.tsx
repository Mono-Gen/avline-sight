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
  if (row.action === 'update') return `更新（${row.old_label || '（名前なし）'} → ${row.label}）`;
  return row.action === 'add' ? '追加' : '変更なし';
}

function PreviewTables({ response }: { response: DeviceAliasImportResponse }) {
  const errors = asArray(response.errors);
  const rows = asArray(response.rows);
  // 行数の予算をエラーに先に割り当て、修正すべき行を見落とさないようにする。
  const shownErrors = errors.slice(0, DISPLAY_LIMIT);
  const shownRows = rows.slice(0, DISPLAY_LIMIT - shownErrors.length);
  return (
    <div className="space-y-2">
      <p>{importSummary(response)}</p>
      {errors.length > 0 && (
        <div className="overflow-auto">
          <h3 className="font-bold">⚠ エラー {errors.length}件</h3>
          <table className="w-full text-sm">
            <thead><tr><th scope="col">行</th><th scope="col">理由</th></tr></thead>
            <tbody>
              {shownErrors.map((error, index) => <tr key={index}><td>{error.line}</td><td>{error.reason}</td></tr>)}
            </tbody>
          </table>
        </div>
      )}
      <div className="overflow-auto">
        <h3 className="font-bold">反映対象 {rows.length}件</h3>
        <table className="w-full text-sm">
          <thead>
            <tr>
              {['行', 'MAC', '名称', 'メモ', '処理'].map(label => <th key={label} scope="col">{label}</th>)}
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
      {errors.length + rows.length > DISPLAY_LIMIT && <p>エラー行と反映対象を合わせて先頭{DISPLAY_LIMIT}件まで表示しています。</p>}
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
      // BOMの3バイトは送信するUTF-8本文の上限に含まれない。
      if (file.size > MAX_CSV_BYTES + 3) {
        throw new Error('機器名CSVを読み込めません（上限は2MBです）。ファイルを分けて読み込んでください。');
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
    if (mode === 'replace' && deleted > 0 && !window.confirm(`登録済みの機器名を${deleted}件削除します。確定しますか？`)) return;
    run(async () => {
      committing.current = true;
      setError('');
      try {
        const result = await api<DeviceAliasImportResponse>(IMPORT_URL, json('POST', { csv: current.csv, mode, commit: true }));
        if (!result.committed) throw new Error('サーバーが反映完了を返しませんでした');
        onImported(result);
      } catch (reason) {
        setError(message(reason));
      } finally {
        committing.current = false;
      }
    });
  };

  return (
    <Dialog title="機器名CSV読込" close={() => { if (!committing.current) close(); }}>
      <label className="block">CSVファイル <input
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
        <legend>読込方法</legend>
        <label><input type="radio" name="device-import-mode" checked={mode === 'merge'} onChange={() => {
          setPreview(undefined);
          setMode('merge');
        }} /> 追加・上書き</label>
        <label><input type="radio" name="device-import-mode" checked={mode === 'replace'} onChange={() => {
          setPreview(undefined);
          setMode('replace');
        }} /> 全部入れ替え</label>
      </fieldset>
      {mode === 'replace' && <p className="text-amber-800 dark:text-amber-300">⚠ CSVに記載されていないMACの登録名は削除されます。</p>}
      {pending && <p role="status">プレビューを取得中…</p>}
      {committing.current && <p role="status">機器名を反映中…</p>}
      {error && <p role="alert" className="text-red-700 dark:text-red-300">⚠ {error}</p>}
      {current && <PreviewTables response={current.response} />}
      <button disabled={!ready || busy || pending} onClick={commit}>確定</button>
    </Dialog>
  );
}
