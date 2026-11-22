import { useCallback, useEffect, useRef, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { api, json } from './api';
import { asArray } from './format';
import { t } from './i18n';
import type { Alert, Event, Phase, Status, SwitchGroup } from './types';
import type { LiveMessage } from './ws';
import { usePolling } from './usePolling';

export function useOperations({ online, replay, profileId, setStatus, run }: {
  online: boolean; replay: boolean; profileId?: number;
  setStatus: Dispatch<SetStateAction<Status | undefined>>;
  run: (action: () => Promise<void>) => void;
}) {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [groups, setGroups] = useState<SwitchGroup[]>([]);
  const [phase, setPhase] = useState<Phase>('show');
  const [toast, setToast] = useState('');
  const previous = useRef<Alert[] | undefined>();
  const localAcks = useRef(new Set<number>());
  const revision = useRef(0);
  const hydrate = useCallback((items: Alert[]) => {
    const next = asArray(items);
    if (previous.current?.some(old => !old.acked && !localAcks.current.has(old.event_id) &&
      (!next.some(item => item.event_id === old.event_id) || next.some(item => item.event_id === old.event_id && item.acked)))) {
      setToast(t('rack.remoteAck'));
    }
    previous.current = next;
    for (const id of localAcks.current) if (!next.some(item => item.event_id === id && !item.acked)) localAcks.current.delete(id);
    setAlerts(next);
  }, []);
  const feed = usePolling(async signal => {
    const version = revision.current;
    const [a, g, p, s] = await Promise.all([
      api<Alert[]>('/api/alerts', { signal }), api<SwitchGroup[]>('/api/groups', { signal }),
      api<{ phase: Phase }>('/api/phase', { signal }), api<Status>('/api/status', { signal }),
    ]);
    if (version === revision.current && !signal.aborted) {
      hydrate(a); setGroups(asArray(g)); setPhase(p.phase); setStatus(s);
    }
    return true;
  }, online ? 0 : 5000, [online, profileId], !replay && profileId !== undefined);
  useEffect(() => {
    previous.current = undefined; localAcks.current.clear(); setAlerts([]); setGroups([]);
  }, [profileId, replay]);
  useEffect(() => {
    if (replay) return;
    const handler = (event: globalThis.Event) => {
      const item = (event as CustomEvent<LiveMessage>).detail;
      revision.current++;
      if (item.type === 'alerts_changed') hydrate(item.data);
      if (item.type === 'work_changed') setGroups(asArray(item.data));
      if (item.type === 'phase_changed') {
        setPhase(item.data.phase); setStatus(old => old ? { ...old, phase: item.data.phase } : old);
      }
      if (item.type === 'lock_changed') setStatus(old => old ? { ...old, ...item.data } : old);
    };
    window.addEventListener('live-control', handler);
    return () => window.removeEventListener('live-control', handler);
  }, [hydrate, replay, setStatus]);
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(''), 4500);
    return () => clearTimeout(timer);
  }, [toast]);
  const ack = (id?: number) => {
    if (id === undefined && !window.confirm(t('rack.ackAllConfirm'))) return;
    const ids = id === undefined ? alerts.filter(alert => !alert.acked).map(alert => alert.event_id) : [id];
    ids.forEach(value => localAcks.current.add(value));
    run(async () => {
      try {
        await api(id === undefined ? '/api/events/ack-all' : `/api/events/${id}/ack`, json('POST'));
        hydrate(await api<Alert[]>('/api/alerts'));
      } finally { ids.forEach(value => localAcks.current.delete(value)); }
    });
  };
  return { alerts, groups, phase, toast, ack, refresh: feed.refresh, feed };
}

export function useAlertHistory(profileId?: number, replay = false) {
  return usePolling(signal => api<Event[]>('/api/events?limit=2000', { signal }), 15000, [profileId], !replay);
}
