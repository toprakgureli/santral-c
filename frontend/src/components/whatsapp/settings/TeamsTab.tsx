// TeamsTab groups the people who answer WhatsApp into teams, so a chat can
// be sent to "Teknik Destek" or "Satış" and shared out inside that team.

import { useEffect, useState } from "react";
import { Pencil, Plus, Trash2, UsersRound } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Card, ConfirmDialog, EmptyState, Modal } from "@/components/ui";
import { ListRow } from "@/components/ui/rows";
import UserAvatar from "@/components/ui/UserAvatar";
import { FormField, inputCls, PeoplePicker } from "@/components/whatsapp/settings/parts";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAAgent, WATeam } from "@/whatsapp/types";

const COLORS = ["#10b981", "#0ea5e9", "#6366f1", "#f59e0b", "#ef4444", "#ec4899", "#8b5cf6", "#64748b"];

export default function TeamsTab() {
  const [teams, setTeams] = useState<WATeam[]>([]);
  const [people, setPeople] = useState<WAAgent[]>([]);
  const [edit, setEdit] = useState<WATeam | "new" | null>(null);
  const [del, setDel] = useState<WATeam | null>(null);
  const [error, setError] = useState<string | null>(null);
  const load = () => {
    waApi.teams().then(setTeams).catch(() => setTeams([]));
    waApi.agents().then(setPeople).catch(() => setPeople([]));
  };
  useEffect(load, []);

  return (
    <Card title="Ekipler" icon={UsersRound} actions={<Button onClick={() => setEdit("new")}><Plus /> Ekip ekle</Button>}>
      <p className="mb-4 text-sm text-muted-foreground">Sohbet bir ekibe aktarılınca o ekipteki müsait kişiye düşer. Bir kişi birden fazla ekipte olabilir. Ekibi görme yetkisi olanlar kendi ekiplerinin bütün sohbetlerini görür.</p>
      {error && <p className="mb-3 text-sm text-destructive">{error}</p>}
      {teams.length === 0 ? (
        <EmptyState icon={<UsersRound />} title="Henüz ekip yok" description="Ekip kurmak zorunlu değil. Kurmazsanız sohbetler numarada çalışan herkese dağıtılır." />
      ) : (
        <div className="grid gap-2 md:grid-cols-2">
          {teams.map((t) => {
            const members = people.filter((p) => t.memberIds.includes(p.id));
            return (
              <ListRow
                key={t.id}
                className="ring-1 ring-border/60"
                leading={<span className="flex size-8 shrink-0 items-center justify-center rounded-xl text-sm font-bold text-white" style={{ background: t.color || COLORS[0] }}>{t.name.slice(0, 1).toLocaleUpperCase("tr")}</span>}
                title={t.name}
                sub={`${t.memberIds.length} kişi · ${members.filter((m) => m.online).length} çevrimiçi`}
                trailing={
                  <>
                    <span className="hidden -space-x-1.5 sm:flex">
                      {members.slice(0, 5).map((m) => <UserAvatar key={m.id} userId={m.id} name={m.name} hasAvatar={m.hasAvatar} version={m.avatarVersion} className="size-7 ring-2 ring-card" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />)}
                    </span>
                    <button type="button" data-tip="Düzenle" onClick={() => setEdit(t)} className="flex size-8 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground"><Pencil className="size-3.5" /></button>
                    <button type="button" data-tip="Sil" onClick={() => setDel(t)} className="flex size-8 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-3.5" /></button>
                  </>
                }
              />
            );
          })}
        </div>
      )}
      {edit && <TeamForm team={edit === "new" ? null : edit} people={people} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); }} />}
      <ConfirmDialog open={!!del} title="Ekip silinsin mi?" description={`${del?.name} silinir. Bu ekibe aktarılmış açık sohbetler ekipsiz kalır, kimseden alınmaz.`} confirmLabel="Sil" onCancel={() => setDel(null)}
        onConfirm={() => del && void waApi.deleteTeam(del.id).then(() => { setDel(null); load(); }).catch((e) => { setError(e instanceof ApiError ? e.message : "Silinemedi."); setDel(null); })} />
    </Card>
  );
}

function TeamForm({ team, people, onClose, onSaved }: { team: WATeam | null; people: WAAgent[]; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(team?.name ?? "");
  const [color, setColor] = useState(team?.color || COLORS[0]);
  const [ids, setIds] = useState<number[]>(team?.memberIds ?? []);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await waApi.saveTeam(team?.id ?? 0, { name, color, memberIds: ids });
      onSaved();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={team ? `${team.name} ekibini düzenle` : "Yeni ekip"} size="lg" footer={<>
      {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
      <span className="mr-auto text-xs text-muted-foreground">{ids.length} kişi seçili</span>
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button onClick={() => void save()} disabled={busy || !name.trim()}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
    </>}>
      <div className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
          <FormField label="Ekip adı"><input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} placeholder="Teknik Destek" /></FormField>
          <div className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">Renk</span>
            <div className="flex h-10 items-center gap-1.5">
              {COLORS.map((c) => <button key={c} type="button" onClick={() => setColor(c)} aria-label={c} className={cn("size-6 rounded-full transition-transform", color === c && "scale-110 ring-2 ring-foreground/60 ring-offset-2 ring-offset-card")} style={{ background: c }} />)}
            </div>
          </div>
        </div>
        <PeoplePicker people={people} value={ids} onChange={setIds} />
      </div>
    </Modal>
  );
}
