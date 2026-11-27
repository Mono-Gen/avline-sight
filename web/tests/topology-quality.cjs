const assert = require('node:assert/strict');
const { test } = require('node:test');
const cytoscape = require('cytoscape');
const { countId, countPosition, topologyStyles, updateTopology } = require('../src/topologyGraph.ts');
const { automaticCore, buildTopology, linkAppearance, topologyHighlights, topologyNodeDetails, topologyQueriers } = require('../src/topologyModel.ts');
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
  const select = (model, selection) => updateTopology(graph, model, undefined,
    { selection, highlights: topologyHighlights(model, 'a', selection) });
  select(model, { kind: 'node', id: 'b' });
  assert.deepEqual(graph.nodes('[?impacted]').map(node => node.id()).sort(), ['c', 'device:CC']);
  assert.match(graph.getElementById('device:CC').data('statusLabel'), /⚠/);
  select(model, { kind: 'edge', id: main.id });
  assert.deepEqual(graph.nodes('[?impacted]').map(node => node.id()).sort(), ['b', 'c', 'device:CC']);

  const down = { switch_id: 2, port: { name: '1', oper: 'down' } };
  const missingLldp = { ...topology, edges: [branch, spare] };
  const failed = buildTopology(missingLldp, [down], inventory, model.edges);
  select(failed);
  assert.deepEqual(graph.nodes('[?impacted]').map(node => node.id()).sort(), ['b', 'c', 'device:CC']);
  assert.equal(graph.getElementById(main.id).data('down'), true);
  assert.equal(buildTopology(missingLldp, [{ ...down, port: { ...down.port, oper: 'up' } }], inventory, failed.edges)
    .edges.some(link => link.id === main.id), false);
  const discarding = { ...topology, edges: [{ ...main, blocking: true }, branch, spare] };
  const knownOutage = buildTopology(discarding, [down], inventory);
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
  const ports = [
    { switch_id: 1, port: { name: '1', oper: 'up', speed_mbps: 10000, util_rx: 95, util_tx: 5 } },
    { switch_id: 2, port: { name: '2', oper: 'up', speed_mbps: 100, util_rx: 0, util_tx: 0 } },
  ];
  updateTopology(graph, buildTopology({ ...topology, edges: [...topology.edges, unknown] }, ports, inventory));
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
  updateTopology(graph, model, undefined, { selection, highlights: topologyHighlights(model, 'a', selection) });
  assert.deepEqual(graph.nodes('[?path]').map(node => node.id()).sort(), ['a', 'b', 'c', 'device:CC']);
  assert.deepEqual(graph.edges('[?path]').map(link => link.id()).sort(), ['attachment:device:CC', 'branch', 'main']);
  const failure = buildTopology(topology, [{ switch_id: 2, port: { name: '1', oper: 'down' } }], inventory);
  updateTopology(graph, failure, undefined, { selection, highlights: topologyHighlights(failure, 'a', selection) });
  assert.equal(graph.elements('[?path]').length, 0);
  assert.equal(graph.getElementById('device:CC').data('impacted'), true);
});

test('Topology remembers a device attachment across uplink-only sightings and missing inventory', () => {
  const healthy = buildTopology(topology, [], inventory);
  const failedTopology = { ...topology, nodes: topology.nodes.map(node =>
    node.id === 'c' ? { ...node, reachable: false } : node) };
  const device = inventory.find(row => row.mac === 'CC');
  const uplink = { ...device, switch_id: 1, port: '1', via_uplink: true };
  const failed = buildTopology(failedTopology, [], [uplink], healthy.edges, healthy.deviceRows);
  assert.equal(failed.parent.get('device:CC'), 'c');
  assert.deepEqual(failed.deviceRows.get('device:CC'), { ...device, seen: false });
  assert.ok(topologyHighlights(failed, 'a').impacted.has('device:CC'));
  const stillFailed = buildTopology(failedTopology, [], [], failed.edges, failed.deviceRows);
  assert.equal(stillFailed.parent.get('device:CC'), 'c');
  const restored = buildTopology(topology, [], [{ ...uplink, port: '8', via_uplink: false }],
    stillFailed.edges, stillFailed.deviceRows);
  assert.equal(restored.parent.get('device:CC'), 'a');
  assert.equal(restored.deviceRows.get('device:CC').port, '8');
  assert.equal(buildTopology(topology, [], [uplink], failed.edges, failed.deviceRows).parent.has('device:CC'), false);
});

