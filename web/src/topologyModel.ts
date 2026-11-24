import { asArray } from './format';
import type { Alert, InventoryRow, Multicast, Port, Topology, TopologyEdge } from './types';
import { alertTone, isPortAlert, matchingDeviceAlerts, summarizeAlerts } from './wallAlerts';

export type TopologySelection = { kind: 'node' | 'edge'; id: string };
export type TopologyPort = { switch_id: number; port: Port };

export function buildTopology(
  topology: Topology,
  ports: TopologyPort[],
  inventory: InventoryRow[],
  previousLinks: TopologyEdge[] = [],
  previousDevices: ReadonlyMap<string, InventoryRow> = new Map(),
) {
  const nodes = asArray(topology.nodes);
  const byId = new Map(nodes.map(node => [node.id, node]));
  const portKey = (id: number | undefined, name: string) => `${id}/${name}`;
  const portByName = new Map(asArray(ports).map(row => [portKey(row.switch_id, row.port.name), row.port]));
  const endpoints = (edge: TopologyEdge) => [
    { node: byId.get(edge.source), name: edge.local_port },
    { node: byId.get(edge.target), name: edge.remote_port },
  ];
  const livePort = (node: (typeof nodes)[number] | undefined, name: string) =>
    node?.switch_id && node.reachable !== false ? portByName.get(portKey(node.switch_id, name)) : undefined;
  const down = (edge: TopologyEdge) => endpoints(edge).some(({ node, name }) => {
    const oper = livePort(node, name)?.oper;
    return oper === 'down' || oper === 'lowerLayerDown' || oper === 'notPresent';
  });
  const current = asArray(topology.edges);
  const currentIds = new Set(current.map(edge => edge.id));
  const currentPorts = new Set(current.flatMap(edge =>
    [`${edge.source}/${edge.local_port}`, `${edge.target}/${edge.remote_port}`]));
  const retained = previousLinks.filter(edge => !currentIds.has(edge.id) &&
    !currentPorts.has(`${edge.source}/${edge.local_port}`) &&
    !currentPorts.has(`${edge.target}/${edge.remote_port}`) &&
    (down(edge) || endpoints(edge).some(({ node }) => node?.reachable === false)));
  const edges = [...current, ...retained].filter(edge => edge.source !== edge.target &&
    byId.has(edge.source) && byId.has(edge.target) && (!edge.blocking || down(edge)));
  const failedLinks = new Set(edges.filter(down).map(edge => edge.id));
  const linkMetrics = new Map(edges.map(edge => {
    const ends = endpoints(edge);
    const [source] = ends;
    const samples = ends.map(({ node, name }) => livePort(node, name));
    const up = samples.find(port => port?.oper === 'up' && port.speed_mbps > 0);
    const speedPort = up ?? samples.find(port => port && port.speed_mbps > 0);
    if (speedPort) return [edge.id, {
      speed: speedPort.speed_mbps,
      util: up && !failedLinks.has(edge.id) ? Math.max(up.util_rx, up.util_tx) : undefined,
    }];
    const [sourcePort] = samples;
    const currentSource = !sourcePort && source.node?.switch_id && source.node.reachable !== false && currentIds.has(edge.id);
    return [edge.id, currentSource ? { speed: edge.speed_mbps } : {}];
  }));
  const failedNodes = new Set(nodes.filter(node => node.reachable === false).map(node => node.id));
  const devices = new Map<string, InventoryRow[]>();
  const deviceRows = new Map<string, InventoryRow>();
  const parent = new Map<string, string>();
  const switches = new Map(nodes.filter(node => node.switch_id).map(node => [node.switch_id, node]));
  const attach = (id: string, row: InventoryRow, nodeId: string) => {
    if (parent.has(id)) return;
    const attached = devices.get(nodeId) ?? [];
    const device = failedNodes.has(nodeId) && row.seen ? { ...row, seen: false } : row;
    attached.push(device);
    devices.set(nodeId, attached);
    deviceRows.set(id, device);
    parent.set(id, nodeId);
  };
  for (const row of asArray(inventory)) {
    const node = switches.get(row.switch_id);
    if (!node || row.via_uplink || !row.mac || !row.port || !(row.seen || (failedNodes.has(node.id) && row.source))) continue;
    attach(`device:${row.mac}`, row, node.id);
  }
  for (const [id, row] of previousDevices) {
    const node = switches.get(row.switch_id);
    if (node && failedNodes.has(node.id)) attach(id, row, node.id);
  }
  if (failedNodes.size > 0) {
    const uplinks = new Set([...current, ...retained].flatMap(edge =>
      endpoints(edge).map(({ node, name }) => portKey(node?.switch_id, name))));
    const inventoryByMac = new Map(asArray(inventory).map(row => [row.mac, row]));
    for (const { switch_id, port } of asArray(ports)) {
      const node = switches.get(switch_id);
      if (!node || !failedNodes.has(node.id) || !port.name || uplinks.has(portKey(switch_id, port.name)) ||
        port.av_role === 'uplink' || port.av_role === 'trunk' || port.type === 'lag') continue;
      const portDevices = asArray(port.devices);
      for (const device of portDevices) {
        const id = `device:${device.mac}`;
        if (!device.mac || parent.has(id)) continue;
        const known = inventoryByMac.get(device.mac);
        attach(id, {
          ...known,
          mac: device.mac,
          label: known?.label || device.label || '',
          note: known?.note ?? '',
          ip: known?.ip || device.ip || '',
          vendor: known?.vendor || device.vendor || '',
          lldp_name: known?.lldp_name || device.lldp_name || '',
          switch_id,
          switch_name: node.label,
          group_tag: known?.switch_id === switch_id ? known.group_tag : '',
          port: port.name,
          if_index: port.if_index,
          vlan: device.vlan || port.pvid,
          poe_w: portDevices.length === 1 ? port.poe?.power_w : undefined,
          speed_mbps: port.speed_mbps,
          seen: false,
          source: device.source,
          via_uplink: false,
        }, node.id);
      }
    }
  }
  const defaultCore = nodes.filter(node => node.kind === 'switch' && node.is_root);
  return { nodes, byId, edges, linkMetrics, devices, deviceRows, parent, failedNodes, failedLinks,
    defaultCore: defaultCore.length === 1 ? defaultCore[0].id : undefined };
}

