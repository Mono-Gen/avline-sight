const assert = require('node:assert/strict');
const { test } = require('node:test');
const cytoscape = require('cytoscape');
const { countId, countPosition, topologyStyles, updateTopology } = require('../src/topologyGraph.ts');
const { automaticCore, buildTopology, topologyHighlights, topologyQueriers } = require('../src/topologyModel.ts');
const { t } = require('../src/i18n/index.ts');

const edge = (id, source, target, local_port, remote_port, fields = {}) => ({
  id, source, target, local_port, remote_port, speed_mbps: 1000, util_pct: 20, ...fields,
});
const main = edge('main', 'a', 'b', '1', '1', { speed_mbps: 10000, util_pct: 95 });
const branch = edge('branch', 'b', 'c', '2', '1', { speed_mbps: 100, util_pct: 0 });
const spare = edge('spare', 'a', 'c', '2', '3', { blocking: true });
const topology = {
  nodes: ['a', 'b', 'c', 'd'].map((id, index) => ({
    id, label: id.toUpperCase(), kind: 'switch', switch_id: index + 1, reachable: true, is_root: index === 0,
  })),
  edges: [main, branch, spare],
};
const inventory = [
  { mac: 'AA', label: 'Console', switch_id: 1, port: '4', seen: true, source: 'lldp' },
  { mac: 'CC', label: 'Stagebox', switch_id: 3, port: '4', seen: true, source: 'fdb' },
  { mac: 'DD', label: 'Screen', switch_id: 4, port: '4', seen: true, source: 'fdb' },
  { mac: 'EE', switch_id: 1, port: '1', seen: true, via_uplink: true },
  { mac: 'FF', switch_id: 1, port: '5', seen: false },
];

function createGraph(context) {
  const graph = cytoscape({ headless: true, styleEnabled: true, style: topologyStyles });
  context.after(() => graph.destroy());
  return graph;
}

test('Topology highlights dependent switches and devices for selection and observed outages', context => {
  const graph = createGraph(context);
  const model = buildTopology(topology, [], inventory);
  assert.equal(model.defaultCore, 'a');
  assert.deepEqual(topologyHighlights(model, undefined, { kind: 'edge', id: main.id }).impacted, new Set());
  updateTopology(graph, model, undefined, { coreId: 'a', selection: { kind: 'node', id: 'b' } });
  assert.deepEqual(graph.nodes('[?impacted]').map(node => node.id()).sort(), ['c', 'device:CC']);
  assert.match(graph.getElementById('device:CC').data('statusLabel'), /⚠/);
  updateTopology(graph, model, undefined, { coreId: 'a', selection: { kind: 'edge', id: main.id } });
  assert.deepEqual(graph.nodes('[?impacted]').map(node => node.id()).sort(), ['b', 'c', 'device:CC']);

  const down = { switch_id: 2, port: { name: '1', oper: 'down' } };
  const missingLldp = { ...topology, edges: [branch, spare] };
  const failed = buildTopology(missingLldp, [down], inventory, model.edges);
  updateTopology(graph, failed, undefined, { coreId: 'a' });
  assert.deepEqual(graph.nodes('[?impacted]').map(node => node.id()).sort(), ['b', 'c', 'device:CC']);
  assert.equal(graph.getElementById(main.id).data('down'), true);
  assert.equal(buildTopology(missingLldp, [{ ...down, port: { ...down.port, oper: 'up' } }], inventory, failed.edges)
    .edges.some(link => link.id === main.id), false);
  const discarding = { ...topology, edges: [{ ...main, blocking: true }, branch, spare] };
  const knownOutage = buildTopology(discarding, [down], inventory, model.edges);
  assert.deepEqual(topologyHighlights(knownOutage, 'a').impacted, new Set(['b', 'c', 'device:CC']));
  assert.equal(buildTopology(discarding, [], inventory, model.edges).edges.some(link => link.id === main.id), false);

  const unreachable = { ...topology,
    nodes: topology.nodes.map(node => node.id === 'b' ? { ...node, reachable: false } : node),
  };
  assert.deepEqual(topologyHighlights(buildTopology(unreachable, [], inventory), 'a').impacted,
    new Set(['b', 'c', 'device:CC']));
  const redundant = { ...topology, edges: [...topology.edges, { ...spare, id: 'forwarding', blocking: false }] };
  const alternate = buildTopology(redundant, [down], inventory);
  assert.deepEqual(topologyHighlights(alternate, 'a').impacted, new Set());
});

