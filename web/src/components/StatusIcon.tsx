import { t } from '../i18n';
export function StatusIcon({ ok, label }: {
  ok: boolean;
  label: string;
}) {
  return <span>{ok ? '✓' : '⚠'} {label}: {ok ? t('ui.m188') : t('ui.m189')}</span>;
}
