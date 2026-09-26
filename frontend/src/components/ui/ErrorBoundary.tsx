// ErrorBoundary keeps a broken page from taking the whole panel down: the
// menu and the phone stay, and the page's place shows what happened with
// a way to try again. It resets when the person goes to another page.

import { Component, type ErrorInfo, type ReactNode } from "react";
import { RotateCcw, TriangleAlert } from "lucide-react";

export default class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("page crashed", error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div className="flex min-h-[50vh] flex-col items-center justify-center gap-4 px-4 text-center">
        <span className="flex size-14 items-center justify-center rounded-2xl bg-destructive/10 text-destructive"><TriangleAlert className="size-6" /></span>
        <div className="space-y-1">
          <p className="text-base font-semibold">Bu sayfa açılırken bir sorun oldu</p>
          <p className="mx-auto max-w-md text-sm text-muted-foreground">Sayfayı yenilemeyi deneyin. Düzelmezse aşağıdaki hata yazısını yöneticinize iletin.</p>
        </div>
        <button type="button" onClick={() => window.location.reload()} className="flex h-10 items-center gap-2 rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground shadow-sm">
          <RotateCcw className="size-4" /> Sayfayı yenile
        </button>
        <details className="max-w-xl text-left text-xs text-muted-foreground">
          <summary className="cursor-pointer text-center">Hata yazısı</summary>
          <pre className="mt-2 overflow-auto rounded-xl bg-muted/50 p-3 whitespace-pre-wrap">{error.message}</pre>
        </details>
      </div>
    );
  }
}
