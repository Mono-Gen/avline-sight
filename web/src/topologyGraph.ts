import type cytoscape from 'cytoscape';
import { asArray, decimal } from './format';
import { t } from './i18n';
import { linkAppearance, topologyNodeDetails, topologyQueriers } from './topologyModel';
import type { buildTopology, topologyHighlights } from './topologyModel';
import type { TopologySelection } from './topologyModel';
import type { Multicast, TopologyNode } from './types';
import { alertStates } from './wallAlerts';

const linkHighlight = (color: string, padding: number): cytoscape.Css.Edge & {
  'underlay-color': string;
  'underlay-opacity': number;
  'underlay-padding': number;
} => ({
  'underlay-color': color, 'underlay-opacity': 1, 'underlay-padding': padding,
});

export const topologyStyles: cytoscape.Stylesheet[] = [
  {
    selector: 'node',
    style: {
      label: 'data(statusLabel)',
      'background-color': 'data(statusColor)',
      color: '#f7f8fa',
      'text-wrap': 'wrap',
      'text-outline-color': '#111419',
      'text-outline-width': 2,
      'text-valign': 'bottom'
    }
  },
  {
    selector: 'node[kind = "external"], node[kind = "device"]',
    style: { shape: 'diamond' }
  },
  // Device labels are kept short so neighbours do not overlap; the full name is in the selection list.
  {
    selector: 'node[kind = "device"]',
    style: { width: 14, height: 14, 'font-size': 9, 'text-wrap': 'ellipsis', 'text-max-width': '84px' }
  },
  { selector: 'node[kind = "switch"]', style: { 'font-size': 12, 'font-weight': 'bold' } },
  { selector: 'node[statusTone = "warning"]', style: { 'border-color': '#fde68a', 'border-width': 3 } },
  {
    selector: 'edge',
    style: {
      width: 'data(lineWidth)',
      'line-color': 'data(lineColor)',
      label: 'data(edgeLabel)',
      'font-size': 10,
      'text-wrap': 'wrap',
      'text-outline-color': '#111419',
      'text-outline-width': 2,
      color: '#f7f8fa'
    }
  },
  {
    selector: 'node[?highlight]',
    style: { 'border-color': '#7dd3fc', 'border-width': 3 }
  },
  { selector: 'edge[?highlight]', style: linkHighlight('#f59e0b', 3) },
  { selector: 'edge[?path], edge[?selected]', style: linkHighlight('#c4b5fd', 4) },
  { selector: 'edge[?down]', style: { 'line-style': 'dashed' } },
  { selector: 'node[?path], node[?selected]', style: { 'border-color': '#c4b5fd', 'border-width': 4 } },
  { selector: 'node[?impacted]', style: { 'background-color': '#ff4d5e', 'border-color': '#fecaca', 'border-width': 3 } },
  {
    selector: 'node[kind = "device-count"]',
    style: {
      shape: 'round-rectangle', width: 'data(badgeWidth)', height: 22, 'font-size': 11, // 文字数に合わせた幅（下の countBadge）
      'background-color': '#111419', 'border-color': '#c4cad4', 'border-width': 1,
      'text-valign': 'center', 'text-outline-width': 0, 'z-index': 10
    }
  },
];

export function countPosition(position: cytoscape.Position): cytoscape.Position {
  return { x: position.x + 27, y: position.y - 17 };
}

export const countId = (node: string) => `count:${node}`;

// 接続台数のバッジ。「10台」「120 dev.」など文字数に合わせて幅を決める（11px 文字の目安で1文字7px＋余白）
function countBadge(count: number) {
  const statusLabel = t('topology.countBadge', { count });
  return { statusLabel, badgeWidth: Math.max(28, 12 + [...statusLabel].length * 7) };
}

