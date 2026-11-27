import { useEffect } from 'react';
import type React from 'react';
import { t } from '../i18n';

// ペインの幅をマウス（とキーボード）で変える仕切り。幅は CSS 変数に入れ、この PC に覚えておく。
// side: 'left' は仕切りの左にあるペイン（左ペイン）、'right' は右にあるペイン（ポート詳細）の幅を変える。
type Props = { cssVar: string; storageKey: string; side: 'left' | 'right'; min: number; max: number; defaultPx: number; label: string };

const clamp = (v: number, min: number, max: number) => Math.min(max, Math.max(min, v));

function apply(cssVar: string, px: number) {
  document.documentElement.style.setProperty(cssVar, `${Math.round(px)}px`);
}

function load(storageKey: string): number | null {
  try {
    const v = Number(localStorage.getItem(storageKey));
    return Number.isFinite(v) && v > 0 ? v : null;
  } catch {
    return null; // 保存できない環境（プライベートモード等）では既定幅のまま
  }
}

function save(storageKey: string, px: number | null) {
  try {
    if (px === null) localStorage.removeItem(storageKey);
    else localStorage.setItem(storageKey, String(Math.round(px)));
  } catch { /* 覚えられなくても操作は続ける */ }
}

export function PaneResizer({ cssVar, storageKey, side, min, max, defaultPx, label }: Props) {
  // 起動時に前回の幅を戻す
  useEffect(() => {
    const saved = load(storageKey);
    if (saved !== null) apply(cssVar, clamp(saved, min, max));
  }, [cssVar, storageKey, min, max]);

  const current = () => {
    const v = parseFloat(getComputedStyle(document.documentElement).getPropertyValue(cssVar));
    return Number.isFinite(v) ? v : defaultPx;
  };

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const startX = e.clientX;
    const startW = current();
    const handle = e.currentTarget;
    handle.setPointerCapture(e.pointerId);
    document.body.classList.add('pane-resizing');
    let last = startW;
    const move = (ev: PointerEvent) => {
      const dx = ev.clientX - startX;
      last = clamp(side === 'left' ? startW + dx : startW - dx, min, max);
      apply(cssVar, last);
    };
    const up = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', up);
      handle.removeEventListener('pointercancel', up);
      document.body.classList.remove('pane-resizing');
      save(storageKey, last);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', up);
    handle.addEventListener('pointercancel', up);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    const step = e.shiftKey ? 48 : 16;
    const grow = side === 'left' ? 'ArrowRight' : 'ArrowLeft';
    const shrink = side === 'left' ? 'ArrowLeft' : 'ArrowRight';
    if (e.key !== grow && e.key !== shrink) return;
    e.preventDefault();
    const next = clamp(current() + (e.key === grow ? step : -step), min, max);
    apply(cssVar, next);
    save(storageKey, next);
  };

  // ダブルクリックで既定の幅に戻す
  const reset = () => {
    document.documentElement.style.removeProperty(cssVar);
    save(storageKey, null);
  };

  return <div className="pane-resizer" role="separator" aria-orientation="vertical" aria-label={label}
    title={`${label}（${t('pane.resizeHelp')}）`} tabIndex={0}
    onPointerDown={onPointerDown} onKeyDown={onKeyDown} onDoubleClick={reset} />;
}
