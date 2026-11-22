import { useEffect, useRef } from 'react';
import { message } from './api';
import { eventTitle, t } from './i18n';
import { newIssueAlerts } from './monitorHealth';
import type { Alert } from './types';

export function useDesktopNotifications({ enabled, alerts, profileId, ready, paused }: {
  enabled: boolean; alerts: Alert[]; profileId?: number; ready: boolean; paused: boolean;
}) {
  const previous = useRef<{ profileId?: number; eligible: boolean; ids: Set<number> }>();
  const displayed = useRef<Notification>();
  useEffect(() => {
    const before = previous.current;
    const eligible = enabled && ready && !paused;
    const allIds = new Set(alerts.map(alert => alert.event_id));
    const baseline = !eligible || !before?.eligible || before.profileId !== profileId;
    // Keep notified IDs through suppression, but never beyond the current alert collection.
    const ids = baseline ? allIds : new Set([...before.ids].filter(id => allIds.has(id)));
    previous.current = { profileId, eligible, ids };
    if (baseline ||
      typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
    const added = newIssueAlerts(alerts, ids);
    if (!added.length) return;
    added.forEach(alert => ids.add(alert.event_id));
    try {
      displayed.current?.close();
      displayed.current = new Notification('AVLine-Sight', {
        body: added.length === 1 ? [added[0].switch_name, added[0].port, added[0].device_name, eventTitle(added[0])].filter(Boolean).join(' · ')
          : t('cleanup.notificationBody', { count: added.length }),
        tag: 'avline-sight-alert', silent: true,
      });
    } catch (reason) {
      window.dispatchEvent(new CustomEvent('api-error', { detail: message(reason) }));
    }
  }, [enabled, alerts, profileId, ready, paused]);
  useEffect(() => () => displayed.current?.close(), []);
}
