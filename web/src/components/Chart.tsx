import * as echarts from 'echarts';
import { useEffect, useRef } from 'react';
import { asArray } from '../format';
import { pcTimestamp } from '../pcTime';

export function Chart({ points, keys }: {
  points: Record<string, number | string>[];
  keys: string[];
}) {
  const ref = useRef<HTMLDivElement>(null);
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
      series: asArray(keys).map(k => ({
        name: k,
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
  }, [points, asArray(keys).join(',')]);
  return <div ref={ref} style={{
    height: 280,
    width: '100%'
  }} />;
}
