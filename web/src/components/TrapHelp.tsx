import { useState } from 'react';
import { t } from '../i18n';
import { Dialog } from './Dialog';

const commands = {
  check: 'Get-NetUDPEndpoint -LocalPort 162 -ErrorAction SilentlyContinue | Select-Object LocalAddress, OwningProcess\nGet-Service SNMPTRAP -ErrorAction SilentlyContinue',
  stop: 'Stop-Service SNMPTRAP; Set-Service SNMPTRAP -StartupType Disabled',
  bind: '.\\avline-sight.exe --trap 0.0.0.0:1162',
  firewall: 'New-NetFirewallRule -DisplayName "AVLine-Sight Trap" -Direction Inbound -Protocol UDP -LocalPort 162 -Action Allow',
  switch: 'snmp-server host <PC IP> traps version snmpv2 <community>',
};

function CopyCommand({ command }: { command: string }) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle');
  return <div className="rounded border border-slate-500 p-2">
    <pre className="select-text whitespace-pre-wrap break-words">{command}</pre>
    <button className="mt-1" onClick={async () => {
      try { await navigator.clipboard.writeText(command); setState('copied'); }
      catch { setState('failed'); }
    }}>{t('operations.copy')}</button>
    <span className="ml-2" role="status">{state === 'copied' ? t('operations.copied') : state === 'failed' ? t('operations.copyFailed') : ''}</span>
  </div>;
}

export function TrapHelp({ close, error }: { close: () => void; error?: string }) {
  return <Dialog title={t('operations.trapHelpTitle')} close={close}>
    {error && <p role="alert" className="text-amber-700 dark:text-amber-300">⚠ {error}</p>}
    <p>{t('operations.trapIntro')}</p>
    <h3 className="font-bold">{t('operations.trapCheck')}</h3><p>{t('operations.trapCheckText')}</p>
    <h3 className="font-bold">{t('operations.trapFind')}</h3><CopyCommand command={commands.check} /><p>{t('operations.trapFindText')}</p>
    <h3 className="font-bold">{t('operations.trapRemedy')}</h3>
    <p>{t('operations.trapStop')}</p><CopyCommand command={commands.stop} />
    <p>{t('operations.trapPort')}</p><CopyCommand command={commands.bind} />
    <p>{t('operations.trapFirewall')}</p><CopyCommand command={commands.firewall} />
    <h3 className="font-bold">{t('operations.trapSwitch')}</h3>
    <p>{t('operations.trapSwitchText')}</p><CopyCommand command={commands.switch} />
  </Dialog>;
}
