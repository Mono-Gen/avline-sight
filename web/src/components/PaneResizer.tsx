import { useEffect, useRef, useState } from 'react';
import type React from 'react';
import { t } from '../i18n';

// Divider that resizes a pane with the mouse (and keyboard). The width lives in a CSS variable and is remembered on this PC.
// side 'left' resizes the pane left of the divider (sidebar); 'right' resizes the pane to its right (port detail).
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
    return null; // storage unavailable (private mode etc.): keep the default width
  }
}

function save(storageKey: string, px: number | null) {
  try {
    if (px === null) localStorage.removeItem(storageKey);
    else localStorage.setItem(storageKey, String(Math.round(px)));
  } catch { /* keep working even if the width cannot be remembered */ }
}

export function PaneResizer({ cssVar, storageKey, side, min, max, defaultPx, label }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  // Cleanup for an ongoing drag; also run on unmount so the cursor and no-select state never stick if the divider disappears mid-drag.
  const cleanup = useRef<(() => void) | null>(null);
  const [width, setWidth] = useState<number>(defaultPx);

  // The rendered width (including any CSS max-width clamp). Dragging must start from this, not the saved value,
  // or shrinking from a clamped width has a dead zone.
  const rendered = () => {
    const pane = side === 'left' ? ref.current?.previousElementSibling : ref.current?.nextElementSibling;
    const w = pane?.getBoundingClientRect().width;
    return w && w > 0 ? w : defaultPx;
  };

  // Restore the previous width on start.
  useEffect(() => {
    const saved = load(storageKey);
    if (saved !== null) apply(cssVar, clamp(saved, min, max));
    setWidth(Math.round(rendered()));
    return () => cleanup.current?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cssVar, storageKey, min, max]);

  const set = (px: number, persist: boolean) => {
    apply(cssVar, px);
    setWidth(Math.round(px));
    if (persist) save(storageKey, px);
  };

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return; // ignore right-click etc. (the end event can be lost)
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

  // Reset to the default width (double-click, Enter, Home).
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
