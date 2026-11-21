import { asArray } from '../format';
import type { Port } from '../types';

export function portHasAlert(port: Port): boolean {
  return [port.err_delta, port.discard_delta, port.fcs_delta, port.cos_drop_delta, port.err_since_reset]
    .some(value => value > 0);
}

export function poeHasFault(port: Port): boolean {
  const fault = port.poe?.fault?.trim().toLowerCase();
  return Boolean(fault && fault !== 'none');
}

export function FrontPanel({ ports, selected, onSelect }: {
  ports: Port[];
  selected: number;
  onSelect: (port: Port) => void;
}) {
  const list = asArray(ports);
  // VLAN絞り込みで配列位置が変わっても、物理ポートの奇数・偶数段を保つ。
  const isOdd = (port: Port, index: number) => {
    const number = port.name.match(/(\d+)\D*$/);
    return number ? Number(number[1]) % 2 === 1 : index % 2 === 0;
  };
  const rows = [
    asArray(list).filter((port, index) => isOdd(port, index)),
    asArray(list).filter((port, index) => !isOdd(port, index)),
  ];

  return (
    <div className="overflow-x-auto rounded bg-slate-700 p-3">
      {asArray(rows).map((row, index) => (
        <div key={index} className="mb-1 flex w-max gap-1">
          {asArray(row).map(port => {
            const fault = poeHasFault(port);
            const alert = portHasAlert(port);
            const color = alert || fault
              ? '!bg-red-200'
              : port.oper === 'up' ? '!bg-green-200' : '!bg-slate-300';
            return (
              <button
                key={port.if_index}
                title={`${port.name} ${port.type}`}
                aria-pressed={selected === port.if_index}
                className={`${color} ${port.excluded ? 'excluded' : ''} min-w-16`}
                onClick={() => onSelect(port)}
              >
                {port.type === 'sfp' ? '◈' : port.type === 'lag' ? '≡' : '▣'} {port.name}
                <br />
                {port.excluded ? '除外' : port.oper === 'up' ? '接続中' : '未接続'}
                {alert && <span title="エラー・破棄カウンター増加"> ⚠ 異常</span>}
                {fault
                  ? <span className="text-red-800" title={port.poe?.fault}> ⛔ PoE障害</span>
                  : (port.poe?.power_w || 0) > 0 && <span title="PoE給電中"> ⚡</span>}
              </button>
            );
          })}
        </div>
      ))}
    </div>
  );
}
