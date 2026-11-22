import { t } from '../i18n';
import { useEffect, useState } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import type { WatchItem } from '../types';
type Run = (f: () => Promise<void>) => void;
export function WatchBar({ items, onChange, run, disabled }: {
  items: WatchItem[];
  onChange: (v: WatchItem[]) => void;
  run: Run;
  disabled: boolean;
}) {
  const [ports, setPorts] = useState<{
    switch_id: number;
    port: {
      if_index: number;
      rx_bps: number;
      tx_bps: number;
    };
  }[]>([]);
  useEffect(() => {
    let active = true;
    const get = () => api<typeof ports>('/api/ports/search').then(v => {
      if (active) setPorts(v || []);
    }).catch(() => {
    });
    void get();
    const timer = setInterval(get, 5000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, []);
  return <div className="panel">{t('ui.m204')}{asArray(items).map((w, i) => {
      const p = asArray(ports).find(x => x.switch_id === w.switch_id && x.port.if_index === w.if_index)?.port;
      return <span key={i} className="mr-3">
        {w.label}
        :
        {p ? `${p.rx_bps} / ${p.tx_bps} bps` : t('ui.m147')}

        <button
          disabled={disabled}
          onClick={() => run(async () => {
            const next = asArray(items).filter((_, j) => i !== j);
            await api('/api/watchlist', json('PUT', next));
            onChange(next);
          })}
        >×
        </button>
      </span>;
    })}
  </div>;
}
