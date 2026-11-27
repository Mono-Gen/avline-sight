import { t } from '../i18n';
import type { Port, SwitchState } from '../switchTypes';
import { asArray } from '../format';

// Trunk で相手が LLDP で NETGEAR スイッチと分かるポートは、Auto-Trunk で自動設定されたものと推定する。
// スイッチは「自動で Trunk になったか」を SNMP で返さないため、確定表示はしない。
export function isAutoTrunk(port: Pick<Port, 'mode' | 'if_index'>, state: Pick<SwitchState, 'neighbors'>) {
  return port.mode === 'trunk' && asArray(state.neighbors).some(n => n.local_if_index === port.if_index && n.is_netgear);
}

export function portModeLabel(port: Pick<Port, 'mode' | 'if_index'>, state: Pick<SwitchState, 'neighbors'>) {
  if (!port.mode) return '';
  if (isAutoTrunk(port, state)) return t('trunk.autoTrunk');
  return port.mode === 'trunk' ? t('trunk.trunk') : port.mode === 'access' ? t('trunk.access') : t('trunk.general');
}

export type PortMark = { mark: string; label: string };

// 前面パネル・ラック図でポート番号に添える役割の記号。色だけに頼らず文字で示す。
// M4250 は普通の機器ポートもモードを General で返す（2026-11-27 実機）ため、VLAN を1つだけ運ぶ General は Access と同じ扱いで記号を付けない。
export function portMarks(port: Pick<Port, 'mode' | 'oper' | 'stp_state' | 'tagged_vlans' | 'untagged_vlans' | 'lag_if_index' | 'lag_name'>): PortMark[] {
  const marks: PortMark[] = [];
  const carried = new Set([...asArray(port.tagged_vlans), ...asArray(port.untagged_vlans)]).size;
  if (port.mode === 'trunk') marks.push({ mark: 'T', label: t('trunk.markTrunk') });
  else if (port.mode === 'general' && carried > 1) marks.push({ mark: 'G', label: t('trunk.markGeneral') });
  if (port.lag_if_index) marks.push({ mark: 'L', label: t('trunk.markLagOf', { lag: port.lag_name || String(port.lag_if_index) }) });
  if (port.stp_state === 'discarding' && port.oper === 'up') marks.push({ mark: 'B', label: t('trunk.markBlocked') });
  return marks;
}

// 凡例用（記号と意味の一覧）
export function portMarkLegend(): PortMark[] {
  return [{ mark: 'T', label: t('trunk.markTrunk') }, { mark: 'G', label: t('trunk.markGeneral') }, { mark: 'L', label: t('trunk.markLag') }, { mark: 'B', label: t('trunk.markBlocked') }];
}

const list = (vlans?: number[]) => asArray(vlans).length ? asArray(vlans).join(', ') : t('trunk.none');

// ポート詳細の「VLAN」欄。モードと、実際に流れている VLAN（タグ付き・タグなし）を並べる。
export function PortVlan({ port, state }: { port: Port; state: SwitchState }) {
  if (!port.mode && !asArray(port.tagged_vlans).length && !asArray(port.untagged_vlans).length) return null;
  const auto = isAutoTrunk(port, state);
  return <section className="detail-section"><h4>VLAN</h4>
    <dl className="detail-kv">
      {port.mode && <><dt>{t('trunk.mode')}</dt><dd title={auto ? t('trunk.autoTrunkHelp') : undefined}>{portModeLabel(port, state)}</dd></>}
      {port.mode === 'trunk' && !!port.native_vlan && <><dt>{t('trunk.native')}</dt><dd>{port.native_vlan}</dd></>}
      <dt>{t('trunk.tagged')}</dt><dd>{list(port.tagged_vlans)}</dd>
      <dt>{t('trunk.untagged')}</dt><dd>{list(port.untagged_vlans)}</dd>
    </dl>
    {auto && <p className="chart-help">{t('trunk.autoTrunkHelp')}</p>}
  </section>;
}
