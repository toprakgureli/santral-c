// ErrorBoundary keeps a broken page from taking the whole panel down: the
// menu and the phone stay, and the page's place shows what happened with
// a way to try again. It resets when the person goes to another page.
// A page that could not load because a new version of the panel was
// published meanwhile is not an error: the panel reloads itself, and
// waits for the end of a call first.

import { Component, useEffect, useState, type ErrorInfo, type ReactNode } from "react";
import { RefreshCw, RotateCcw, TriangleAlert } from "lucide-react";
import { isUpdateError, reloadOnce } from "@/lib/update";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

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
    if (isUpdateError(error)) return <NewVersion />;
    return (
      <div className="flex min-h-[50vh] flex-col items-center justify-center gap-4 px-4 text-center">
        <span className="flex size-14 items-center justify-center rounded-2xl bg-destructive/10 text-destructive"><TriangleAlert className="size-6" /></span>
        <div className="space-y-1">
          <p className="text-base font-semibold">Bu sayfa açılırken bir sorun oldu</p>
          <p className="mx-auto max-w-md text-sm text-muted-foreground">Sayfayı yenilemeyi dene. Düzelmezse aşağıdaki hata yazısını yöneticine ilet.</p>
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

// busy are the phone states during which the page must not reload.
const busy = new Set(["calling", "ringing", "incoming", "in-call", "held"]);

function NewVersion() {
  const phone = useSoftphoneContext();
  const onCall = busy.has(phone.status);
  // stuck: a reload a moment ago did not bring the file either.
  const [stuck, setStuck] = useState(false);
  useEffect(() => {
    if (!onCall && !reloadOnce()) setStuck(true);
  }, [onCall]);
  if (stuck) {
    return (
      <div className="flex min-h-[50vh] flex-col items-center justify-center gap-4 px-4 text-center">
        <span className="flex size-14 items-center justify-center rounded-2xl bg-destructive/10 text-destructive"><TriangleAlert className="size-6" /></span>
        <p className="max-w-md text-sm text-muted-foreground">Sayfanın yeni sürümü yüklenemedi. Biraz sonra sayfayı yenile; düzelmezse yöneticine haber ver.</p>
        <button type="button" onClick={() => window.location.reload()} className="flex h-10 items-center gap-2 rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground shadow-sm">
          <RotateCcw className="size-4" /> Sayfayı yenile
        </button>
      </div>
    );
  }
  return (
    <div className="flex min-h-[50vh] flex-col items-center justify-center gap-4 px-4 text-center" role="status">
      <span className="flex size-14 items-center justify-center rounded-2xl bg-primary/10 text-primary"><RefreshCw className={onCall ? "size-6" : "size-6 animate-spin"} /></span>
      <div className="space-y-1">
        <p className="text-base font-semibold">Panelin yeni sürümü yayında</p>
        <p className="mx-auto max-w-md text-sm text-muted-foreground">
          {onCall ? "Görüşmen bitince sayfa kendiliğinden yenilenecek. Görüşmeni sürdürebilirsin." : "Sayfa yenileniyor..."}
        </p>
      </div>
      {onCall && (
        <p className="text-xs text-muted-foreground">Telefon ve diğer sayfalar çalışmaya devam ediyor.</p>
      )}
    </div>
  );
}