interface MonitorTarget {
  switchId: number;
  portName?: string;
}

export function topologyNodeDetails(model: ReturnType<typeof buildTopology>, alerts: Alert[] = []) {
  const bySwitch = new Map<number, Alert[]>();
  const byPortName = new Map<string, Alert[]>();
  const byPortIndex = new Map<string, Alert[]>();
  const byMac = new Map<string, Alert[]>();
  const portKey = (switchId: number | undefined, port: string | number) => `${switchId}/${port}`;
  const add = <K>(index: Map<K, Alert[]>, key: K, alert: Alert) => {
    const items = index.get(key);
    if (items) items.push(alert);
    else index.set(key, [alert]);
  };
  for (const alert of alerts) {
    add(bySwitch, alert.switch_id, alert);
    if (alert.mac) add(byMac, alert.mac.toLowerCase(), alert);
    if (isPortAlert(alert)) {
      add(byPortName, portKey(alert.switch_id, alert.port), alert);
      if (!alert.mac) add(byPortIndex, portKey(alert.switch_id, alert.if_index), alert);
    }
  }

  const details = new Map<string, { monitorTarget?: MonitorTarget; statusTone: ReturnType<typeof alertTone> }>();
  for (const node of model.nodes) {
    let monitorTarget: MonitorTarget | undefined;
    let relevant: Alert[] = [];
    if (node.switch_id) {
      monitorTarget = { switchId: node.switch_id };
      relevant = bySwitch.get(node.switch_id) ?? [];
    } else if (node.kind === 'external') {
      const links = model.edges.flatMap(edge => {
        const incoming = edge.target === node.id;
        if (!incoming && edge.source !== node.id) return [];
        const switchId = model.byId.get(incoming ? edge.source : edge.target)?.switch_id;
        if (!switchId) return [];
        const portName = incoming ? edge.local_port : edge.remote_port;
        const issues = summarizeAlerts(byPortName.get(portKey(switchId, portName)) ?? []);
        return [{ target: { switchId, portName }, issues }];
      });
      monitorTarget = (links.find(link => link.issues.active) ||
        links.find(link => link.issues.unacked) || links[0])?.target;
      relevant = links.flatMap(link => link.issues.items);
    }
    details.set(node.id, { monitorTarget, statusTone: alertTone(summarizeAlerts(relevant), node.reachable === true) });
  }
  for (const [id, row] of model.deviceRows) {
    const macAlerts = byMac.get(row.mac.toLowerCase()) ?? [];
    const portAlerts = byPortIndex.get(portKey(row.switch_id, row.if_index ?? 0)) ?? [];
    const relevant = matchingDeviceAlerts([...macAlerts, ...portAlerts], row);
    if (!row.mac || !row.if_index) {
      for (const alert of byPortName.get(portKey(row.switch_id, row.port)) ?? []) {
        if (!alert.mac && !relevant.includes(alert)) relevant.push(alert);
      }
    }
    const monitorTarget = row.switch_id ? { switchId: row.switch_id, portName: row.port } : undefined;
    details.set(id, { monitorTarget, statusTone: alertTone(summarizeAlerts(relevant), row.seen) });
  }
  return details;
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

export function linkAppearance(metrics: { speed?: number; util?: number } | undefined, down: boolean) {
  const speed = metrics?.speed !== undefined && Number.isFinite(metrics.speed) && metrics.speed > 0 ? metrics.speed : undefined;
  const util = !down && speed !== undefined && metrics?.util !== undefined && Number.isFinite(metrics.util) && metrics.util >= 0
    ? metrics.util : undefined;
  const width = speed === undefined ? 2 : speed < 1000 ? 3 : speed < 10000 ? 5 : 7;
  const color = down ? '#ff4d5e' : util === undefined ? '#9aa3b2' : util >= 90 ? '#fb923c' : util >= 70 ? '#fde047' : '#67e8f9';
  return { speed, util, width, color };
}
