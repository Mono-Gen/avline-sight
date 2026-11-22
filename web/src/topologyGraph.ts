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
      color: '#f8fafc',
      'text-wrap': 'wrap',
      'text-outline-color': '#0f172a',
      'text-outline-width': 2,
      'text-valign': 'bottom'
    }
  },
  {
    selector: 'node[kind = "external"], node[kind = "device"]',
    style: { shape: 'diamond' }
  },
  { selector: 'node[statusTone = "warning"]', style: { 'border-color': '#fde68a', 'border-width': 3 } },
  {
    selector: 'edge',
    style: {
      width: 'data(lineWidth)',
      'line-color': 'data(lineColor)',
      label: 'data(edgeLabel)',
      'font-size': 10,
      'text-wrap': 'wrap',
      'text-outline-color': '#0f172a',
      'text-outline-width': 2,
      color: '#f8fafc'
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
  { selector: 'node[?impacted]', style: { 'background-color': '#f87171', 'border-color': '#fecaca', 'border-width': 3 } },
  {
    selector: 'node[kind = "device-count"]',
    style: {
      shape: 'round-rectangle', width: 28, height: 22, 'font-size': 11,
      'background-color': '#0f172a', 'border-color': '#cbd5e1', 'border-width': 1,
      'text-valign': 'center', 'text-outline-width': 0, 'z-index': 10
    }
  },
];

export function countPosition(position: cytoscape.Position): cytoscape.Position {
  return { x: position.x + 27, y: position.y - 17 };
}

export const countId = (node: string) => `count:${node}`;
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
        edgeLabel: `${edge.local_port} ↔ ${edge.remote_port}\n${down ? `${t('topology.linkDown')} · ` : ''}${metrics}`,
        highlight: groups.some(item =>
          carries(edge.source, edge.local_port, item.vlan) && carries(edge.target, edge.remote_port, item.vlan)),
      });
    });
    for (const [device, source] of parent) {
      const id = `attachment:${device}`;
      upsertEdge({ id, source, target: device, edgeLabel: '', lineWidth: 1, lineColor: '#94a3b8', path: path?.has(id) ?? false });
    }
  });
  if (initial && nodes.length) {
    const saved = model.nodes.some(node => Number.isFinite(node.x) && Number.isFinite(node.y));
    core.elements().not('node[kind = "device-count"]').layout({ name: saved ? 'preset' : 'grid' }).run();
  }
  core.batch(() => {
    for (const node of switchNodes) {
      const id = countId(node.id);
      const count = devices.get(node.id)?.length ?? 0;
      const position = countPosition(core.getElementById(node.id).position());
      const existing = core.getElementById(id);
      if (existing.length) {
        updateData(existing, { statusLabel: String(count) });
        const current = existing.position();
        if (current.x !== position.x || current.y !== position.y) existing.position(position);
      } else {
        core.add({ group: 'nodes', data: { id, kind: 'device-count', switchNode: node.id, statusLabel: String(count), statusColor: '#0f172a' },
          position, grabbable: false, selectable: false });
      }
    }
  });
}
