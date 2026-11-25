import { asArray } from '../format';
import { t } from '../i18n';

type VlanRow = { vlan: number; name?: string; snooping: boolean; fast_leave: boolean; querier_oper?: string; last_querier?: string };
// config_settings holds a value only when the setting was actually read (absent after a failed read), so it is the source here.
type PortRow = { name: string; type?: string; oper?: string; pvid: number; max_frame?: number; alias?: string; config_settings?: Record<string, string> | null };

// The text a port description holds. The AV UI writes "VID:<vlan>,<name>,<template>" there, but anyone can edit it,
// so it is shown as written text, never as the profile in use (the switch does not store the profile itself).
export function writtenText(alias: string | undefined) {
  const text = (alias || '').trim();
  const match = /^VID:\d+,[^,]*,(.+)$/.exec(text);
  return match ? match[1].trim() : text;
}

const realAddress = (address?: string) => !!address && address !== '0.0.0.0' && address !== '::';

function querierText(v: VlanRow) {
  if (v.querier_oper === 'querier') return t('vlan.querierSelf');
  if (realAddress(v.last_querier)) return t('vlan.querierOther', { address: v.last_querier });
  // Waiting as non-querier without a real address: whether another querier exists is not known from this switch.
  if (v.querier_oper === 'non-querier') return t('vlan.querierUnknown');
  return t('vlan.querierNone');
}

// "a" when every port has the same value, otherwise every value with its count ("1500 ×2 / 9198 ×4").
function sameOrMixed(values: string[]) {
  const counts = new Map<string, number>();
  for (const value of values) counts.set(value, (counts.get(value) || 0) + 1);
  if (counts.size <= 1) return values[0] ?? '';
  return [...counts].sort((a, b) => b[1] - a[1]).map(([value, count]) => `${value} ×${count}`).join(' / ');
}

// One row per VLAN of this switch: the settings read over SNMP (facts), and the text written in the port descriptions.
// No judgement or warning here: what is right depends on the site, and the profile in use cannot be known for sure.
export function VlanFacts({ vlans, ports, globalSnoopingOff = false }: { vlans: readonly VlanRow[]; ports: readonly PortRow[]; globalSnoopingOff?: boolean }) {
  const access = asArray(ports as PortRow[]).filter(port => port.type !== 'lag');
  const ids = [...new Set([...asArray(vlans as VlanRow[]).map(v => v.vlan), ...access.map(port => port.pvid)])].filter(id => id > 0).sort((a, b) => a - b);
  if (!ids.length) return null;
  return <section className="panel focus-card vlan-facts" aria-label={t('vlan.title')}>
    <h3>{t('vlan.title')}</h3>
    <ul>{ids.map(id => {
      const v = asArray(vlans as VlanRow[]).find(row => row.vlan === id);
      const members = access.filter(port => port.pvid === id);
      const linked = members.filter(port => port.oper === 'up').length;
      const written = sameOrMixed(members.map(port => writtenText(port.alias)).filter(Boolean));
      const facts = [
        // A VLAN set to snoop has no effect while snooping is off for the whole switch; say both facts.
        v ? `${t('vlan.snooping')} ${t(v.snooping ? 'vlan.on' : 'vlan.off')}${v.snooping && globalSnoopingOff ? t('vlan.globalOff') : ''}` : '',
        v ? `${t('vlan.querier')} ${querierText(v)}` : '',
        v ? `${t('vlan.fastLeave')} ${t(v.fast_leave ? 'vlan.on' : 'vlan.off')}` : '',
        members.some(port => port.max_frame) ? `${t('vlan.frameSize')} ${sameOrMixed(members.filter(port => port.max_frame).map(port => String(port.max_frame)))}` : '',
        members.some(port => port.config_settings?.bcast_storm_ctl)
          ? `${t('vlan.broadcastControl')} ${sameOrMixed(members.filter(port => port.config_settings?.bcast_storm_ctl)
            .map(port => t(port.config_settings?.bcast_storm_ctl === 'enabled' ? 'vlan.on' : 'vlan.off')))}` : '',
      ].filter(Boolean);
      return <li key={id}>
        <div className="vlan-facts-head"><strong>VLAN {id}</strong>{v?.name && <span>{v.name}</span>}
          <span className="vlan-facts-ports">{t('vlan.ports', { count: members.length, linked })}</span></div>
        {facts.length > 0 && <div className="vlan-facts-row"><span className="vlan-facts-label">{t('vlan.settings')}</span>{facts.join(' · ')}</div>}
        {written && <div className="vlan-facts-row vlan-facts-written" title={t('vlan.writtenHelp')}>
          <span className="vlan-facts-label">{t('vlan.written')}</span><q>{written}</q></div>}
      </li>;
    })}</ul>
  </section>;
}
