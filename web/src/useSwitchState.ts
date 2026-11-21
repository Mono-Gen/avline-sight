import { api } from './api';
import { asArray } from './format';
import type { SwitchState } from './types';
import { STATE_REFRESH_MS, usePolling } from './usePolling';

export function useSwitchState(id: number, live: SwitchState | undefined, replay: SwitchState[] | null) {
  const polling = usePolling(
    signal => api<SwitchState>(`/api/switches/${id}/state`, { signal }),
    STATE_REFRESH_MS, [id], !replay && id > 0,
  );
  const polled = polling.data?.id === id ? polling.data : undefined;
  const currentLive = live?.id === id ? live : undefined;
  // WSと定期取得の到着順が入れ替わっても、採取時刻の新しい状態を表示する。
  const state = replay ? asArray(replay).find(item => item.id === id)
    : currentLive && (!polled || Date.parse(currentLive.last_poll) >= Date.parse(polled.last_poll))
      ? currentLive : polled;
  return { state, polling };
}