test('On first load, topology recovers failed switch devices from access ports only', () => {
  const failedTopology = { ...topology, nodes: topology.nodes.map(node =>
    node.id === 'c' ? { ...node, reachable: false } : node) };
  const port = (name, macs, fields = {}, switch_id = 3) => ({
    switch_id,
    port: {
      name, if_index: Number(name), pvid: 20, speed_mbps: 1000,
      devices: macs.map(mac => ({ mac, source: 'fdb', vlan: 0 })),
      ...fields,
    },
  });
  const ports = [
    port('4', ['CC', 'CA', 'AA', '']),
    port('1', ['INTERSWITCH']),
    port('3', ['BLOCKING']),
    port('5', ['UPLINK'], { av_role: 'uplink' }),
    port('6', ['TRUNK'], { av_role: 'trunk' }),
    port('7', ['LAG'], { type: 'lag' }),
    port('4', ['REACHABLE'], {}, 2),
    port('8', [], { devices: null }),
  ];
  const uplink = { ...inventory[1], switch_id: 1, switch_name: 'A', port: '1', if_index: 1, via_uplink: true };
  const model = buildTopology(failedTopology, ports, [uplink, inventory[0]]);
  assert.deepEqual([...model.parent], [['device:AA', 'a'], ['device:CC', 'c'], ['device:CA', 'c']]);
  const recovered = model.deviceRows.get('device:CC');
  assert.equal(recovered.label, 'Stagebox');
  assert.equal(recovered.switch_id, 3);
  assert.equal(recovered.switch_name, 'C');
  assert.equal(recovered.port, '4');
  assert.equal(recovered.if_index, 4);
  assert.equal(recovered.vlan, 20);
  assert.equal(recovered.seen, false);
  assert.equal(recovered.via_uplink, false);
  assert.ok(topologyHighlights(model, 'a').impacted.has('device:CC'));
});

test('Topology shows utilization only from operationally up ports on links that are up', context => {
  const graph = createGraph(context);
  const twoEnds = { ...topology, edges: [main] };
  const port = (switch_id, oper, util_rx) => ({
    switch_id, port: { name: '1', oper, speed_mbps: 1000, util_rx, util_tx: 0 },
  });
  const up = port(2, 'up', 23);
  const testing = port(1, 'testing', 99);
  const down = port(1, 'down', 99);
  for (const [ports, util, isDown] of [
    [[testing, up], 23, false],
    [[testing], undefined, false],
    [[], undefined, false],
    [[down, up], undefined, true],
  ]) {
    const model = buildTopology(twoEnds, ports, []);
    assert.equal(model.linkMetrics.get(main.id).util, util);
    updateTopology(graph, model);
    assert.equal(graph.getElementById(main.id).data('down'), isDown);
    const label = graph.getElementById(main.id).data('edgeLabel');
    if (util === undefined) assert.doesNotMatch(label, /%/);
    else assert.match(label, /23[.,]0%/);
  }
  assert.equal(linkAppearance({ speed: 1000, util: 23 }, true).util, undefined);
});

