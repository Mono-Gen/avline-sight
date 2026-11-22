import { useState } from 'react';
import { download, message, responseBlob } from '../api';
import { t } from '../i18n';
import { parseJstDateTime } from '../pcTime';

export function showReportURL(from = '', to = '') {
  const path = '/api/export/show-report.csv';
  if (!from && !to) return path;
  const start = parseJstDateTime(from);
  const end = parseJstDateTime(to);
  if (!from || !to || !(start < end)) throw new Error(t('checklist.reportRangeError'));
  return `${path}?${new URLSearchParams({ from: start.toISOString(), to: end.toISOString() })}`;
}

export function ShowReportButton({ run, busy, from, to }: {
  run: (action: () => Promise<void>) => void;
  busy: boolean;
  from?: string;
  to?: string;
}) {
  const [error, setError] = useState('');
  return <><button type="button" disabled={busy}
    title={t(from || to ? 'checklist.reportSelectedRange' : 'checklist.reportLatest')}
    onClick={() => run(async () => {
      setError('');
      try {
        const csv = await responseBlob(showReportURL(from, to));
        download(csv, 'avline-sight-show-report.csv');
      } catch (cause) {
        setError(message(cause));
      }
    })}>{t('checklist.report')}</button>
    {error && <p role="alert">⚠ {error}</p>}
  </>;
}

