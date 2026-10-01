import { useState } from "react";
import { Headset, ListOrdered, Phone, PhoneOutgoing, UsersRound } from "lucide-react";
import type { PBXExtension, PBXQueue } from "../../api/types";
import UserAvatar from "../ui/UserAvatar";
import { STATUS_COLOR, chipClass, dotClass, type StatusTone } from "../../lib/status";
import { useSoftphoneContext } from "../../softphone/SoftphoneContext";
import { displayNumber } from "../../softphone/dial";
import { Badge, Button, Card } from "../ui";
import { cn } from "../../lib/utils";
import { ContextMenu, type MenuItem } from "../ContextMenu";

const agentStatus: Record<string, { label: string; tone: StatusTone }> = {
  AVAILABLE: { label: "Boşta", tone: STATUS_COLOR.available.tone },
  TALKING: { label: STATUS_COLOR.talking.label, tone: STATUS_COLOR.talking.tone },
  UNREGISTERED: { label: STATUS_COLOR.unregistered.label, tone: STATUS_COLOR.unregistered.tone },
  BREAK: { label: STATUS_COLOR.break.label, tone: STATUS_COLOR.break.tone },
  BACKOFFICE: { label: STATUS_COLOR.backoffice.label, tone: STATUS_COLOR.backoffice.tone },
  SS_DND: { label: STATUS_COLOR.dnd.label, tone: STATUS_COLOR.dnd.tone },
  OFF_SHIFT: { label: STATUS_COLOR.off.label, tone: STATUS_COLOR.off.tone },
};

// joinNames reads "Toprak", "Toprak ve Ahmet", "Toprak, Ahmet ve Mehmet".
function joinNames(names: string[]): string {
  if (names.length <= 1) return names[0] ?? "";
  return `${names.slice(0, -1).join(", ")} ve ${names[names.length - 1]}`;
}

