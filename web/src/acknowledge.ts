import { api, json } from './api';

const ACK_BATCH_SIZE = 4;

export async function acknowledgeEvents(ids: Iterable<number>, refresh: () => Promise<unknown>, onAcknowledged?: (ids: number[]) => void) {
  const targets = [...new Set(ids)];
  try {
    for (let offset = 0; offset < targets.length; offset += ACK_BATCH_SIZE) {
      const batch = targets.slice(offset, offset + ACK_BATCH_SIZE);
      const results = await Promise.allSettled(batch.map(id => api(`/api/events/${id}/ack`, json('POST'))));
      onAcknowledged?.(batch.filter((_, index) => results[index].status === 'fulfilled'));
      const failed = results.find(result => result.status === 'rejected');
      if (failed?.status === 'rejected') throw failed.reason;
    }
  } catch (error) {
    await refresh().catch(() => undefined);
    throw error;
  }
  await refresh();
}
