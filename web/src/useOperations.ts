import { useCallback, useEffect, useRef, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { api } from './api';
import { acknowledgeEvents, unacknowledgeEvents } from './acknowledge';
import { asArray } from './format';
import { t } from './i18n';
import type { Alert, Event, Phase, Status, SwitchGroup } from './types';
import type { LiveMessage } from './ws';
import { usePolling } from './usePolling';

export const ACK_UNDO_MS = 5000;

export function useOperations({ online, replay, profileId, setStatus, run }: {
  online: boolean; replay: boolean; profileId?: number;
  setStatus: Dispatch<SetStateAction<Status | undefined>>;
  run: (action: () => Promise<void>) => void;
}) {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [groups, setGroups] = useState<SwitchGroup[]>([]);
  const [phase, setPhase] = useState<Phase>();
  const [toast, setToast] = useState(0);
  const [undo, setUndo] = useState<{ ids: number[]; until: number }>();
  const [disappeared, setDisappeared] = useState<Alert[]>([]);
  const previous = useRef<Alert[] | undefined>();
  const localAcks = useRef(new Map<number, number>());
  const scope = useRef(profileId);
  scope.current = profileId;
  const revision = useRef({ alerts: 0, groups: 0, phase: 0, lock: 0 });
  const hydrate = useCallback((items: Alert[]) => {
    const next = asArray(items);
    const now = Date.now();
    for (const [id, until] of localAcks.current) {
      if (until <= now) localAcks.current.delete(id);
    }
    const byId = new Map(next.map(item => [item.event_id, item]));
    const changed = asArray(previous.current).filter(old => !old.acked && !localAcks.current.has(old.event_id));
    if (changed.some(old => byId.get(old.event_id)?.acked)) setToast(Date.now());
    const removed = changed.filter(old => !byId.has(old.event_id));
    if (removed.length) setDisappeared(removed);
    previous.current = next;
    setAlerts(next);
  }, []);
  const removedCheck = usePolling(async signal => {
    const from = disappeared.reduce((min, item) => item.ts < min ? item.ts : min, disappeared[0].ts);
    const to = disappeared.reduce((max, item) => item.ts > max ? item.ts : max, disappeared[0].ts);
    const ids = new Set(disappeared.map(item => item.event_id));
    const query = new URLSearchParams({ from, to, limit: '10000' });
    const events = await api<Event[]>(`/api/events?${query}`, { signal });
    return asArray(events).some(event => ids.has(event.id) && event.acked && !localAcks.current.has(event.id));
  }, 0, [disappeared], disappeared.length > 0 && !replay);
  useEffect(() => { if (removedCheck.data) setToast(Date.now()); }, [removedCheck.data, removedCheck.updatedAt]);
  const feed = usePolling(async signal => {
    const version = { ...revision.current };
    const [a, g, p, s] = await Promise.all([
      api<Alert[]>('/api/alerts', { signal }), api<SwitchGroup[]>('/api/groups', { signal }),
      api<{ phase: Phase }>('/api/phase', { signal }), api<Status>('/api/status', { signal }),
    ]);
    if (!signal.aborted) {
      if (version.alerts === revision.current.alerts) hydrate(a);
      if (version.groups === revision.current.groups) setGroups(asArray(g));
      if (version.phase === revision.current.phase) setPhase(p.phase);
      setStatus(old => version.lock === revision.current.lock || !old ? s : { ...s, locked: old.locked, pin_set: old.pin_set });
    }
    return a;
  }, online ? 15000 : 5000, [online, profileId], !replay && profileId !== undefined);
  const alertKey = JSON.stringify(alerts.map(item => [item.event_id, item.acked]));
  const displayDetails = usePolling(signal => api<Alert[]>('/api/alerts', { signal }), 0, [alertKey, profileId],
    !replay && alerts.some(item => !item.detail || item.acked && !item.acked_at), 150);
  useEffect(() => {
    previous.current = undefined;
    localAcks.current.clear();
    setAlerts([]);
    setGroups([]);
    setDisappeared([]);
    setToast(0);
    setUndo(undefined);
  }, [profileId, replay]);
  useEffect(() => {
    if (replay) return;
    const handler = (event: globalThis.Event) => {
      const item = (event as CustomEvent<LiveMessage>).detail;
      if (item.type === 'alerts_changed') {
        revision.current.alerts++;
        const old = new Map(asArray(previous.current).map(alert => [alert.event_id, alert]));
        hydrate(asArray(item.data).map(alert => ({ ...old.get(alert.event_id), ...alert })));
      }
      if (item.type === 'groups_changed') {
        revision.current.groups++;
        void feed.refresh().catch(() => undefined);
      }
      if (item.type === 'phase_changed') {
        revision.current.phase++;
        setPhase(item.data.phase); setStatus(old => old ? { ...old, phase: item.data.phase } : old);
      }
      if (item.type === 'lock_changed') {
        revision.current.lock++;
        setStatus(old => old ? { ...old, ...item.data } : old);
      }
    };
    window.addEventListener('live-control', handler);
    return () => window.removeEventListener('live-control', handler);
  }, [hydrate, replay, setStatus, feed.refresh]);
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(0), 4500);
    return () => clearTimeout(timer);
  }, [toast]);
  useEffect(() => {
    if (!undo) return;
    const timer = setTimeout(() => setUndo(undefined), Math.max(0, undo.until - Date.now()));
    return () => clearTimeout(timer);
  }, [undo]);
  const refreshAlerts = async () => {
    const version = revision.current.alerts;
    const fresh = await api<Alert[]>('/api/alerts');
    if (scope.current === profileId && version === revision.current.alerts) hydrate(fresh);
  };
  const ackMany = (targets: number[]) => {
    const ids = [...new Set(targets)].filter(id => alerts.some(alert => alert.event_id === id && !alert.acked && !alert.suppressed));
    if (!ids.length || replay) return;
    run(async () => {
      const succeeded: number[] = [];
      ids.forEach(id => localAcks.current.set(id, Infinity));
      setUndo(undefined);
      try {
        await acknowledgeEvents(ids, refreshAlerts, changed => succeeded.push(...changed));
      } finally {
        ids.forEach(id => localAcks.current.set(id, Date.now() + ACK_UNDO_MS));
        if (succeeded.length && scope.current === profileId) setUndo({ ids: succeeded, until: Date.now() + ACK_UNDO_MS });
      }
    });
  };
  const undoAck = () => {
    if (!undo || Date.now() >= undo.until || replay) return;
    const ids = undo.ids;
    run(async () => {
      await unacknowledgeEvents(ids, refreshAlerts, changed => {
          if (scope.current !== profileId) return;
          changed.forEach(id => localAcks.current.delete(id));
          setUndo(current => current ? { ...current, ids: current.ids.filter(id => !changed.includes(id)) } : current);
      });
    });
  };
  const ack = (id?: number) => confirmAck(id, run, async () => {
    const ids = id === undefined ? alerts.filter(alert => !alert.acked).map(alert => alert.event_id) : [id];
    ids.forEach(value => localAcks.current.set(value, Infinity));
    try {
      await acknowledgeEvents(ids, async () => {
        const version = revision.current.alerts;
        const fresh = await api<Alert[]>('/api/alerts');
        if (version === revision.current.alerts) hydrate(fresh);
      });
    } finally {
      ids.forEach(value => localAcks.current.set(value, Date.now() + 5000));
    }
  });
  const details = new Map(asArray(displayDetails.data).map(item => [item.event_id, item]));
  const withDetails = alerts.map(item => {
    const info = details.get(item.event_id);
    return { ...item, detail: item.detail || info?.detail, hint: item.hint || info?.hint, acked_at: item.acked_at || info?.acked_at };
  });
  return { alerts: withDetails, groups, phase, setPhase, toast: toast ? t('rack.remoteAck') : '', ack, ackMany, undoAck,
    undoCount: undo?.ids.length || 0, refresh: feed.refresh, ready: !!feed.updatedAt };
}

export function confirmAck(id: number | undefined, run: (action: () => Promise<void>) => void, action: () => Promise<void>) {
  if (id !== undefined || window.confirm(t('rack.ackAllConfirm'))) run(action);
}

export type Operations = ReturnType<typeof useOperations>;
