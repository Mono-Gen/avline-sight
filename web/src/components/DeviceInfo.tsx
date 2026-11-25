import { t } from '../i18n';

// Boot version is the last comma-separated field of sysDescr on NETGEAR M4250
// ("M4250-26G4F-PoE+ ..., 13.0.7.6, 1.0.0.13"). Shown only when it looks like a version, never guessed otherwise.
export function bootVersion(sysDescr: string | undefined) {
  const parts = (sysDescr || '').split(',').map(part => part.trim());
  const last = parts.length >= 3 ? parts[parts.length - 1] : '';
  return /^\d+(\.\d+)+$/.test(last) ? last : '';
}

export function uptimeText(seconds: number | undefined) {
  if (!seconds || seconds < 0) return '';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor(seconds % 86400 / 3600);
  const m = Math.floor(seconds % 3600 / 60);
  return t('device.uptimeValue', { d, h, m });
}

type Peer = { id: number; name: string; model?: string; fw?: string };

// Facts about the switch body only. A firmware that differs from other switches of the same model is listed as a fact,
// never as "update recommended": a site may run a version on purpose.
export function DeviceInfo({ device, peers }: {
  device: { id: number; name: string; ip?: string; sys_name?: string; model?: string; fw?: string; serial?: string; base_mac?: string; uptime_sec?: number; sys_descr?: string };
  peers: readonly Peer[];
}) {
  const others = peers.filter(peer => peer.id !== device.id && !!device.model && peer.model === device.model && !!peer.fw && peer.fw !== device.fw);
  const rows: [string, string][] = [
    [t('device.model'), device.model || ''],
    [t('device.serial'), device.serial || ''],
    [t('device.firmware'), device.fw || ''],
    [t('device.boot'), bootVersion(device.sys_descr)],
    [t('device.baseMac'), device.base_mac || ''],
    [t('device.ip'), device.ip || ''],
    [t('device.sysName'), device.sys_name || ''],
    [t('device.uptime'), uptimeText(device.uptime_sec)],
  ];
  return <details className="device-info">
    {/* The firmware difference is visible while folded too; worded as a fact, not as advice. */}
    <summary>{t('device.title')}{others.length > 0 && <span className="device-info-flag">{t('device.otherFirmwareShort')}</span>}</summary>
    <dl className="detail-kv">
      {rows.filter(([, value]) => value).flatMap(([label, value]) => [<dt key={`${label}-label`}>{label}</dt>, <dd key={`${label}-value`}>{value}</dd>])}
    </dl>
    {others.length > 0 && <p className="device-info-peers">
      {t('device.otherFirmware', { list: others.map(peer => t('device.peerFirmware', { name: peer.name, fw: peer.fw })).join(t('device.listSeparator')) })}
    </p>}
  </details>;
}
