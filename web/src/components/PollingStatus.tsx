export function PollingStatus({ pending, error, updatedAt }: {
  pending: boolean;
  error: string;
  updatedAt?: Date;
}) {
  if (error) return <p role="alert" className="text-red-700 dark:text-red-300">⚠ {error}</p>;
  return <p className="text-sm text-slate-500 dark:text-slate-300" role="status">
    {pending ? '更新中…' : updatedAt ? `最終更新: ${updatedAt.toLocaleTimeString()}` : '取得待ち'}
  </p>;
}