// リンクの線に流れている VLAN を添える。5本までは番号、それより多ければ本数（全部はリンクを選ぶと見える）。
// 両端で食い違うときは ⚠ を付ける（色だけに頼らない）。
const VLAN_LIST_MAX = 5;
export function vlanLine(edge: { vlans?: number[]; vlan_mismatch?: boolean }) {
  const vlans = edge.vlans ?? [];
  // 共通の VLAN が1つも無い食い違いが最も重いので、一覧が空でも ⚠ は出す
  if (!vlans.length) return edge.vlan_mismatch ? `
⚠ ${t('topology.vlanNone')}` : '';
  const body = vlans.length <= VLAN_LIST_MAX ? t('topology.vlanList', { list: vlans.join(',') }) : t('topology.vlanCount', { count: vlans.length });
  return `\n${edge.vlan_mismatch ? '⚠ ' : ''}${body}`;
}
export const deviceLabel = (row: { label: string; lldp_name: string; mac: string }) => row.label || row.lldp_name || row.mac;

function updateData(element: cytoscape.SingularElementReturnValue, data: Record<string, unknown>) {
  const current = element.data();
  const changes = Object.entries(data).filter(([key, value]) => !Object.is(current[key], value));
  if (changes.length) element.data(Object.fromEntries(changes));
}

export function updateTopology(
  core: cytoscape.Core,
  model: ReturnType<typeof buildTopology>,
  multicast: Multicast | undefined,
  { group = '', vlan = 0, highlights, selection, initial = false, nodeDetails = topologyNodeDetails(model) }: {
    group?: string; vlan?: number; highlights?: ReturnType<typeof topologyHighlights>; selection?: TopologySelection;
    initial?: boolean; nodeDetails?: ReturnType<typeof topologyNodeDetails>;
  } = {},
) {
  const { edges, parent, devices, deviceRows } = model;
  const { impacted, path } = highlights ?? {};
  const queriers = topologyQueriers(multicast, vlan).switchIds;
  const nodes: TopologyNode[] = [...model.nodes, ...Array.from(deviceRows, ([id, row]) => ({
    id, kind: 'device', label: deviceLabel(row),
  }))];
  const groups = asArray(multicast?.groups).filter(item => item.group === group);
  const members = new Set(groups.map(item => item.switch_id));
  const carries = (node: string, port: string, vlan: number) => groups.some(item =>
    item.switch_id === model.byId.get(node)?.switch_id && item.vlan === vlan && asArray(item.ports).includes(port));
  const switchNodes = model.nodes.filter(node => node.kind === 'switch' && node.switch_id);
  const ids = new Set([...nodes.map(node => node.id), ...edges.map(edge => edge.id),
    ...switchNodes.map(node => countId(node.id)), ...Array.from(parent.keys(), id => `attachment:${id}`)]);
  core.batch(() => {
    core.elements().filter(element => !ids.has(element.id())).remove();
    nodes.forEach((node, index) => {
      const { monitorTarget, statusTone = 'unknown' } = nodeDetails.get(node.id) ?? {};
      const status = alertStates[statusTone];
      const isImpacted = impacted?.has(node.id) ?? false;
      const isQuerier = node.switch_id !== undefined && queriers.has(node.switch_id) && node.reachable !== false;
      const badge = isQuerier ? `\n${t('topology.querierBadge', { vlan })}` : '';
      const impactLabel = isImpacted ? `\n${t('topology.affected')}` : '';
      const data = {
        id: node.id,
        kind: node.kind,
        label: node.label,
        monitorSwitchId: monitorTarget?.switchId ?? 0,
        monitorPortName: monitorTarget?.portName ?? '',
        statusTone,
        statusLabel: `${status.symbol} ${node.label}${badge}${impactLabel}`,
        statusColor: status.color,
        highlight: node.switch_id !== undefined && members.has(node.switch_id),
        impacted: isImpacted,
        querier: isQuerier,
        path: path?.has(node.id) ?? false,
        selected: selection?.kind === 'node' && selection.id === node.id,
      };
      const existing = core.getElementById(node.id);
      if (existing.length) {
        updateData(existing, data);
      } else {
        core.add({
          group: 'nodes',
          data,
          position: Number.isFinite(node.x) && Number.isFinite(node.y)
            ? { x: node.x!, y: node.y! }
            : { x: 80 + (index % 5) * 160, y: 80 + Math.floor(index / 5) * 100 },
        });
      }
    });
    const upsertEdge = (data: cytoscape.EdgeDataDefinition) => {
      const existing = core.getElementById(data.id!);
      if (existing.length && existing.data('source') === data.source && existing.data('target') === data.target) {
        updateData(existing, data);
      } else {
        existing.remove();
        core.add({ group: 'edges', data });
      }
    };
    edges.forEach(edge => {
      const down = model.failedLinks.has(edge.id);
      const { width, color, speed, util } = linkAppearance(model.linkMetrics.get(edge.id), down);
      const metrics = t('topology.linkMetrics', {
        speed: speed === undefined ? '—' : `${speed} Mbps`,
        utilization: util === undefined ? '—' : `${decimal(util)}%`,
      });
      upsertEdge({
        id: edge.id, source: edge.source, target: edge.target, interSwitch: true,
        lineWidth: width, lineColor: color, down, path: path?.has(edge.id) ?? false,
        selected: selection?.kind === 'edge' && selection.id === edge.id,
        edgeLabel: `${edge.local_port} ↔ ${edge.remote_port}\n${down ? `${t('topology.linkDown')} · ` : ''}${metrics}${vlanLine(edge)}`,
        highlight: groups.some(item =>
          carries(edge.source, edge.local_port, item.vlan) && carries(edge.target, edge.remote_port, item.vlan)),
      });
    });
    for (const [device, source] of parent) {
      const id = `attachment:${device}`;
      upsertEdge({ id, source, target: device, edgeLabel: '', lineWidth: 1, lineColor: '#9aa3b2', path: path?.has(id) ?? false });
    }
  });
  if (initial && nodes.length) {
    const saved = model.nodes.some(node => Number.isFinite(node.x) && Number.isFinite(node.y));
    if (saved) core.elements().not('node[kind = "device-count"]').layout({ name: 'preset' }).run();
    else layoutBySwitch(core, switchNodes.map(node => node.id), parent);
  }
  core.batch(() => {
    for (const node of switchNodes) {
      const id = countId(node.id);
      const count = devices.get(node.id)?.length ?? 0;
      const position = countPosition(core.getElementById(node.id).position());
      const existing = core.getElementById(id);
      if (existing.length) {
        updateData(existing, countBadge(count));
        const current = existing.position();
        if (current.x !== position.x || current.y !== position.y) existing.position(position);
      } else {
        core.add({ group: 'nodes', data: { id, kind: 'device-count', switchNode: node.id, ...countBadge(count), statusColor: '#111419' },
          position, grabbable: false, selectable: false });
      }
    }
  });
}

