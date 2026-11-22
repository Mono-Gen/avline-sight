import { api } from './api';
import { asArray } from './format';
import type { InventoryRow, ModelDefinitions, Port, ReplayRef, SwitchState, Topology } from './types';
import { usePolling } from './usePolling';

export interface RackData {
  ports: { switch_id: number; port: Port }[];
  devices: InventoryRow[];
  topology?: Topology;
  definitions: ModelDefinitions;
}
export function useRackData(profileId: number | undefined, replay: SwitchState[] | null, replayRef?: ReplayRef) {
  const models = usePolling(signal => api<ModelDefinitions>('/api/models', { signal }), 0, []);
  return usePolling(async signal => {
    if (replay !== null) {
      const devices = replayRef ? await api<InventoryRow[]>(`/api/replay/${encodeURIComponent(replayRef.id)}/devices?ts=${encodeURIComponent(replayRef.ts)}`, { signal }) : [];
      return { ports: asArray(replay).flatMap(state => asArray(state.ports).map(port => ({ switch_id: state.id, port }))), devices: asArray(devices), definitions: models.data || {} } as RackData;
    }
    const [ports, devices, topology] = await Promise.all([
      api<RackData['ports']>('/api/ports/search', { signal }),
      api<InventoryRow[]>('/api/devices?scope=all', { signal }),
      api<Topology>('/api/topology', { signal }),
    ]);
    return { ports: asArray(ports), devices: asArray(devices), topology, definitions: models.data || {} } as RackData;
  }, replay !== null ? 0 : 5000, [profileId, replayRef?.id, replayRef?.ts, replay, models.data], true);
}
