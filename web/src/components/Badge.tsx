import { t } from '../i18n';
const levels: Record<string, {
  icon: string;
  text: string;
  className: string;
}> = {
  CRITICAL: {
    icon: '⛔',
    get text() { return t('ui.m005'); },
    className: 'bg-red-200 text-red-950'
  },
  WARNING: {
    icon: '⚠',
    get text() { return t('ui.m006'); },
    className: 'bg-amber-200 text-amber-950'
  },
  INFO: {
    icon: 'ℹ',
    get text() { return t('ui.m007'); },
    className: 'bg-sky-200 text-sky-950'
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