export function AgentsQueues({ exts, queues, canCall, loading }: { exts: PBXExtension[]; queues: PBXQueue[]; canCall: boolean; loading?: boolean }) {
  const phone = useSoftphoneContext();
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const [confirmExt, setConfirmExt] = useState<string | null>(null);
  const inCall = phone.status === "in-call" || phone.status === "held";
  const canDial = phone.status === "registered" && canCall;

  const sorted = [...exts].sort((a, b) => (a.status === "UNREGISTERED" ? 1 : 0) - (b.status === "UNREGISTERED" ? 1 : 0));

  function agentMenu(e: React.MouseEvent, ext: string, status: string) {
    e.preventDefault();
    // Listening dials the PBX spy code (*5 + extension), so it needs the target
    // to be on a call and our own line to be free.
    const talking = status === "TALKING";
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: `${ext} dahilisini ara`, onClick: () => phone.call(ext).catch(() => undefined), disabled: !canDial || inCall },
        { label: `Çağrıyı ${ext} dahilisine aktar`, onClick: () => phone.transfer(ext).catch(() => undefined), disabled: !inCall },
        {
          label: talking ? `${ext} dahilisinin çağrısını dinle` : `Çağrıyı dinle (${ext} görüşmede değil)`,
          onClick: () => phone.call(`*5${ext}`).catch(() => undefined),
          disabled: !talking || !canDial || inCall,
        },
      ],
    });
  }
  function queueMenu(e: React.MouseEvent, num: string) {
    e.preventDefault();
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: [{ label: `Çağrıyı ${num} kuyruğuna aktar`, onClick: () => phone.transfer(num).catch(() => undefined), disabled: !inCall }],
    });
  }

  return (
    <div className="space-y-4">
      <Card title="Temsilciler" icon={UsersRound}>
        <ul className="max-h-72 space-y-0.5 overflow-y-auto">
          {loading && exts.length === 0 &&
            Array.from({ length: 5 }).map((_, i) => (
              <li key={`sk-${i}`} className="flex items-center justify-between rounded-lg px-2 py-1.5">
                <span className="flex items-center gap-2">
                  <span className="size-2 rounded-full bg-muted animate-pulse" />
                  <span className="h-3.5 w-12 rounded bg-muted/70 animate-pulse" />
                </span>
                <span className="h-4 w-16 rounded bg-muted/70 animate-pulse" />
              </li>
            ))}
          {sorted.map((e) => {
            const s = agentStatus[e.status] ?? { label: e.status, tone: "slate" as const };
            return (
              <li
                key={e.extension}
                onContextMenu={(ev) => agentMenu(ev, e.extension, e.status)}
                onClick={() => canDial && !inCall && setConfirmExt(e.extension)}
                data-tip="Sol tık: ara · Sağ tık: aktar veya dinle"
                className="flex cursor-pointer items-center justify-between rounded-2xl px-2 py-1.5 transition-colors hover:bg-accent/60"
              >
                <span className="flex min-w-0 items-center gap-2.5 text-sm">
                  {e.users && e.users.length > 0 ? (
                    <span className="relative shrink-0">
                      <UserAvatar userId={e.users[0].id} name={e.users[0].name} hasAvatar={e.users[0].hasAvatar} className="size-8" fallbackClassName="bg-primary/10 text-[0.65rem] text-primary" />
                      <span className={cn("absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full ring-2 ring-card", dotClass(s.tone), e.status === "TALKING" && "animate-pulse")} />
                    </span>
                  ) : (
                    <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-xl", chipClass(s.tone))}>
                      {e.status === "TALKING" ? <Phone className="size-4" /> : <Headset className="size-4" />}
                    </span>
                  )}
                  <span className="min-w-0">
                    <span className="flex items-center gap-2">
                      <span className="font-medium">{e.extension}</span>
                      {e.names && e.names.length > 0 && (
                        <span className="truncate text-muted-foreground" data-tip={joinNames(e.names)}>{joinNames(e.names)}</span>
                      )}
                    </span>
                    {e.status === "TALKING" && e.peer && (
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground" data-tip="Görüştüğü numara">
                        <PhoneOutgoing className="size-3 shrink-0 text-primary" />
                        <span className="font-mono tabular-nums">{displayNumber(e.peer) || e.peer}</span>
                        {e.peerName && <span className="truncate">{e.peerName}</span>}
                      </span>
                    )}
                  </span>
                </span>
                <Badge tone={s.tone}>{s.label}</Badge>
              </li>
            );
          })}
          {!loading && exts.length === 0 && <li className="py-4 text-center text-sm text-muted-foreground">Liste alınamadı.</li>}
        </ul>
      </Card>

      <Card title="Kuyruklar" icon={ListOrdered}>
        <ul className="max-h-56 space-y-0.5 overflow-y-auto">
          {queues.map((q) => (
            <li
              key={q.number}
              onContextMenu={(ev) => queueMenu(ev, q.number)}
              data-tip="Sağ tık: kuyruğa aktar"
              className="flex cursor-pointer items-center gap-2.5 rounded-2xl px-2 py-1.5 transition-colors hover:bg-accent/60"
            >
              <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground"><ListOrdered className="size-4" /></span>
              <span className="min-w-0 text-sm"><span className="font-medium">{q.number}</span> <span className="text-muted-foreground">{q.name}</span></span>
            </li>
          ))}
          {queues.length === 0 && <li className="py-4 text-center text-sm text-muted-foreground">Kuyruk yok.</li>}
        </ul>
      </Card>

      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}

      {confirmExt && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={() => setConfirmExt(null)}>
          <div className="w-full max-w-xs rounded-2xl border border-border bg-card p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
            <p className="text-sm">{confirmExt} dahilisini aramak ister misiniz?</p>
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setConfirmExt(null)}>İptal</Button>
              <Button onClick={() => { phone.call(confirmExt).catch(() => undefined); setConfirmExt(null); }}>Ara</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