test('Topology reads the reachable endpoint, refreshes retained link metrics, and drops stale samples', context => {
  const graph = createGraph(context);
  const outage = (...failed) => topology.nodes.map(node => ({ ...node, reachable: !failed.includes(node.id) }));
  const sample = (switch_id, speed_mbps, util_rx, util_tx) => ({
    switch_id, port: { name: '1', oper: 'up', speed_mbps, util_rx, util_tx },
  });
  const ports = [sample(1, 10000, 99, 80), sample(2, 1000, 7, 23)];
  const check = (model, expected) => {
    updateTopology(graph, model);
    assert.match(graph.getElementById(main.id).data('edgeLabel'), expected);
  };
  const current = buildTopology({ nodes: outage('a'), edges: [main] }, ports, []);
  check(current, /1000 Mbps.*23[.,]0%/);
  const retained = buildTopology({ nodes: outage('a'), edges: [] },
    [ports[0], sample(2, 100, 42, 12)], [], current.edges);
  check(retained, /100 Mbps.*42[.,]0%/);
  check(buildTopology({ nodes: outage('b'), edges: [] }, ports, [], retained.edges), /10000 Mbps.*99[.,]0%/);
  check(buildTopology({ nodes: outage('a', 'b'), edges: [] }, ports, [], retained.edges), /—.*—/);
});

test('Topology uses device MAC alerts and limits port fallback to alerts for the port itself', context => {
  const graph = createGraph(context);
  const rows = [
    { ...inventory[0], if_index: 4 },
    { ...inventory[0], mac: 'AB', if_index: 4 },
    inventory[1],
  ];
  const alerts = [
    { mac: 'aa', switch_id: 2, if_index: 1, active: true },
    { mac: 'OTHER', switch_id: 1, if_index: 4, port: '4', active: true },
    { mac: 'OTHER', switch_id: 3, if_index: 4, port: '4', active: true },
    { switch_id: 3, if_index: 4, port: '4', active: false },
  ].map(alert => ({ severity: alert.active ? 'CRITICAL' : 'WARNING', acked: false, ...alert }));
  const model = buildTopology(topology, [], rows);
  updateTopology(graph, model, undefined, { nodeDetails: topologyNodeDetails(model, alerts) });
  const tone = mac => graph.getElementById(`device:${mac}`).data('statusTone');
  assert.equal(tone('AA'), 'critical');
  assert.equal(tone('AB'), 'healthy');
  assert.equal(tone('CC'), 'warning');
  updateTopology(graph, model, undefined, { nodeDetails: topologyNodeDetails(model,
    [{ switch_id: 1, if_index: 4, port: 'old-name', active: false, acked: false, severity: 'WARNING' }]) });
  assert.equal(tone('AB'), 'warning');
});

test('Topology writes data only when node, edge or count values change', context => {
  const graph = createGraph(context);
  const model = buildTopology(topology, [], inventory);
  updateTopology(graph, model);
  const writes = [];
  graph.on('data', 'node, edge', event => writes.push(event.target.id()));
  updateTopology(graph, buildTopology(topology, [], inventory));
  assert.deepEqual(writes, []);
  const ports = [{ switch_id: 1, port: { name: '1', oper: 'up', speed_mbps: 10000, util_rx: 17, util_tx: 10 } }];
  updateTopology(graph, buildTopology(topology, ports, inventory));
  assert.deepEqual(writes, [main.id]);
});

test('Topology switch count badges point to the exact attached device list, including an empty switch', context => {
  const graph = createGraph(context);
  const model = buildTopology(topology, [], inventory);
  updateTopology(graph, model);
  for (const [id, macs] of [['a', ['AA']], ['b', []], ['c', ['CC']], ['d', ['DD']]]) {
    const badge = graph.getElementById(countId(id));
    assert.equal(badge.data('switchNode'), id);
    assert.equal(badge.data('statusLabel'), `${macs.length}台`); // the badge carries its unit (2026-11-28 UI review)
    assert.deepEqual((model.devices.get(badge.data('switchNode')) ?? []).map(row => row.mac), macs);
    assert.deepEqual(badge.position(), countPosition(graph.getElementById(id).position()));
  }
});
