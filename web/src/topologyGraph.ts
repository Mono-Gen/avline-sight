import type cytoscape from 'cytoscape';
import { asArray } from './format';
import type { Alert, Multicast, Topology } from './types';
import { alertStates, alertTone, isPortAlert, summarizeAlerts } from './wallAlerts';

export const topologyStyles: cytoscape.Stylesheet[] = [
  {
    selector: 'node',
    style: {
      label: 'data(statusLabel)',
      'background-color': 'data(statusColor)',
      color: '#ddd',
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
      width: 3,
      'line-color': '#94a3b8',
      label: 'data(local_port)',
      'font-size': 9,
      color: '#ddd'
    }
  },
  { selector: 'edge[?blocking]', style: { 'line-style': 'dashed' } },
  { selector: 'edge.hidden', style: { display: 'none' } },
  {
    selector: 'node[?highlight]',
    style: { 'border-color': '#7dd3fc', 'border-width': 3 }
  },
  { selector: 'edge[?highlight]', style: { 'line-color': '#f59e0b', width: 5 } },
];

export function updateTopology(
  core: cytoscape.Core,
  topology: Topology,
  multicast: Multicast | undefined,
  selectedGroup: string,
  showBlocking: boolean,
  initial: boolean,
  alerts: Alert[] = [],
) {
  const nodes = asArray(topology.nodes);
  const edges = asArray(topology.edges);
  const groups = asArray(multicast?.groups).filter(item => item.group === selectedGroup);
  const members = new Set(groups.map(item => item.switch_id));
  const switchIds = new Map(nodes.map(node => [node.id, node.switch_id]));
  const carries = (node: string, port: string, vlan: number) => groups.some(item =>
    item.switch_id === switchIds.get(node) && item.vlan === vlan && asArray(item.ports).includes(port));
  const ids = new Set([...nodes.map(node => node.id), ...edges.map(edge => edge.id)]);
  const nodeIds = new Set(nodes.map(node => node.id));
  core.batch(() => {
    core.elements().filter(element => !ids.has(element.id())).remove();
    nodes.forEach((node, index) => {
      const localEdge = node.switch_id ? undefined : edges.find(edge => edge.target === node.id && switchIds.get(edge.source));
      const switchId = node.switch_id || (localEdge && switchIds.get(localEdge.source));
      const portName = localEdge?.local_port;
      const relevant = node.switch_id ? alerts.filter(alert => alert.switch_id === switchId) :
        switchId && portName ? alerts.filter(alert => alert.switch_id === switchId && alert.port === portName && isPortAlert(alert)) : [];
      const statusTone = alertTone(summarizeAlerts(relevant), node.reachable === true);
      const status = alertStates[statusTone];
      const data = {
        ...node,
        monitorSwitchId: switchId || 0,
        monitorPortName: portName || '',
        statusTone,
        statusLabel: `${status.symbol} ${node.label}`,
        statusColor: status.color,
        highlight: Boolean(selectedGroup && node.switch_id !== undefined && members.has(node.switch_id))
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
    edges.forEach(edge => {
      if (!nodeIds.has(edge.source) || !nodeIds.has(edge.target)) return;
      const data = {
        ...edge,
        blocking: Boolean(edge.blocking),
        highlight: Boolean(selectedGroup && groups.some(item =>
          carries(edge.source, edge.local_port, item.vlan) && carries(edge.target, edge.remote_port, item.vlan))),
      };
      let existing = core.getElementById(edge.id);
      if (existing.length && (existing.data('source') !== edge.source || existing.data('target') !== edge.target)) {
        existing.remove();
        existing = core.collection();
      }
      const element = existing.length ? existing.data(data) : core.add({ group: 'edges', data });
      element.toggleClass('hidden', !showBlocking && data.blocking);
    });
  });
  
  if (initial && nodes.length) {
    const saved = nodes.some(node => Number.isFinite(node.x) && Number.isFinite(node.y));
    core.layout({ name: saved ? 'preset' : 'grid' }).run();
  }
}
