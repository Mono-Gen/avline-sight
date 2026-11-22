import type cytoscape from 'cytoscape';
import { asArray, decimal } from './format';
import { t } from './i18n';
import { buildTopology, linkAppearance, topologyHighlights, topologyQueriers } from './topologyModel';
import type { TopologySelection } from './topologyModel';
import type { Alert, Multicast, TopologyNode } from './types';
import { alertStates, alertTone, isPortAlert, summarizeAlerts } from './wallAlerts';

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

export function updateTopology(
  core: cytoscape.Core,
  model: ReturnType<typeof buildTopology>,
  multicast: Multicast | undefined,
  { group = '', vlan = 0, coreId, selection, initial = false, alerts = [] }: {
    group?: string; vlan?: number; coreId?: string; selection?: TopologySelection; initial?: boolean; alerts?: Alert[];
  } = {},
) {
  const { edges, parent, devices } = model;
  const { impacted, path } = topologyHighlights(model, coreId, selection);
  const queriers = topologyQueriers(multicast, vlan).switchIds;
  const rows = new Map([...devices.values()].flat().map(row => [`device:${row.mac}`, row]));
  const nodes: TopologyNode[] = [...model.nodes, ...Array.from(rows, ([id, row]) => ({
    id, kind: 'device', label: deviceLabel(row), reachable: row.seen,
  }))];
  const groups = asArray(multicast?.groups).filter(item => item.group === group);
  const members = new Set(groups.map(item => item.switch_id));
  const switchIds = new Map(nodes.map(node => [node.id, node.switch_id]));
  const carries = (node: string, port: string, vlan: number) => groups.some(item =>
    item.switch_id === switchIds.get(node) && item.vlan === vlan && asArray(item.ports).includes(port));
  const switchNodes = model.nodes.filter(node => node.kind === 'switch' && node.switch_id);
  const ids = new Set([...nodes.map(node => node.id), ...edges.map(edge => edge.id),
    ...switchNodes.map(node => countId(node.id)), ...Array.from(parent.keys(), id => `attachment:${id}`)]);
  core.batch(() => {
    core.elements().filter(element => !ids.has(element.id())).remove();
    nodes.forEach((node, index) => {
      const links = node.switch_id ? [] : edges.flatMap(edge => {
        const incoming = edge.target === node.id;
        if (!incoming && edge.source !== node.id) return [];
        const switchId = switchIds.get(incoming ? edge.source : edge.target);
        if (!switchId) return [];
        const portName = incoming ? edge.local_port : edge.remote_port;
        const relevant = portName ? alerts.filter(alert =>
          alert.switch_id === switchId && alert.port === portName && isPortAlert(alert)) : [];
        return [{ switchId, portName, issues: summarizeAlerts(relevant).items }];
      });
      const monitor = links.find(link => link.issues.some(alert => alert.active)) ||
        links.find(link => link.issues.some(alert => !alert.acked)) || links[0];
      const relevant = node.switch_id ? alerts.filter(alert => alert.switch_id === node.switch_id) :
        links.flatMap(link => link.issues);
      const row = rows.get(node.id);
      const switchId = node.switch_id || row?.switch_id || monitor?.switchId;
      const portName = row?.port || monitor?.portName;
      const deviceAlerts = row ? alerts.filter(alert => alert.switch_id === switchId && alert.port === portName && isPortAlert(alert)) : relevant;
      const statusTone = alertTone(summarizeAlerts(deviceAlerts), node.reachable === true);
      const status = alertStates[statusTone];
      const isImpacted = impacted.has(node.id);
      const isQuerier = node.switch_id !== undefined && queriers.has(node.switch_id) && node.reachable !== false;
      const badge = isQuerier ? `\n${t('topology.querierBadge', { vlan })}` : '';
      const impactLabel = isImpacted ? `\n${t('topology.affected')}` : '';
      const data = {
        id: node.id,
        kind: node.kind,
        label: node.label,
        monitorSwitchId: switchId || 0,
        monitorPortName: portName || '',
        statusTone,
        statusLabel: `${status.symbol} ${node.label}${badge}${impactLabel}`,
        statusColor: status.color,
        highlight: node.switch_id !== undefined && members.has(node.switch_id),
        impacted: isImpacted,
        querier: isQuerier,
        path: path.has(node.id),
        selected: selection?.kind === 'node' && selection.id === node.id,
      };
      const existing = core.getElementById(node.id);
      if (existing.length) {
        existing.data(data);
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
        existing.data(data);
      } else {
        existing.remove();
        core.add({ group: 'edges', data });
      }
    };
    edges.forEach(edge => {
      const down = model.failedLinks.has(edge.id);
      const { width, color, speed, util } = linkAppearance(edge, down, !model.failedNodes.has(edge.source));
      const metrics = t('topology.linkMetrics', {
        speed: speed === undefined ? '—' : `${speed} Mbps`,
        utilization: util === undefined ? '—' : `${decimal(util)}%`,
      });
      upsertEdge({
        id: edge.id, source: edge.source, target: edge.target, interSwitch: true,
        lineWidth: width, lineColor: color, down, path: path.has(edge.id),
        selected: selection?.kind === 'edge' && selection.id === edge.id,
        edgeLabel: `${edge.local_port} ↔ ${edge.remote_port}\n${down ? `${t('topology.linkDown')} · ` : ''}${metrics}`,
        highlight: groups.some(item =>
          carries(edge.source, edge.local_port, item.vlan) && carries(edge.target, edge.remote_port, item.vlan)),
      });
    });
    for (const [device, source] of parent) {
      const id = `attachment:${device}`;
      upsertEdge({ id, source, target: device, edgeLabel: '', lineWidth: 1, lineColor: '#94a3b8', path: path.has(id) });
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
        existing.data('statusLabel', String(count));
        existing.position(position);
      } else {
        core.add({ group: 'nodes', data: { id, kind: 'device-count', switchNode: node.id, statusLabel: String(count), statusColor: '#0f172a' },
          position, grabbable: false, selectable: false });
      }
    }
  });
}
