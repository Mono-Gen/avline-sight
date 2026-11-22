import { t } from '../i18n';
import { api, upload } from '../api';
import type { Timeline } from '../types';
type Run = (f: () => Promise<void>) => void;
export function ImportReplay({ run, onImport, disabled }: {
  run: Run;
  onImport: (id: string, t: Timeline) => void;
  disabled: boolean;
}) {
  return <label className="cursor-pointer rounded border p-1">{t('ui.m140')}<input
      hidden
      type="file"
      accept=".avsight"
      disabled={disabled}
      onChange={e => {
        const file = e.target.files?.[0];
        e.target.value = '';
        if (file) run(async () => {
          const r = await api<{
            session_id: string;
          }>('/api/replay/import', upload(file));
          const t = await api<Timeline>(`/api/replay/${encodeURIComponent(r.session_id)}/timeline`);
          onImport(r.session_id, t);
        });
      }}
    />
  </label>;
}
