import { api, json } from '../api';
import { alertCount, asArray, decimal, formatUptime } from '../format';
import type { SwitchConfig, SwitchSummary } from '../types';
import type { ViewProps } from '../views/props';
import { Badge } from './Badge';
import { StatusIcon } from './StatusIcon';

export function SwitchCard({ summary: item, actions, onEdit, onRefresh }: {
  summary: SwitchSummary;
  actions: ViewProps;
  onEdit: (value: SwitchConfig | null) => void;
  onRefresh: () => Promise<void>;
}) {
  const budget = item.poe_budget_w || 0;
  const used = item.poe_used_w || 0;
  // 未取得・予算ゼロの装置で無限大にならないよう、バーだけ0〜100%へ制限する。
  const percent = budget > 0 ? Math.max(0, Math.min(100, used / budget * 100)) : 0;
  const openWeb = () => actions.run(async () => {
    const access = await api<{
      web_url: string;
    }>(`/api/switches/${item.id}/access-info`);
    const url = new URL(access.web_url);
    if (!asArray(['http:', 'https:']).includes(url.protocol)) throw new Error('URLが不正です。アクセス設定を確認してください。');
    window.open(url.href, '_blank', 'noopener,noreferrer');
  });
  return (
    <article className="panel space-y-2">
      <h2 className="text-xl font-bold">{item.reachable ? '🟢 接続中' : '⚪ 未接続'} {item.name}</h2>
      <p>{item.ip} · {item.group_tag} · {item.model} {item.fw}</p>
      {!item.reachable && <p role="status" className="text-red-600 dark:text-red-300">⚠ {item.last_error || 'スイッチに接続できません'}</p>}
      <p>ポート {item.ports_up}/{item.ports_total} · CPU {decimal(item.cpu_5s)}% · 温度 {decimal(item.temp_max_c)}℃</p>
      <p>Mbuf {decimal(item.mbuf_used_pct)}% · 空きメモリ {decimal(item.mem_free_pct)}%</p>
      <p>稼働時間: {formatUptime(item.uptime_sec)}</p>
      <p><StatusIcon ok={item.fans_ok} label="ファン" /> / <StatusIcon ok={item.psu_ok} label="電源" /></p>
      <div>
        <p>PoE {decimal(used)} / {decimal(budget)} W</p>
        <div
          role="progressbar"
          aria-label="PoE使用率"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={percent}
          className="h-2 overflow-hidden rounded bg-slate-300"
        >
          <div style={{ width: `${percent}%` }} className={`h-full ${percent >= 80 ? 'bg-amber-500' : 'bg-sky-500'}`} />
        </div>
      </div>
      <p><Badge text="CRITICAL" /> {alertCount(item.alerts, 'critical')} <Badge text="WARNING" /> {alertCount(item.alerts, 'warning')}</p>
      <div className="flex flex-wrap gap-1">
        <button onClick={() => actions.select(item.id)}>ポート</button>
        <button disabled={!!actions.replay || actions.busy} onClick={openWeb}>WebUI</button>
        <button disabled={actions.locked} onClick={() => actions.openTerminal(item.id)}>ターミナル</button>
        <button
          disabled={actions.locked}
          onClick={() => actions.run(async () => {
            const configs = await api<SwitchConfig[]>('/api/switches');
            onEdit(Array.isArray(configs) ? asArray(configs).find(config => config.id === item.id) || null : null);
          })}
        >編集</button>
        <button
          disabled={actions.locked || actions.busy}
          onClick={() => {
            if (confirm(`${item.name} を削除しますか？`)) actions.run(async () => {
              await api(`/api/switches/${item.id}`, json('DELETE'));
              await onRefresh();
            });
          }}
        >削除</button>
      </div>
    </article>
  );
}
