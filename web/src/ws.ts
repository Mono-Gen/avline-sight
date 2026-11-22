import { wsUrl } from './api';
import type { Alert, Event, Overview, Phase, SwitchState } from './types';

export type LiveMessage =
  | { type: 'alerts_changed'; data: Alert[] }
  | { type: 'phase_changed'; data: { phase: Phase } }
  | { type: 'groups_changed'; data: null }
  | { type: 'lock_changed'; data: { locked: boolean; pin_set: boolean } }
  | {
    type: 'overview_tick';
    data: Overview;
  }
  | {
    type: 'event_pushed';
    data: Event;
  }
  | {
    type: 'switch_state';
    data: SwitchState;
  };

export class LiveSocket {
  private socket?: WebSocket;
  private timer = 0;
  private closed = false;
  private id = 0;

  constructor(
    private onMessage: (message: LiveMessage) => void,
    private onStatus: (connected: boolean) => void,
  ) {
    this.connect();
  }

  private connect() {
    if (this.closed) return;
    const socket = new WebSocket(wsUrl('/ws/live'));
    this.socket = socket;
    socket.onopen = () => {
      this.onStatus(true);
      this.subscribe(this.id);
    };
    socket.onmessage = event => {
      try {
        this.onMessage(JSON.parse(String(event.data)) as LiveMessage);
      } catch {
        this.onStatus(false);
        socket.close();
      }
    };
    socket.onerror = () => { this.onStatus(false); socket.close(); };
    socket.onclose = () => {
      this.onStatus(false);
      if (!this.closed) this.timer = window.setTimeout(() => this.connect(), 3000);
    };
  }

  subscribe(id: number) {
    this.id = id;
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify({ subscribe: id }));
    }
  }

  close() {
    this.closed = true;
    clearTimeout(this.timer);
    this.socket?.close();
  }
}
