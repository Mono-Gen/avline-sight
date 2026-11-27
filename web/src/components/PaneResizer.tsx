import { useEffect, useRef, useState } from 'react';
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
  const ref = useRef<HTMLDivElement>(null);
  // ドラッグ中の後始末。途中で画面が切り替わって仕切りが消えても、カーソルや選択禁止が残らないよう unmount でも呼ぶ
  const cleanup = useRef<(() => void) | null>(null);
  const [width, setWidth] = useState<number>(defaultPx);

  // 実際に見えている幅（CSS の上限で縮んでいる分も含む）。保存値ではなくこれを起点にしないと、
  // 上限に張り付いた状態から縮めるときに動かない区間ができる
  const rendered = () => {
    const pane = side === 'left' ? ref.current?.previousElementSibling : ref.current?.nextElementSibling;
    const w = pane?.getBoundingClientRect().width;
    return w && w > 0 ? w : defaultPx;
  };

  // 起動時に前回の幅を戻す
  useEffect(() => {
    const saved = load(storageKey);
    if (saved !== null) apply(cssVar, clamp(saved, min, max));
    setWidth(rendered());
    return () => cleanup.current?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cssVar, storageKey, min, max]);

  const set = (px: number, persist: boolean) => {
    apply(cssVar, px);
    setWidth(Math.round(px));
    if (persist) save(storageKey, px);
  };

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return; // 右クリックなどでは始めない（終わりのイベントを取り逃がすため）
    e.preventDefault();
    cleanup.current?.();
    const startX = e.clientX;
    const startW = rendered();
    const handle = e.currentTarget;
    handle.setPointerCapture(e.pointerId);
    document.body.classList.add('pane-resizing');
    let last = startW;
    const move = (ev: PointerEvent) => {
      const dx = ev.clientX - startX;
      last = clamp(side === 'left' ? startW + dx : startW - dx, min, max);
      set(last, false);
    };
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', up);
      handle.removeEventListener('pointercancel', up);
      handle.removeEventListener('lostpointercapture', up);
      document.body.classList.remove('pane-resizing');
      cleanup.current = null;
    };
    const up = () => {
      end();
      save(storageKey, last);
    };
    cleanup.current = end;
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', up);
    handle.addEventListener('pointercancel', up);
    handle.addEventListener('lostpointercapture', up);
  };

  // 既定の幅に戻す（ダブルクリック・Enter・Home）
  const reset = () => {
    document.documentElement.style.removeProperty(cssVar);
    save(storageKey, null);
    requestAnimationFrame(() => setWidth(Math.round(rendered())));
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' || e.key === 'Home') {
      e.preventDefault();
      reset();
      return;
    }
    const step = e.shiftKey ? 48 : 16;
    const grow = side === 'left' ? 'ArrowRight' : 'ArrowLeft';
    const shrink = side === 'left' ? 'ArrowLeft' : 'ArrowRight';
    if (e.key !== grow && e.key !== shrink) return;
    e.preventDefault();
    set(clamp(rendered() + (e.key === grow ? step : -step), min, max), true);
  };

  return <div ref={ref} className="pane-resizer" role="separator" aria-orientation="vertical" aria-label={label}
    aria-valuenow={width} aria-valuemin={min} aria-valuemax={max}
    title={`${label}（${t('pane.resizeHelp')}）`} tabIndex={0}
    onPointerDown={onPointerDown} onKeyDown={onKeyDown} onDoubleClick={reset} />;
}