// First layout without saved positions: switches in one row, each switch's devices in a block of columns below it.
export function layoutBySwitch(core: cytoscape.Core, switchIds: readonly string[], parent: ReadonlyMap<string, string>) {
  const COLS = 4;
  const DX = 96;
  const DY = 46;
  const children = new Map<string, string[]>(switchIds.map(id => [id, []]));
  for (const [device, owner] of parent) children.get(owner)?.push(device);
  let x = 0;
  core.batch(() => {
    for (const id of switchIds) {
      const devices = children.get(id) ?? [];
      const width = Math.max(1, Math.min(COLS, devices.length)) * DX;
      core.getElementById(id).position({ x: x + width / 2, y: 0 });
      devices.forEach((device, index) => core.getElementById(device).position({
        x: x + (index % COLS) * DX + DX / 2,
        y: 110 + Math.floor(index / COLS) * DY,
      }));
      x += width + 80;
    }
    // Anything not placed above (external nodes) goes on a row of its own above the switches.
    const placed = new Set([...switchIds, ...parent.keys()]);
    core.nodes().not('node[kind = "device-count"]').filter(node => !placed.has(node.id()))
      .forEach((node, index) => { node.position({ x: index * DX, y: -110 }); });
  });
  core.fit(undefined, 30);
}
