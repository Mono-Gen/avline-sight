import { asArray } from '../format';
import type { SwitchState } from '../types';
import { Badge } from './Badge';
import { Table } from './Table';

export function AvStatus({ state }: {
  state: SwitchState
}) {
  const igmp = state.igmp;
  const groups = asArray(igmp.mvr_groups);
  const failures = asArray(igmp.msrp_failed);
  const warning = failures.length > 0 || igmp.mvr_rx_failures > 0;
  const gptp = igmp.gptp;
  const content = Boolean(igmp.mvr || groups.length || warning || igmp.msrp_streams || igmp.msrp_reservations ||
    gptp?.clock_id || gptp?.grandmaster || gptp?.steps_removed);
  return (
    <details className="rounded border p-3" open={content}>
      <summary className="cursor-pointer font-bold">
        {state.name} · MVR / AVB {content ? '' : '（データなし）'} {warning && <Badge text="WARNING" />}
      </summary>
      <div className="space-y-3 pt-3">
        <Table
          rows={[{
            mvr: igmp.mvr,
            mvr_rx_failures: igmp.mvr_rx_failures || 0,
            mvr_rx_fail_delta: igmp.mvr_rx_fail_delta || 0,
            msrp_streams: igmp.msrp_streams || 0,
            msrp_reservations: igmp.msrp_reservations || 0,
          }]}
        />
        {groups.length > 0 && <><h3>MVRグループ</h3><Table rows={groups} /></>}
        {failures.length > 0 && <><h3>⚠ MSRP予約失敗</h3><Table rows={failures} /></>}
        {gptp && <><h3>gPTPクロック</h3><Table rows={[gptp]} /></>}
      </div>
    </details>
  );
}
