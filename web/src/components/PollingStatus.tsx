import { t } from '../i18n';
export function PollingStatus({ pending, error, updatedAt }: {
  pending: boolean;
  error: string;
  updatedAt?: Date;
}) {
  if (error) return <p role="alert" className="text-red-700 dark:text-red-300">⚠ {error}</p>;
  return <p className="text-sm text-slate-500 dark:text-slate-300" role="status">
    {pending ? t('ui.m145') : updatedAt ? t('ui.m146', { p0: updatedAt.toLocaleTimeString() }) : t('ui.m147')}
  </p>;
}
