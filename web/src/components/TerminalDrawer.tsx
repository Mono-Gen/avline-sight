import { t } from '../i18n';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import { useEffect, useRef, useState } from 'react';
import { download, wsUrl } from '../api';
import { asArray } from '../format';
const commands = [
  'show igmpsnooping',
  'show igmpsnooping querier',
  'show igmpsnooping mrouter',
  'show poe status',
  'show poe port info all',
  'show fiber-ports optical-transceiver all',
  'show logging buffered',
  'show mac-addr-table',
  'show lldp remote-device all',
  'show running-config'
];
export function TerminalDrawer({ id, close }: {
  id: number;
  close: () => void;
}) {
  const [user, setUser] = useState('');
  const [password, setPassword] = useState('');
  const [proto, setProto] = useState('ssh');
  const [active, setActive] = useState(false);
  const [error, setError] = useState('');
  const host = useRef<HTMLDivElement>(null);
  const socket = useRef<WebSocket>();
  const output = useRef('');
  useEffect(() => {
    if (!active || !host.current) return;
    const term = new Terminal({ scrollback: 5000 });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host.current);
    fit.fit();
    const ws = new WebSocket(wsUrl(`/ws/terminal/${id}?proto=${proto}`));
    socket.current = ws;
    ws.onopen = () => {
      ws.send(JSON.stringify({
        type: 'auth',
        user: proto === 'ssh' ? user : '',
        password: proto === 'ssh' ? password : '',
        cols: term.cols,
        rows: term.rows
      }));
      setPassword('');
    };
    ws.onmessage = e => {
      const s = String(e.data);
      term.write(s);
      output.current = (output.current + s).slice(-1000000);
    };
    ws.onclose = e => {
      term.writeln(t('ui.m197', { p0: e.reason || t('ui.m196') }));
      socket.current = undefined;
    };
    ws.onerror = () => setError(t('ui.m198'));
    const input = term.onData(data => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({
        type: 'data',
        data
      }));
    });
    const resize = () => {
      fit.fit();
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({
        type: 'resize',
        cols: term.cols,
        rows: term.rows
      }));
    };
    window.addEventListener('resize', resize);
    return () => {
      window.removeEventListener('resize', resize);
      input.dispose();
      ws.close();
      term.dispose();
    };
  }, [active, id, proto]);
  return <div
    className="fixed inset-x-0 bottom-0 z-40 max-h-[75vh] overflow-auto bg-slate-900 p-3 text-white shadow-xl"
  >
    <p className="bg-amber-200 p-2 text-black">{t('ui.m199')}</p>
    <button onClick={close}>{t('ui.m117')}</button>

    <button
      onClick={() => download(new Blob([output.current], { type: 'text/plain;charset=utf-8' }), 'terminal.txt')}
    >{t('ui.m200')}</button>
    {!active && <form onSubmit={e => {
      e.preventDefault();
      setActive(true);
    }}>
      <select value={proto} onChange={e => setProto(e.target.value)}>
        <option>ssh
        </option>
        <option>telnet
        </option>
      </select>
      {/* Telnet logs in at the switch's own prompt (the server ignores these), so the fields are for SSH only. */}
      {proto === 'ssh' && <>
        <input placeholder={t('ui.m201')} value={user} onChange={e => setUser(e.target.value)} />
        <input type="password" placeholder={t('ui.m202')} value={password} onChange={e => setPassword(e.target.value)} />
      </>}
      <button>{t('ui.m203')}</button>
    </form>}
    {error && <p role="alert">
      {error}
    </p>}
    {active && <>
      <div className="flex flex-wrap gap-1">
        {asArray(commands).map(c => <button
          key={c}
          onClick={() => socket.current?.send(JSON.stringify({
            type: 'data',
            data: c + '\r'
          }))}
        >
          {c}
        </button>)}
      </div>
      <div ref={host} className="h-64" />
    </>}
  </div>;
}
