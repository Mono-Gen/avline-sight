import { api, json } from './api';

const ACK_BATCH_SIZE = 4;

export async function acknowledgeEvents(ids: Iterable<number>, refresh: () => Promise<unknown>, onAcknowledged?: (ids: number[]) => void) {
  return changeAcknowledgement(ids, true, refresh, onAcknowledged);
}

export async function unacknowledgeEvents(ids: Iterable<number>, refresh: () => Promise<unknown>, onChanged?: (ids: number[]) => void) {
  return changeAcknowledgement(ids, false, refresh, onChanged);
}

async function changeAcknowledgement(ids: Iterable<number>, acked: boolean, refresh: () => Promise<unknown>, onChanged?: (ids: number[]) => void) {
  const targets = [...new Set(ids)];
  try {
    for (let offset = 0; offset < targets.length; offset += ACK_BATCH_SIZE) {
      const batch = targets.slice(offset, offset + ACK_BATCH_SIZE);
      const results = await Promise.allSettled(batch.map(id => api(`/api/events/${id}/${acked ? 'ack' : 'unack'}`, json('POST'))));
      onChanged?.(batch.filter((_, index) => results[index].status === 'fulfilled'));
      const failed = results.find(result => result.status === 'rejected');
      if (failed?.status === 'rejected') throw failed.reason;
    }
  } catch (error) {
    await refresh().catch(refreshError => console.warn('Could not refresh acknowledgement state', refreshError));
    throw error;
  }
  await refresh();
}
