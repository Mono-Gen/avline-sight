import { t } from '../i18n';
import cytoscape from 'cytoscape';
import { useEffect, useRef, useState } from 'react';
import { api, json, upload } from '../api';
import { WatchBar } from '../components/Dialogs';
import { PollingStatus } from '../components/PollingStatus';
import { asArray } from '../format';
import { topologyStyles, updateTopology } from '../topologyGraph';
import type { Topology as Graph, Multicast } from '../types';
import { TOPOLOGY_REFRESH_MS, usePolling } from '../usePolling';
import type { ViewProps } from './props';

export function Topology(props: ViewProps) {
  const [group, setGroup] = useState('');
  const [blocking, setBlocking] = useState(true);
  const container = useRef<HTMLDivElement>(null);
  const graph = useRef<cytoscape.Core>();
  const initialized = useRef(false);
  const onSelect = useRef(props.select);
  onSelect.current = props.select;
  const result = usePolling(async signal => {
    const [topology, multicast] = await Promise.all([
      api<Graph>('/api/topology', { signal }),
      api<Multicast>('/api/multicast', { signal }),
    ]);
    return { topology, multicast };
  }, TOPOLOGY_REFRESH_MS, [], !props.replay);

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
      const id = event.target.data('switch_id');
      if (id) onSelect.current(id);
    });
    return () => {
      instance.destroy();
      graph.current = undefined;
      initialized.current = false;
    };
  }, []);
  useEffect(() => {
    if (!graph.current || !result.data) return;
    updateTopology(graph.current, result.data.topology, result.data.multicast, group, blocking, !initialized.current);
    if (asArray(result.data.topology.nodes).length) initialized.current = true;
  }, [result.data, group, blocking]);

  const background = result.data?.topology.background;
  return (
    <>
      <WatchBar items={props.watch} onChange={props.setWatch} run={props.run} disabled={props.locked || props.busy} />
      <PollingStatus {...result} />
      <div className="flex flex-wrap gap-2">
        <label><input type="checkbox" checked={blocking} onChange={event => setBlocking(event.target.checked)} /> {t('ui.m302')}</label>
        <select value={group} onChange={event => setGroup(event.target.value)}>
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
            graph.current?.nodes().forEach(node => { positions[node.id()] = node.position(); });
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
      <p>{t('ui.m306')}</p>
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