test('Automatic path origin keeps the last known core through ambiguous election or loss of reachability', () => {
  const model = buildTopology(topology, [], []);
  const ambiguous = buildTopology({ ...topology, nodes: topology.nodes.map(node =>
    node.id === 'b' ? { ...node, is_root: true } : node) }, [], []);
  const newRoot = buildTopology({ ...topology, nodes: topology.nodes.map(node =>
    node.id === 'a' ? { ...node, reachable: false, is_root: false } :
      { ...node, is_root: node.id === 'b' }) }, [], []);
  assert.equal(automaticCore(model), 'a');
  assert.equal(automaticCore(ambiguous), undefined);
  assert.equal(automaticCore(ambiguous, 'a'), 'a');
  assert.equal(automaticCore(newRoot, 'a'), 'a');
});

test('Topology inter-switch width and color reflect speed and utilization, including idle and unknown', context => {
  const graph = createGraph(context);
  const unknown = edge('unknown', 'a', 'd', '8', '8', { speed_mbps: 0, util_pct: 0 });
  updateTopology(graph, buildTopology({ ...topology, edges: [...topology.edges, unknown] }, [], inventory));
  const fast = graph.getElementById(main.id);
  const idle = graph.getElementById(branch.id);
  const unavailable = graph.getElementById(unknown.id);
  assert.ok(parseFloat(fast.style('width')) > parseFloat(idle.style('width')));
  assert.notEqual(fast.style('line-color'), idle.style('line-color'));
  assert.match(fast.data('edgeLabel'), /10000 Mbps.*95[.,]0%/);
  assert.match(idle.data('edgeLabel'), /100 Mbps.*0[.,]0%/);
  assert.match(unavailable.data('edgeLabel'), /—.*—/);
  assert.doesNotMatch(unavailable.data('edgeLabel'), /0%/);
});

test('Topology marks the operational querier for the chosen VLAN and hides STP details', context => {
  const graph = createGraph(context);
  const model = buildTopology(topology, [], inventory);
  const multicast = { vlans: [
    { switch_id: 1, vlan: 20, querier_oper: 'querier' },
    { switch_id: 1, vlan: 10, querier_oper: 'non-querier', querier_admin: true },
    { switch_id: 2, vlan: 10, querier_oper: 'querier' },
  ] };
  assert.deepEqual(topologyQueriers(multicast, 10).vlans, [10, 20]);
  updateTopology(graph, model, multicast, { vlan: 10 });
  assert.deepEqual(graph.nodes('[?querier]').map(node => node.id()), ['b']);
  assert.ok(graph.getElementById('b').data('statusLabel').includes(t('topology.querierBadge', { vlan: 10 })));
  assert.equal(graph.getElementById('a').data('is_root'), undefined);
  assert.equal(graph.getElementById(spare.id).length, 0);
  updateTopology(graph, model, multicast, { vlan: 20 });
  assert.deepEqual(graph.nodes('[?querier]').map(node => node.id()), ['a']);
  updateTopology(graph, model, multicast);
  assert.equal(graph.nodes('[?querier]').length, 0);
});

test('Topology highlights the device attachment and each live hop to the core', context => {
  const graph = createGraph(context);
  const model = buildTopology(topology, [], inventory);
  const selection = { kind: 'node', id: 'device:CC' };
  updateTopology(graph, model, undefined, { coreId: 'a', selection });
  assert.deepEqual(graph.nodes('[?path]').map(node => node.id()).sort(), ['a', 'b', 'c', 'device:CC']);
  assert.deepEqual(graph.edges('[?path]').map(link => link.id()).sort(), ['attachment:device:CC', 'branch', 'main']);
  const failure = buildTopology(topology, [{ switch_id: 2, port: { name: '1', oper: 'down' } }], inventory);
  updateTopology(graph, failure, undefined, { coreId: 'a', selection });
  assert.equal(graph.elements('[?path]').length, 0);
  assert.equal(graph.getElementById('device:CC').data('impacted'), true);
});

test('Topology switch count badges point to the exact attached device list, including an empty switch', context => {
  const graph = createGraph(context);
  const model = buildTopology(topology, [], inventory);
  updateTopology(graph, model);
  for (const [id, macs] of [['a', ['AA']], ['b', []], ['c', ['CC']], ['d', ['DD']]]) {
    const badge = graph.getElementById(countId(id));
    assert.equal(badge.data('switchNode'), id);
    assert.equal(badge.data('statusLabel'), String(macs.length));
    assert.deepEqual((model.devices.get(badge.data('switchNode')) ?? []).map(row => row.mac), macs);
    assert.deepEqual(badge.position(), countPosition(graph.getElementById(id).position()));
  }
});
