import { t, useLang } from '../i18n';
import cytoscape from 'cytoscape';
import { useEffect, useMemo, useRef, useState } from 'react';
import { api, json, upload } from '../api';
import { WatchBar } from '../components/Dialogs';
import { PollingStatus } from '../components/PollingStatus';
import { asArray } from '../format';
import { countId, countPosition, deviceLabel, topologyStyles, updateTopology } from '../topologyGraph';
import { automaticCore, buildTopology, topologyHighlights, topologyQueriers } from '../topologyModel';
import type { TopologyPort, TopologySelection } from '../topologyModel';
import type { InventoryRow, Topology as Graph, Multicast, TopologyEdge } from '../types';
import { TOPOLOGY_REFRESH_MS, usePolling } from '../usePolling';
import { DeviceInventoryTable } from './Devices';
import type { ViewProps } from './props';

export function Topology(props: ViewProps & { openDevices: () => void }) {
  const [language] = useLang();
  const [group, setGroup] = useState('');
  const [vlan, setVlan] = useState(0);
  const [chosenCore, setChosenCore] = useState('');
  const [selection, setSelection] = useState<TopologySelection>();
  const [listedSwitch, setListedSwitch] = useState('');
  const container = useRef<HTMLDivElement>(null);
  const graph = useRef<cytoscape.Core>();
  const initialized = useRef(false);
  const knownLinks = useRef<TopologyEdge[]>([]);
  const knownCore = useRef<string>();
  const result = usePolling(async signal => {
    const [topology, multicast, ports, inventory] = await Promise.all([
      api<Graph>('/api/topology', { signal }),
      api<Multicast>('/api/multicast', { signal }),
      api<TopologyPort[]>('/api/ports/search', { signal }),
      api<InventoryRow[]>('/api/devices?scope=all&include=registration', { signal }),
    ]);
    return { topology, multicast, ports, inventory };
  }, TOPOLOGY_REFRESH_MS, [], !props.replay);
  const data = result.data;
  const model = useMemo(() => data && buildTopology(data.topology, data.ports, data.inventory, knownLinks.current), [data]);
  const autoCore = model && automaticCore(model, knownCore.current);
  useEffect(() => {
    if (model) {
      knownLinks.current = model.edges;
      knownCore.current = autoCore;
    }
  }, [model, autoCore]);
  const switches = model?.nodes.filter(node => node.kind === 'switch') ?? [];
  const coreId = switches.find(node => node.id === chosenCore)?.id ?? autoCore;
  const listed = switches.find(node => node.id === listedSwitch);
  const listedDevices = listed ? model?.devices.get(listed.id) ?? [] : [];
  const selectedNode = model?.nodes.find(node => selection?.kind === 'node' && node.id === selection.id);
  const selectedDevice = selection?.kind === 'node' && model?.parent.has(selection.id);
  const selectedLink = model?.edges.find(edge => selection?.kind === 'edge' && edge.id === selection.id);
  const activeSelection = selectedNode || selectedDevice || selectedLink ? selection : undefined;
  const selectedVlan = topologyQueriers(data?.multicast, vlan);

  useEffect(() => {
    if (!container.current) return;
    const instance = cytoscape({
      container: container.current,
      elements: [],
      layout: { name: 'preset' },
      style: topologyStyles
    });
    graph.current = instance;
    instance.on('tap', 'node', event => {
      const switchNode = event.target.data('switchNode');
      if (switchNode) setListedSwitch(switchNode);
      else setSelection({ kind: 'node', id: event.target.id() });
    });
    instance.on('tap', 'edge[?interSwitch]', event => setSelection({ kind: 'edge', id: event.target.id() }));
    instance.on('tap', event => { if (event.target === instance) setSelection(undefined); });
    instance.on('position', 'node[kind = "switch"]', event => {
      instance.getElementById(countId(event.target.id())).position(countPosition(event.target.position()));
    });
    return () => {
      instance.destroy();
      graph.current = undefined;
      initialized.current = false;
    };
  }, []);
  useEffect(() => {
    if (!graph.current || !model) return;
    updateTopology(graph.current, model, data?.multicast, {
      group, vlan, coreId, selection: activeSelection, initial: !initialized.current, alerts: props.operations?.alerts,
    });
    if (model.nodes.length) initialized.current = true;
  }, [model, data, group, vlan, coreId, activeSelection, props.operations?.alerts, language]);

  const background = data?.topology.background;
  const switchNames = new Map(model?.nodes.map(node => [node.id, node.label]));
  const choices = model ? [
    ...model.nodes.map(node => ({ kind: 'node' as const, id: node.id, label: node.label })),
    ...model.edges.map(edge => ({ kind: 'edge' as const, id: edge.id,
      label: `${switchNames.get(edge.source)} ${edge.local_port} ↔ ${switchNames.get(edge.target)} ${edge.remote_port}` })),
    ...[...model.devices.values()].flat().map(row => ({ kind: 'node' as const, id: `device:${row.mac}`,
      label: `${deviceLabel(row)} · ${row.switch_name} ${row.port}` })),
  ] : [];
  const selectionKey = ({ kind, id }: TopologySelection) => `${kind}:${id}`;
  const monitorTarget = activeSelection?.kind === 'node' ? graph.current?.getElementById(activeSelection.id) : undefined;
  const monitorSwitchId = selectedNode?.switch_id || monitorTarget?.data('monitorSwitchId');
  const impact = model && topologyHighlights(model, coreId, activeSelection);
  return (
    <>
      <WatchBar items={props.watch} onChange={props.setWatch} run={props.run} disabled={props.locked || props.busy} />
      <PollingStatus {...result} />
      <div className="flex flex-wrap gap-2">
        <label>{t('topology.core')} <select value={chosenCore} onChange={event => setChosenCore(event.target.value)}>
          <option value="">{t('topology.autoCore')}</option>
          {switches.map(node => <option key={node.id} value={node.id}>{node.label}</option>)}
        </select></label>
        <label>{t('topology.querierVlan')} <select value={vlan} onChange={event => setVlan(Number(event.target.value))}>
          <option value={0}>{t('topology.noVlan')}</option>
          {selectedVlan.vlans.map(id => <option key={id} value={id}>VLAN {id}</option>)}
        </select></label>
        <select aria-label={t('ui.m303')} value={group} onChange={event => setGroup(event.target.value)}>
          <option value="">{t('ui.m303')}</option>
          {Array.from(new Set(asArray(result.data?.multicast.groups).map(item => item.group))).map(address => <option key={address}>{address}</option>)}
        </select>
        <button
          disabled={props.locked || props.busy}
          onClick={() => props.run(async () => {
            const positions: Record<string, {
              x: number;
              y: number
            }> = {};
            graph.current?.nodes('[kind = "switch"], [kind = "external"]').forEach(node => { positions[node.id()] = node.position(); });
            await api('/api/topology/layout', json('PUT', { positions }));
          })}
        >{t('ui.m304')}</button>
        <label>{t('ui.m305')}<input
          type="file"
          accept="image/*"
          disabled={props.locked || props.busy}
          onChange={event => {
            const file = event.target.files?.[0];
            if (file) props.run(async () => {
              await api('/api/topology/background', upload(file));
              await result.refresh();
            });
          }}
        /></label>
      </div>
      {group && <p>{t('ui.m306')}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <label>{t('topology.selection')} <select className="max-w-full"
          value={activeSelection ? selectionKey(activeSelection) : ''}
          onChange={event => setSelection(choices.find(item => selectionKey(item) === event.target.value))}>
          <option value="">{t('topology.noSelection')}</option>
          {choices.map(item => <option key={selectionKey(item)} value={selectionKey(item)}>{item.label}</option>)}
        </select></label>
        {selectedNode?.switch_id && <button onClick={() => setListedSwitch(selectedNode.id)}>
          {t('topology.deviceCount', { count: model?.devices.get(selectedNode.id)?.length ?? 0 })}
        </button>}
        {Boolean(monitorSwitchId) && <button onClick={() => props.openPortMonitor?.({
          switchId: monitorSwitchId, portName: monitorTarget?.data('monitorPortName'),
        })}>{t('rack.nav.monitor')}</button>}
      </div>
      <div role="status">
        {model && !coreId && switches.length > 0 && <p>{t('topology.chooseCore')}</p>}
        {coreId && selectedDevice && !impact?.path.size && <p>{t('topology.noPath')}</p>}
        {vlan > 0 && !switches.some(node => node.reachable !== false && node.switch_id && selectedVlan.switchIds.has(node.switch_id)) &&
          <p>{t('topology.noQuerier', { vlan })}</p>}
      </div>
      {listed && <section className="panel space-y-2" aria-label={t('topology.deviceList', { name: listed.label })}>
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="font-bold">{t('topology.deviceList', { name: listed.label })}</h2>
          <button onClick={props.openDevices}>{t('rack.nav.devices')}</button>
          <button onClick={() => setListedSwitch('')}>{t('rack.close')}</button>
        </div>
        {listedDevices.length
          ? <DeviceInventoryTable rows={listedDevices} alerts={props.operations?.alerts} onNavigate={props.openPortMonitor} />
          : <p>{t('topology.noDevices')}</p>}
      </section>}
      <div
        ref={container}
        className="h-[600px] rounded bg-slate-800"
        style={background ? {
          backgroundImage: `url(${JSON.stringify(background).slice(1, -1)})`,
          backgroundSize: 'contain',
          backgroundRepeat: 'no-repeat',
          backgroundPosition: 'center',
        } : undefined}
      />
    </>
  );
}
