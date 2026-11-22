import { t } from '../i18n';
import React from 'react';

interface Props {
  children: React.ReactNode;
  name?: string;
}

export class ErrorBoundary extends React.Component<Props, {
  error: Error | null;
}> {
  state: {
    error: Error | null;
  } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <section className="panel space-y-3" role="alert">
        <h2 className="text-xl font-bold">⚠ {this.props.name || t('ui.m123')}{t('ui.m124')}</h2>
        <p>{t('ui.m125')}</p>
        <p>{t('ui.m126')}</p>
        <button onClick={() => window.location.reload()}>{t('ui.m127')}</button>
      </section>
    );
  }
}
