const levels: Record<string, {
  icon: string;
  text: string;
  className: string;
}> = {
  CRITICAL: {
    icon: '⛔',
    text: '重大',
    className: 'bg-red-200 text-red-950'
  },
  WARNING: {
    icon: '⚠',
    text: '警告',
    className: 'bg-amber-200 text-amber-950'
  },
  INFO: {
    icon: 'ℹ',
    text: '情報',
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
