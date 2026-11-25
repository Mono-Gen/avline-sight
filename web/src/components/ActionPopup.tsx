import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react';

// text: what the trigger shows (a labelled "Actions ▾" where the menu must be easy to find); the default is the compact "⋯".
export function ActionPopup({ label, children, text = '⋯' }: { label: string; children: ReactNode; text?: string }) {
  const [open, setOpen] = useState(false);
  const popup = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const itemsId = useId();
  // Return focus to the trigger only on Escape or item choice; an outside click keeps focus where the user clicked.
  const close = useCallback((restoreFocus = true) => {
    setOpen(false);
    if (restoreFocus) trigger.current?.focus({ preventScroll: true });
  }, []);

  useEffect(() => {
    if (!open) return;
    const outside = (event: MouseEvent) => {
      if (event.target instanceof Node && !popup.current?.contains(event.target)) close(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      event.stopPropagation();
      close();
    };
    // A menu opened near the bottom of the page would hang below the window; bring it into view.
    popup.current?.querySelector?.('.group-menu-items')?.scrollIntoView?.({ block: 'nearest' });
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
      }}>{text}</button>
    {open && <div id={itemsId} className="group-menu-items" onClickCapture={event => {
      if (event.target instanceof Element && event.target.closest('button:enabled, a[href]')) close();
    }}>{children}</div>}
  </div>;
}
