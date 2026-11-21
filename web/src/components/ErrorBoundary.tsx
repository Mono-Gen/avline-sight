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
        <h2 className="text-xl font-bold">⚠ {this.props.name || 'アプリ'}を表示できませんでした</h2>
        <p>原因: 受信データまたは画面処理に問題が発生しました。</p>
        <p>対処: 再読み込みしてください。繰り返す場合はサーバーの状態を確認してください。</p>
        <button onClick={() => window.location.reload()}>再読み込み</button>
      </section>
    );
  }
}
