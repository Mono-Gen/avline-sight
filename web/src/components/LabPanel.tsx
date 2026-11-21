import { useEffect, useState } from 'react';
import { api, json } from '../api';
import { asArray, labSwitchIndex } from '../format';

export function LabPanel({ run, busy, switches }: {
  run: (action: () => Promise<void>) => void;
  busy: boolean;
  switches: {
    id: number;
    name: string;
  }[];
}) {
  const [scenarios, setScenarios] = useState<{
    id: string;
    title: string;
  }[]>([]);
  const [selected, setSelected] = useState('');
  useEffect(() => {
    api<typeof scenarios>('/api/sim/scenarios').then(value => setScenarios(asArray(value))).catch(() => setScenarios([]));
  }, []);
  // DBのIDや表示順はラボ番号と一致しないため、AV-SW-NNのNNから対象を求める。
  const options = asArray(switches).flatMap(item => {
    const index = labSwitchIndex(item.name);
    return index !== null ? [{
      ...item,
      index
    }] : [];
  });
  const current = asArray(options).find(item => String(item.id) === selected) || options[0];
  return (
    <section className="panel space-y-2">
      <h2>仮想ラボ</h2>
      <label>スイッチ <select
        value={current?.id || ''}
        disabled={busy || !options.length}
        onChange={event => setSelected(event.target.value)}
      >
        {!options.length && <option value="">対象スイッチなし</option>}
        {asArray(options).map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
      </select></label>
      <div className="flex flex-wrap gap-1">
        {asArray(scenarios).map(scenario => (
          <button
            key={scenario.id}
            disabled={busy || !current}
            onClick={() => run(async () => {
              await api('/api/sim/fault', json('POST', {
                switch_index: current.index,
                scenario: scenario.id
              }));
            })}
          >{scenario.title}</button>
        ))}
        <button
          disabled={busy}
          onClick={() => {
            if (confirm('ラボ状態をリセットしますか？')) run(async () => {
              await api('/api/sim/reset', json('POST'));
            });
          }}
        >リセット</button>
      </div>
    </section>
  );
}
