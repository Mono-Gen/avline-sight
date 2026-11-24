import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react';

export function ActionPopup({ label, children }: { label: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const popup = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const itemsId = useId();
  const close = useCallback(() => {
    setOpen(false);
    trigger.current?.focus({ preventScroll: true });
  }, []);

  useEffect(() => {
    if (!open) return;
    const outside = (event: MouseEvent) => {
      if (event.target instanceof Node && !popup.current?.contains(event.target)) close();
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      event.stopPropagation();
      close();
    };
    document.addEventListener('click', outside, true);
    document.addEventListener('keydown', escape, true);
    return () => {
      document.removeEventListener('click', outside, true);
      document.removeEventListener('keydown', escape, true);
    };
  }, [open, close]);

  return <div className="group-menu" ref={popup}>
    <button type="button" className="group-menu-trigger" ref={trigger} aria-label={label}
      aria-expanded={open} aria-controls={open ? itemsId : undefined} onClick={event => {
        event.currentTarget.focus({ preventScroll: true });
        setOpen(!open);
      }}>⋯</button>
    {open && <div id={itemsId} className="group-menu-items" onClickCapture={event => {
      if (event.target instanceof Element && event.target.closest('button:enabled, a[href]')) close();
    }}>{children}</div>}
  </div>;
}
