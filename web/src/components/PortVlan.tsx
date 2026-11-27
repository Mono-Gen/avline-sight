import { t } from '../i18n';
import type { Port, SwitchState } from '../switchTypes';
import { asArray } from '../format';

// A trunk whose LLDP neighbour is a NETGEAR switch is presumed to be set by Auto-Trunk.
// The switch does not report over SNMP whether a trunk was automatic, so this is never shown as certain.
export function isAutoTrunk(port: Pick<Port, 'mode' | 'if_index'>, state: Pick<SwitchState, 'neighbors'>) {
  return port.mode === 'trunk' && asArray(state.neighbors).some(n => n.local_if_index === port.if_index && n.is_netgear);
}

export function portModeLabel(port: Pick<Port, 'mode' | 'if_index'>, state: Pick<SwitchState, 'neighbors'>) {
  if (!port.mode) return '';
  if (isAutoTrunk(port, state)) return t('trunk.autoTrunk');
  return port.mode === 'trunk' ? t('trunk.trunk') : port.mode === 'access' ? t('trunk.access') : t('trunk.general');
}

export type PortMark = { mark: string; label: string };

// Colour-by-VLAN view of the rack. Colours repeat after eight VLANs, so the VLAN number is always written on the port
// (colour is never the only cue). Okabe-Ito based so the hues stay apart for common colour-vision deficiencies.
export const VLAN_PALETTE = ['#0072B2', '#E69F00', '#009E73', '#CC79A7', '#56B4E9', '#D55E00', '#F0E442', '#8C6BB1'];
export const vlanColor = (vlan: number) => VLAN_PALETTE[((vlan % VLAN_PALETTE.length) + VLAN_PALETTE.length) % VLAN_PALETTE.length];

// The VLAN a port belongs to for colouring. Ports carrying several VLANs (trunk, multi-VLAN general) get no single colour.
export function portVlanGroup(port: Pick<Port, 'mode' | 'pvid' | 'tagged_vlans' | 'untagged_vlans'>): { vlan?: number; multi: boolean } {
  const carried = [...new Set([...asArray(port.tagged_vlans), ...asArray(port.untagged_vlans)])];
  if (port.mode === 'trunk' || carried.length > 1) return { multi: true };
  const vlan = carried[0] ?? (port.pvid > 0 ? port.pvid : undefined);
  return { vlan, multi: false };
}

// Role marks shown with the port number in the front panel and rack view: letters, not colour alone.
// The M4250 reports ordinary device ports as General (seen on hardware 2026-11-27), so a General port carrying one VLAN is treated like Access and gets no mark.
export function portMarks(port: Pick<Port, 'mode' | 'oper' | 'stp_state' | 'tagged_vlans' | 'untagged_vlans' | 'lag_if_index' | 'lag_name'>): PortMark[] {
  const marks: PortMark[] = [];
  const carried = new Set([...asArray(port.tagged_vlans), ...asArray(port.untagged_vlans)]).size;
  if (port.mode === 'trunk') marks.push({ mark: 'T', label: t('trunk.markTrunk') });
  else if (port.mode === 'general' && carried > 1) marks.push({ mark: 'G', label: t('trunk.markGeneral') });
  if (port.lag_if_index) marks.push({ mark: 'L', label: t('trunk.markLagOf', { lag: port.lag_name || String(port.lag_if_index) }) });
  if (port.stp_state === 'discarding' && port.oper === 'up') marks.push({ mark: 'B', label: t('trunk.markBlocked') });
  return marks;
}

// For the legend (marks and their meaning).
export function portMarkLegend(): PortMark[] {
  return [{ mark: 'T', label: t('trunk.markTrunk') }, { mark: 'G', label: t('trunk.markGeneral') }, { mark: 'L', label: t('trunk.markLag') }, { mark: 'B', label: t('trunk.markBlocked') }];
}

const list = (vlans?: number[]) => asArray(vlans).length ? asArray(vlans).join(', ') : t('trunk.none');

// The VLAN section of the port detail: the mode and the VLANs actually carried (tagged and untagged).
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
