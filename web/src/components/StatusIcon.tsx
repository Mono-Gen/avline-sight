export function StatusIcon({ ok, label }: {
  ok: boolean;
  label: string;
}) {
  return <span>{ok ? '✓' : '⚠'} {label}: {ok ? '正常' : '要確認'}</span>;
}
