import { t } from '../i18n';
const levels: Record<string, {
  icon: string;
  text: string;
  className: string;
}> = {
  CRITICAL: {
    icon: '⛔',
    get text() { return t('ui.m005'); },
    className: 'badge-critical'
  },
  WARNING: {
    icon: '⚠',
    get text() { return t('ui.m006'); },
    className: 'badge-warning'
  },
  INFO: {
    icon: 'ℹ',
    get text() { return t('ui.m007'); },
    className: 'badge-info'
  },
};

export function Badge({ text }: {
  text: string;
}) {
  const level = levels[String(text).toUpperCase()];
  return (
    <span className={`inline-block rounded px-2 py-1 ${level?.className || 'bg-slate-200 text-slate-900'}`}>
      {level ? `${level.icon} ${level.text}` : text}
    </span>
  );
}
