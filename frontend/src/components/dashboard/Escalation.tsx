import { useEffect, useState } from "react";
import { TriangleAlert } from "lucide-react";
import type { EscalationCategory } from "../../api/types";
import { EscalationForm } from "../escalation/EscalationForm";
import { markWrapUpDone } from "../layout/WrapUpCard";
import { displayNumber } from "../../softphone/dial";
import { Badge } from "../ui";
import { cn } from "../../lib/utils";

export function Escalation({ categories, activePeer, connected, callId, canSearch }: { categories: EscalationCategory[]; activePeer?: string; connected: boolean; callId: string | null; canSearch: boolean }) {
  const [customer, setCustomer] = useState("");
  const [historyCount, setHistoryCount] = useState(0);
  const [dirty, setDirty] = useState(false);
  // A call that arrived while an entry for the previous customer is half
  // written; offered as a switch instead of wiping the form.
  const [pendingPeer, setPendingPeer] = useState<string | null>(null);
  const onActiveCall = !!activePeer;

  // Follow the live call's number; keep it after the call ends so the agent can
  // still wrap up. A half-written entry is never replaced under the agent.
  useEffect(() => {
    if (!activePeer) return;
    const next = displayNumber(activePeer);
    if (next === customer) return;
    if (dirty && customer) setPendingPeer(next);
    else {
      setCustomer(next);
      setPendingPeer(null);
    }
  }, [activePeer]); // eslint-disable-line react-hooks/exhaustive-deps

  function switchToPending() {
    if (!pendingPeer) return;
    setCustomer(pendingPeer);
    setPendingPeer(null);
  }

  if (categories.length === 0) {
    return (
      <EscalationFrame>
        <p className="rounded-xl bg-muted/40 px-4 py-6 text-center text-sm text-muted-foreground">
          Henüz eskalasyon durumu tanımlı değil. Yönetici, <span className="font-medium">Eskalasyon</span> menüsünden kategori ve durum ekleyebilir.
        </p>
      </EscalationFrame>
    );
  }

  if (!customer.trim()) {
    return (
      <EscalationFrame>
        <p className="rounded-xl bg-muted/40 px-4 py-8 text-center text-sm text-muted-foreground">
          Bir çağrı başladığında müşteri bilgisi burada belirir ve eskalasyon girebilirsiniz.
        </p>
      </EscalationFrame>
    );
  }

  return (
    <EscalationFrame active={onActiveCall} connected={connected}>
      <div className="space-y-4">
        {/* Prominent customer header, highlighted during a live call */}
        <div className={cn("rounded-2xl px-4 py-3 transition", onActiveCall ? "bg-primary/10 ring-1 ring-primary/30" : "bg-muted/40")}>
          <div className="flex items-center justify-between gap-2">
            <div>
              <div className="text-xs text-muted-foreground">{onActiveCall ? "Görüşülen müşteri" : "Son müşteri"}</div>
              <div className={cn("font-bold tabular-nums tracking-wide", connected ? "text-3xl" : "text-2xl")}>{customer}</div>
            </div>
            {canSearch && (
              <Badge tone={historyCount ? "amber" : "slate"}>{historyCount} geçmiş kayıt</Badge>
            )}
          </div>
          {connected && (
            <p className="mt-2 text-xs text-primary">Görüşme bitince eskalasyon kartı açılır. Şimdiden girersen çağrı sonunda tekrar sorulmaz.</p>
          )}
        </div>

        {pendingPeer && (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-warning/10 px-3.5 py-2 text-xs ring-1 ring-warning/30">
            <span>
              Yeni çağrı: <span className="font-mono font-semibold tabular-nums">{pendingPeer}</span>. Bu kayıt bitmeden müşteri değişmedi.
            </span>
            <button type="button" onClick={switchToPending} className="font-medium text-warning underline-offset-2 hover:underline">Yeni müşteriye geç</button>
          </div>
        )}
        <EscalationForm
          categories={categories}
          number={customer}
          canSearch={canSearch}
          callUuid={connected && callId && !pendingPeer ? callId : undefined}
          onSaved={() => {
            if (connected && callId && !pendingPeer) markWrapUpDone(callId);
            if (pendingPeer) switchToPending();
          }}
          onHistory={(items) => setHistoryCount(items.length)}
          onDirtyChange={setDirty}
        />
      </div>
    </EscalationFrame>
  );
}

// EscalationFrame is a deliberately prominent card: a coloured header with an
// icon and a strong ring so the escalation area stands out during a call.
function EscalationFrame({ active, connected, children }: { active?: boolean; connected?: boolean; children: React.ReactNode }) {
  return (
    <section className={cn("rounded-2xl bg-card shadow-sm ring-1 transition-[box-shadow,--tw-ring-color] duration-300", connected ? "ring-primary/60 shadow-lg shadow-primary/10" : active ? "ring-primary/40" : "ring-border/60")}>
      <header className="flex items-center gap-2.5 border-b border-border/60 px-5 py-3.5">
        <span className={cn("flex size-8 items-center justify-center rounded-xl transition-colors", connected ? "bg-primary text-primary-foreground shadow-sm shadow-primary/30" : "bg-primary/10 text-primary")}>
          <TriangleAlert className="size-4" />
        </span>
        <div>
          <h2 className="text-sm font-semibold leading-tight tracking-tight">Eskalasyon</h2>
          <p className="text-xs text-muted-foreground">Görüşme sonucunu kaydet</p>
        </div>
        {active && <span className="ml-auto flex items-center gap-1.5 rounded-full bg-primary/15 px-2.5 py-1 text-xs font-medium text-primary"><span className="size-1.5 animate-pulse rounded-full bg-primary" /> Canlı çağrı</span>}
      </header>
      <div className="p-5">{children}</div>
    </section>
  );
}
