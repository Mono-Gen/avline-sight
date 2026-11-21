import React from 'react';

export function Dialog({ title, close, children }: {
  title: string;
  close: () => void;
  children: React.ReactNode;
}) {
  return <div
    className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-3"
    role="dialog"
    aria-modal="true"
    aria-label={title}
  >
    <section className="panel max-h-[90vh] w-full max-w-3xl space-y-3 overflow-auto">
      <div className="flex justify-between">
        <h2 className="text-xl font-bold">
          {title}
        </h2>
        <button onClick={close}>閉じる
        </button>
      </div>
      {children}
    </section>
  </div>;
}
