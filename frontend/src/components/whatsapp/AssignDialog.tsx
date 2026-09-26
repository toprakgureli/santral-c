// AssignDialog hands a conversation to a person or a team, with a note
// that stays in the history.

import { useEffect, useMemo, useState } from "react";
import { Search, Users } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Modal } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAAgent, WAConversation, WATeam } from "@/whatsapp/types";

export default function AssignDialog({ conv, open, onClose }: { conv: WAConversation; open: boolean; onClose: () => void }) {
  const [agents, setAgents] = useState<WAAgent[]>([]);
  const [teams, setTeams] = useState<WATeam[]>([]);
  const [q, setQ] = useState("");
  const [pick, setPick] = useState<{ userId?: number; teamId?: number } | null>(null);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setPick(null);
    setNote("");
    setError(null);
    waApi.agents().then(setAgents).catch(() => setAgents([]));
    waApi.teams().then(setTeams).catch(() => setTeams([]));
  }, [open]);

  const people = useMemo(() => {
    const needle = q.trim().toLocaleLowerCase("tr");
    return agents
      .filter((a) => a.canReply && a.channelIds.includes(conv.channelId) && a.id !== conv.ticket?.owner?.id)
      .filter((a) => !needle || a.name.toLocaleLowerCase("tr").includes(needle))
      .sort((a, b) => Number(b.available && b.online) - Number(a.available && a.online) || a.name.localeCompare(b.name, "tr"));
  }, [agents, q, conv]);

  const save = async () => {
    if (!pick) return;
    setBusy(true);
    setError(null);
    try {
      await waApi.assign(conv.id, { ...pick, note });
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Aktarılamadı.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="Sohbeti aktar" description="Seçtiğiniz kişi sorumlu olur. Siz yardımcı olarak kalırsınız; notunuz geçmişe yazılır."
      footer={<>
        {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
        <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
        <Button onClick={() => void save()} disabled={!pick || busy}>{busy ? "Aktarılıyor..." : "Aktar"}</Button>
      </>}
    >
      <div className="space-y-3">
        {teams.length > 0 && (
          <div>
            <p className="mb-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.12em] text-muted-foreground">Ekibe</p>
            <div className="flex flex-wrap gap-1.5">
              {teams.map((t) => (
                <button key={t.id} type="button" onClick={() => setPick({ teamId: t.id })} className={cn("flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium ring-1 transition-colors", pick?.teamId === t.id ? "bg-primary/10 text-primary ring-primary/30" : "ring-border/60 hover:bg-accent")}>
                  <Users className="size-3.5" /> {t.name}
                </button>
              ))}
            </div>
          </div>
        )}
        <div>
          <p className="mb-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.12em] text-muted-foreground">Kişiye</p>
          <div className="relative mb-2">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="İsim ara" className="h-9 w-full rounded-xl border border-border/60 bg-muted/40 pl-9 pr-3 text-sm outline-none focus:border-ring/50" />
          </div>
          <div className="max-h-64 space-y-0.5 overflow-y-auto">
            {people.map((a) => (
              <button key={a.id} type="button" onClick={() => setPick({ userId: a.id })} className={cn("flex w-full items-center gap-3 rounded-xl px-2 py-1.5 text-left transition-colors", pick?.userId === a.id ? "bg-primary/10 ring-1 ring-primary/30" : "hover:bg-accent/60")}>
                <UserAvatar userId={a.id} name={a.name} hasAvatar={a.hasAvatar} version={a.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary">
                  <span className={cn("absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full ring-2 ring-card", a.online && a.available ? "bg-success" : a.online ? "bg-warning" : "bg-muted-foreground/40")} />
                </UserAvatar>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{a.name}</span>
                  <span className="block text-[0.65rem] text-muted-foreground">{a.online && a.available ? "Müsait" : a.online ? "Panelde, müsait değil" : "Panelde değil"}</span>
                </span>
              </button>
            ))}
            {people.length === 0 && <p className="px-2 py-4 text-xs text-muted-foreground">Bu cihazda aktarılabilecek başka kimse yok.</p>}
          </div>
        </div>
        <label className="block space-y-1">
          <span className="text-[0.7rem] font-medium text-muted-foreground">Not (isteğe bağlı)</span>
          <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={2} placeholder="Örn. fatura iadesi konuşuluyor, müşteri 15:00'te dönüş bekliyor" className="w-full resize-none rounded-xl border border-border/60 bg-muted/40 px-3 py-2 text-sm outline-none focus:border-ring/50" />
        </label>
      </div>
    </Modal>
  );
}
