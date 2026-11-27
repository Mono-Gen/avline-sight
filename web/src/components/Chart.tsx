import * as echarts from 'echarts';
import { useEffect, useRef } from 'react';
import { asArray } from '../format';
import { pcTimestamp } from '../pcTime';
import { columnLabels } from './columnLabels';
import { useLang } from '../i18n';

// Series colours (receive blue, send yellow, ...) chosen to stay distinct on dark and light backgrounds.
const SERIES_COLORS = ['#38bdf8', '#facc15', '#f472b6', '#2dd4bf'];

// Text and grid colours follow the current theme (dark/light), read from the element's CSS variables.
function themeColors(el: HTMLElement) {
  const style = getComputedStyle(el);
  const pick = (name: string, fallback: string) => style.getPropertyValue(name).trim() || fallback;
  return { text: pick('--text-primary', '#1f2937'), muted: pick('--text-muted', '#4b5563'), grid: pick('--border', '#d1d5db') };
}

// X-axis ticks show time only (HH:mm); with the date they do not fit. The date is in the tooltip.
function shortTime(label: string) {
  const match = /(\d{1,2}:\d{2})(:\d{2})?/.exec(label);
  return match ? match[1] : label;
}

export function Chart({ points, keys, format, height = 280, labels = {}, unit }: {
  points: Record<string, number | string>[];
  keys: string[];
  // Formats axis and tooltip values (for example bps); plain numbers when omitted.
  format?: (value: number) => string;
  height?: number;
  // Series names for the legend; falls back to the column label, then the key.
  labels?: Record<string, string>;
  // Y-axis unit appended to ticks when there is no format (e.g. "pps").
  unit?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [language] = useLang();
  useEffect(() => {
    if (!ref.current) return;
    const colors = themeColors(ref.current);
    const chart = echarts.init(ref.current);
    const p = asArray(points).slice(-3600);
    const valueText = (value: number) => format ? format(value) : `${Number.isInteger(value) ? value : value.toFixed(1)}${unit ? ` ${unit}` : ''}`;
    chart.setOption({
      color: SERIES_COLORS,
      tooltip: { trigger: 'axis', valueFormatter: (value: unknown) => typeof value === 'number' ? valueText(value) : String(value ?? '—') },
      legend: { top: 0, textStyle: { color: colors.text, fontSize: 12 }, itemWidth: 18 },
      // containLabel keeps the axis numbers inside the chart instead of being cut off at the left edge.
      grid: { left: 8, right: 16, top: 32, bottom: 4, containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: p.map(point => pcTimestamp(point.ts, true)),
        axisLabel: { color: colors.muted, formatter: shortTime, hideOverlap: true },
        axisLine: { lineStyle: { color: colors.grid } },
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: colors.muted, formatter: (value: number) => valueText(value) },
        splitLine: { lineStyle: { color: colors.grid, type: 'dashed' } },
      },
      // Series names use the same column labels as the tables (raw keys only when no label exists).
      series: asArray(keys).map(k => ({
        name: labels[k] || columnLabels[k] || k,
        type: 'line',
        showSymbol: false,
        lineStyle: { width: 1.5 },
        data: asArray(p).map(x => x[k] ?? null)
      }))
    });
    const resize = () => chart.resize();
    window.addEventListener('resize', resize);
    return () => {
      window.removeEventListener('resize', resize);
      chart.dispose();
    };
  }, [points, asArray(keys).join(','), language, format, height, JSON.stringify(labels), unit]);
  return <div ref={ref} style={{
    height,
    width: '100%'
  }} />;
}
