import * as echarts from 'echarts';
import { useEffect, useRef } from 'react';
import { asArray } from '../format';
import { pcTimestamp } from '../pcTime';
import { columnLabels } from './columnLabels';
import { useLang } from '../i18n';

export function Chart({ points, keys }: {
  points: Record<string, number | string>[];
  keys: string[];
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [language] = useLang();
  useEffect(() => {
    if (!ref.current) return;
    const chart = echarts.init(ref.current);
    const p = asArray(points).slice(-3600);
    chart.setOption({
      tooltip: { trigger: 'axis' },
      legend: { textStyle: { color: '#888' } },
      xAxis: {
        type: 'category',
        data: p.map(point => pcTimestamp(point.ts, true)),
      },
      yAxis: { type: 'value' },
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
  }, [points, asArray(keys).join(','), language]);
  return <div ref={ref} style={{
    height: 280,
    width: '100%'
  }} />;
}
