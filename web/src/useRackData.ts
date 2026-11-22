import { api } from './api';
import { asArray } from './format';
import type { InventoryRow, ModelDefinitions, Port, ReplayRef, SwitchConfig, SwitchState, Topology } from './types';
import { usePolling } from './usePolling';

export interface RackData {
  ports: { switch_id: number; port: Port }[];
  devices: InventoryRow[];
  configs?: SwitchConfig[];
}
export function useRackData(profileId: number | undefined, replay: SwitchState[] | null, replayRef?: ReplayRef) {
  const models = usePolling(signal => api<ModelDefinitions>('/api/models', { signal }), 0, []);
  const topology = usePolling(signal => api<Topology>('/api/topology', { signal }), 15000, [profileId], replay === null);
  const data = usePolling(async (signal): Promise<RackData> => {
    if (replay !== null) {
      const devices = replayRef ? await api<InventoryRow[]>(`/api/replay/${encodeURIComponent(replayRef.id)}/devices?ts=${encodeURIComponent(replayRef.ts)}`, { signal }) : [];
      return { ports: asArray(replay).flatMap(state => asArray(state.ports).map(port => ({ switch_id: state.id, port }))), devices: asArray(devices) };
    }
    const [ports, devices, configs] = await Promise.all([
      api<RackData['ports']>('/api/ports/search', { signal }),
      api<InventoryRow[]>('/api/devices?scope=all&include=registration', { signal }),
      api<SwitchConfig[]>('/api/switches', { signal }),
    ]);
    return { ports: asArray(ports), devices: asArray(devices), configs: asArray(configs) };
  }, replay !== null ? 0 : 5000, [profileId, replayRef?.id, replayRef?.ts, replay]);
  return { ...data, definitions: models.data || {}, topology: replay !== null ? undefined : topology.data };
}
