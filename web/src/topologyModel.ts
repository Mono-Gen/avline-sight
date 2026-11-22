import { asArray } from './format';
import type { InventoryRow, Multicast, Port, Topology, TopologyEdge } from './types';

export type TopologySelection = { kind: 'node' | 'edge'; id: string };
export type TopologyPort = { switch_id: number; port: Port };

export function buildTopology(
  topology: Topology,
  ports: TopologyPort[],
  inventory: InventoryRow[],
  previousLinks: TopologyEdge[] = [],
) {
  const nodes = asArray(topology.nodes);
  const byId = new Map(nodes.map(node => [node.id, node]));
  const portKey = (id: number | undefined, name: string) => `${id}/${name}`;
  const portByName = new Map(asArray(ports).map(row => [portKey(row.switch_id, row.port.name), row.port]));
  const endpoints = (edge: TopologyEdge) => [
    { node: byId.get(edge.source), name: edge.local_port },
    { node: byId.get(edge.target), name: edge.remote_port },
  ];
  const down = (edge: TopologyEdge) => endpoints(edge).some(({ node, name }) => {
    const oper = portByName.get(portKey(node?.switch_id, name))?.oper;
    return node?.reachable !== false && (oper === 'down' || oper === 'lowerLayerDown' || oper === 'notPresent');
  });
  const current = asArray(topology.edges);
  const currentIds = new Set(current.map(edge => edge.id));
  const previousById = new Map(previousLinks.map(edge => [edge.id, edge]));
  const currentPorts = new Set(current.flatMap(edge =>
    [`${edge.source}/${edge.local_port}`, `${edge.target}/${edge.remote_port}`]));
  const retained = previousLinks.filter(edge => !currentIds.has(edge.id) &&
    !currentPorts.has(`${edge.source}/${edge.local_port}`) &&
    !currentPorts.has(`${edge.target}/${edge.remote_port}`) &&
    (down(edge) || endpoints(edge).some(({ node }) => node?.reachable === false)));
  const edges = [...current, ...retained].flatMap(edge => {
    if (edge.source === edge.target || !byId.has(edge.source) || !byId.has(edge.target)) return [];
    if (!edge.blocking) return [edge];
    const prior = previousById.get(edge.id);
    return prior && !prior.blocking && down(edge) ? [prior] : [];
  });
  const devices = new Map<string, InventoryRow[]>();
  const parent = new Map<string, string>();
  const switches = new Map(nodes.filter(node => node.switch_id).map(node => [node.switch_id, node]));
  for (const row of asArray(inventory)) {
    const node = switches.get(row.switch_id);
    if (!node || row.via_uplink || !row.mac || !row.port || !(row.seen || (node.reachable === false && row.source))) continue;
    const id = `device:${row.mac}`;
    if (parent.has(id)) continue;
    const attached = devices.get(node.id) ?? [];
    attached.push(row);
    devices.set(node.id, attached);
    parent.set(id, node.id);
  }
  const failedNodes = new Set(nodes.filter(node => node.reachable === false).map(node => node.id));
  const failedLinks = new Set(edges.filter(down).map(edge => edge.id));
  const defaultCore = nodes.filter(node => node.kind === 'switch' && node.is_root);
  return { nodes, edges, devices, parent, failedNodes, failedLinks,
    defaultCore: defaultCore.length === 1 ? defaultCore[0].id : undefined };
}

export function automaticCore(model: ReturnType<typeof buildTopology>, previous: string | undefined) {
  const known = model.nodes.find(node => node.id === previous && node.kind === 'switch');
  if (known && (known.reachable === false || !model.defaultCore)) return known.id;
  return model.defaultCore;
}

export function topologyHighlights(
  model: ReturnType<typeof buildTopology>, core: string | undefined, selection?: TopologySelection,
) {
  const impacted = new Set<string>();
  const path = new Set<string>();
  if (!core) return { impacted, path };
  const adjacency = new Map<string, { node: string; edge: string }[]>();
  for (const edge of model.edges) {
    for (const [from, to] of [[edge.source, edge.target], [edge.target, edge.source]]) {
      const links = adjacency.get(from) ?? [];
      links.push({ node: to, edge: edge.id });
      adjacency.set(from, links);
    }
  }
  const walk = (live: boolean, simulate = false) => {
    const unavailable = (id: string) => (live && model.failedNodes.has(id)) ||
      (simulate && selection?.kind === 'node' && selection.id === id);
    const visited = new Map<string, { node: string; edge: string } | null>();
    if (unavailable(core)) return visited;
    visited.set(core, null);
    const queue = [core];
    for (let index = 0; index < queue.length; index++) {
      const node = queue[index];
      for (const link of adjacency.get(node) ?? []) {
        if (visited.has(link.node) || unavailable(link.node) ||
          (live && model.failedLinks.has(link.edge)) ||
          (simulate && selection?.kind === 'edge' && selection.id === link.edge)) continue;
        visited.set(link.node, { node, edge: link.edge });
        queue.push(link.node);
      }
    }
    return visited;
  };
  const baseline = walk(false);
  const live = walk(true);
  const selectedParent = selection?.kind === 'node' ? model.parent.get(selection.id) : undefined;
  const available = selection && !selectedParent ? walk(true, true) : live;
  for (const node of baseline.keys()) {
    if (!available.has(node) && (model.failedNodes.has(node) || selection?.id !== node)) impacted.add(node);
  }
  for (const [device, node] of model.parent) {
    if (baseline.has(node) && !available.has(node)) impacted.add(device);
  }
  if (selectedParent && live.has(selectedParent) && selection) {
    path.add(selection.id);
    path.add(`attachment:${selection.id}`);
    let node = selectedParent;
    for (let hop = live.get(node); hop; hop = live.get(node)) {
      path.add(node);
      path.add(hop.edge);
      node = hop.node;
    }
    path.add(core);
  }
  return { impacted, path };
}

export function topologyQueriers(multicast: Multicast | undefined, vlan: number) {
  const rows = asArray(multicast?.vlans);
  return {
    vlans: [...new Set(rows.map(row => row.vlan))].sort((a, b) => a - b),
    switchIds: new Set(rows.filter(row => row.vlan === vlan && row.querier_oper === 'querier').map(row => row.switch_id)),
  };
}

export function linkAppearance(edge: TopologyEdge, down: boolean, telemetryLive: boolean) {
  const speed = Number.isFinite(edge.speed_mbps) && edge.speed_mbps > 0 ? edge.speed_mbps : undefined;
  const util = telemetryLive && !down && speed !== undefined && Number.isFinite(edge.util_pct) && edge.util_pct >= 0
    ? edge.util_pct : undefined;
  const width = speed === undefined ? 2 : speed < 1000 ? 3 : speed < 10000 ? 5 : 7;
  const color = down ? '#f87171' : util === undefined ? '#94a3b8' : util >= 90 ? '#fb923c' : util >= 70 ? '#fde047' : '#67e8f9';
  return { speed, util, width, color };
}
