import { asArray } from './format';
import type { ModelDefinition, Port } from './types';

export interface PanelRow {
  label?: string;
  slots: (Port | null)[];
}

export function frontPanelRows(ports: Port[], definition?: ModelDefinition): PanelRow[] {
  const list = asArray(ports);
  const validDefinition = definition && Array.isArray(definition.rows) && definition.rows.length > 0 &&
    definition.rows.every(row => Array.isArray(row) && row.every(name => typeof name === 'string'));
  if (validDefinition) {
    const byName = new Map(list.map(port => [port.name, port]));
    const assigned = new Set<string>();
    const rows: PanelRow[] = definition.rows.map(row => ({
      slots: row.map(name => {
        if (name) assigned.add(name);
        // 未取得・非表示のポートでも物理配置が詰まらないよう、空き位置を残す。
        return name ? byName.get(name) || null : null;
      }),
    }));
    const extra = list.filter(port => !assigned.has(port.name));
    if (extra.length) rows.push({ label: 'その他', slots: extra });
    return rows;
  }
  const order = ['rj45', 'sfp', 'lag'];
  const rank = (port: Port) => {
    const index = order.indexOf(port.type);
    return index < 0 ? order.length : index;
  };
  const sorted = [...list].sort((left, right) => rank(left) - rank(right) ||
    left.name.localeCompare(right.name, undefined, { numeric: true }));
  const isOdd = (port: Port, index: number) => {
    const number = port.name.match(/(\d+)\D*$/);
    return number ? Number(number[1]) % 2 === 1 : index % 2 === 0;
  };
  return [
    { slots: sorted.filter((port, index) => isOdd(port, index)) },
    { slots: sorted.filter((port, index) => !isOdd(port, index)) },
  ];
}
