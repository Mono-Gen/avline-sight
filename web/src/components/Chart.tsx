import * as echarts from 'echarts';
import { useEffect, useRef } from 'react';
import { asArray } from '../format';
import { pcTimestamp } from '../pcTime';
import { columnLabels } from './columnLabels';
import { useLang } from '../i18n';

export function Chart({ points, keys, format, height = 280 }: {
  points: Record<string, number | string>[];
  keys: string[];
  // Formats axis and tooltip values (for example bps); plain numbers when omitted.
  format?: (value: number) => string;
  height?: number;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [language] = useLang();
  useEffect(() => {
    if (!ref.current) return;
    const chart = echarts.init(ref.current);
    const p = asArray(points).slice(-3600);
    chart.setOption({
      tooltip: { trigger: 'axis', valueFormatter: format ? (value: unknown) => typeof value === 'number' ? format(value) : String(value ?? '—') : undefined },
      legend: { textStyle: { color: '#888' } },
      // containLabel keeps the axis numbers inside the chart instead of being cut off at the left edge.
      grid: { left: 8, right: 12, top: 36, bottom: 8, containLabel: true },
      xAxis: {
        type: 'category',
        data: p.map(point => pcTimestamp(point.ts, true)),
      },
      yAxis: { type: 'value', axisLabel: format ? { formatter: (value: number) => format(value) } : undefined },
      // Series names use the same column labels as the tables (raw keys only when no label exists).
      series: asArray(keys).map(k => ({
        name: columnLabels[k] || k,
        type: 'line',
        showSymbol: false,
        data: asArray(p).map(x => x[k] ?? null)
      }))
    });
    const resize = () => chart.resize();
    window.addEventListener('resize', resize);
    return () => {
      window.removeEventListener('resize', resize);
      chart.dispose();
    };
  }, [points, asArray(keys).join(','), language, format, height]);
  return <div ref={ref} style={{
    height,
    width: '100%'
  }} />;
}
