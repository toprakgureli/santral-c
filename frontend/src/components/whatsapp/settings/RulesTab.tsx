// RulesTab builds the automatic messages: "when this happens, and these
// hold, do that". Rules run top to bottom; each number has its own.

import { useEffect, useMemo, useState } from "react";
import { ArrowDown, ArrowUp, Bot, Copy, Pencil, Plus, Sparkles, Trash2, Wand2, X } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Card, ConfirmDialog, EmptyState, Modal } from "@/components/ui";
import { areaCls, DeviceChips, DevicePicker, FormField, inputCls, Switch } from "@/components/whatsapp/settings/parts";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAAgent, WAChannel, WARule, WARuleAction, WARuleCondition, WATeam, WATemplate } from "@/whatsapp/types";
import { PRIORITY_WORD, since, STATUS_WORD, TRIGGER_WORD } from "@/whatsapp/util";

const CONDITIONS: Record<string, { label: string; short?: string; needs?: string; placeholder?: string }> = {
  hours_open: { label: "Mesai içindeyse" },
  hours_closed: { label: "Mesai dışındaysa" },
  text_contains: { label: "Mesajda şu kelimelerden biri geçiyorsa", short: "mesajda geçerse", needs: "text", placeholder: "fiyat, ücret, kampanya" },
  text_equals: { label: "Mesaj tam olarak şuysa", short: "mesaj tam olarak", needs: "text", placeholder: "merhaba" },
  text_regex: { label: "Mesaj şu kalıba uyuyorsa (ileri düzey)", short: "mesaj kalıba uyarsa", needs: "text", placeholder: "^\\d{6}$" },
  tag_has: { label: "Sohbette ya da müşteride şu etiket varsa", short: "etiket", needs: "text", placeholder: "vip" },
  status_is: { label: "Sohbetin durumu şuysa", needs: "status" },
  no_owner: { label: "Sohbet henüz kimseye verilmediyse" },
};

const ACTIONS: Record<string, string> = {
  send_text: "Mesaj gönder",
  send_template: "Şablon gönder",
  assign_team: "Ekibe aktar",
  assign_user: "Kişiye ver",
  add_tag: "Etiket ekle",
  set_priority: "Önceliği değiştir",
  set_category: "Konu yaz",
  note: "Ekibe not bırak",
  send_survey: "Memnuniyet anketi gönder",
  resolve: "Sohbeti çözüldü yap",
  webhook: "Başka bir sisteme haber ver",
};

type Draft = Omit<WARule, "id" | "runs" | "lastRunAt" | "updatedAt" | "position">;

const RECIPES: { title: string; sub: string; make: (ids: number[]) => Draft }[] = [
  {
    title: "Mesai dışı cevabı",
    sub: "Mesai dışında yazana ne zaman döneceğinizi söyler.",
    make: (ids) => ({ name: "Mesai dışı cevabı", active: true, channelIds: ids, trigger: "outside_hours", conditions: [], cooldownMin: 720, actions: [{ kind: "send_text", text: "Merhaba {musteri}, şu an mesai saatleri dışındayız. Mesajınızı aldık, ilk iş saatinde size dönüş yapacağız." }] }),
  },
  {
    title: "İlk mesaja hoş geldin",
    sub: "İlk kez yazan müşteriyi karşılar.",
    make: (ids) => ({ name: "Hoş geldin", active: true, channelIds: ids, trigger: "first_message", conditions: [{ kind: "hours_open", value: "" }], cooldownMin: 0, actions: [{ kind: "send_text", text: "Merhaba {musteri}, bize yazdığınız için teşekkürler. Birazdan bir arkadaşımız sizinle ilgilenecek." }] }),
  },
  {
    title: "Bekleyene haber ver",
    sub: "Müşteri 10 dakika cevap alamazsa unutulmadığını söyler.",
    make: (ids) => ({ name: "Bekleme bilgisi", active: true, channelIds: ids, trigger: "no_reply", conditions: [{ kind: "after_minutes", value: "10" }], cooldownMin: 0, actions: [{ kind: "send_text", text: "Yoğunluk nedeniyle biraz gecikiyoruz, sırada olduğunuzu bilmenizi isteriz. En kısa sürede dönüyoruz." }] }),
  },
  {
    title: "Fiyat sorularını etiketle",
    sub: "Mesajda fiyat geçince sohbete satış etiketi koyar.",
    make: (ids) => ({ name: "Fiyat soruları", active: true, channelIds: ids, trigger: "message_in", conditions: [{ kind: "text_contains", value: "fiyat, ücret, teklif" }, { kind: "no_owner", value: "" }], cooldownMin: 0, actions: [{ kind: "add_tag", value: "satış" }] }),
  },
];

export default function RulesTab({ channels }: { channels: WAChannel[] }) {
  const [rules, setRules] = useState<WARule[]>([]);
  const [edit, setEdit] = useState<Draft & { id?: number } | null>(null);
  const [del, setDel] = useState<WARule | null>(null);
  const [device, setDevice] = useState(0);
  const [copy, setCopy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);
  const now = Date.now();
  const load = () => waApi.rules().then(setRules).catch(() => setRules([]));
  useEffect(() => { void load(); }, []);

  const shown = useMemo(() => rules.filter((r) => !device || r.channelIds.includes(device)).sort((a, b) => a.position - b.position || a.id - b.id), [rules, device]);

  const move = async (i: number, dir: -1 | 1) => {
    const list = [...shown];
    const j = i + dir;
    if (j < 0 || j >= list.length) return;
    [list[i], list[j]] = [list[j], list[i]];
    const ids = list.map((r) => r.id);
    setRules((cur) => cur.map((r) => ({ ...r, position: ids.indexOf(r.id) >= 0 ? ids.indexOf(r.id) : r.position })));
    await waApi.orderRules(ids).catch(() => void load());
  };

  const toggle = async (r: WARule) => {
    setRules((cur) => cur.map((x) => (x.id === r.id ? { ...x, active: !r.active } : x)));
    try {
      await waApi.saveRule(r.id, { name: r.name, active: !r.active, channelIds: r.channelIds, trigger: r.trigger, conditions: r.conditions, actions: r.actions, cooldownMin: r.cooldownMin });
    } catch (e) {
      setMsg(e instanceof ApiError ? e.message : "Değiştirilemedi.");
      void load();
    }
  };

  const allIds = channels.map((c) => c.id);
  return (
    <div className="space-y-4">
      <Card title="Otomatik mesajlar" icon={Wand2} actions={
        <span className="flex items-center gap-2">
          {channels.length > 1 && <Button variant="secondary" onClick={() => setCopy(true)}><Copy /> Cihaza kopyala</Button>}
          <Button onClick={() => setEdit({ name: "", active: true, channelIds: device ? [device] : allIds.slice(0, 1), trigger: "message_in", conditions: [], actions: [{ kind: "send_text", text: "" }], cooldownMin: 0 })}><Plus /> Yeni kural</Button>
        </span>
      }>
        <p className="mb-4 text-sm text-muted-foreground">Bir şey olduğunda, şartlar tutuyorsa sistem sizin yerinize bir iş yapar. Kurallar yukarıdan aşağıya sırayla çalışır. Müşteri son 24 saatte yazmadıysa düz mesaj gidemez; öyle durumlar için şablon seçin.</p>
        {channels.length > 1 && (
          <div className="mb-3 flex flex-wrap gap-1">
            <DeviceFilter active={device === 0} onClick={() => setDevice(0)}>Tümü</DeviceFilter>
            {channels.map((c) => <DeviceFilter key={c.id} active={device === c.id} onClick={() => setDevice(c.id)}>{c.name}</DeviceFilter>)}
          </div>
        )}
        {msg && <p className="mb-3 text-sm text-destructive">{msg}</p>}
        {info && <p className="mb-3 rounded-xl bg-success/10 px-3 py-2 text-sm text-success">{info}</p>}
        {shown.length === 0 ? (
          <EmptyState icon={<Wand2 />} title="Henüz kural yok" description="Aşağıdaki hazır örneklerden biriyle başlayabilirsiniz." />
        ) : (
          <div className="space-y-2">
            {shown.map((r, i) => (
              <div key={r.id} className={cn("flex items-start gap-3 rounded-2xl px-3 py-3 ring-1 transition-colors", r.active ? "bg-card ring-border/60" : "bg-muted/30 ring-border/40")}>
                <span className="flex flex-col">
                  <button type="button" disabled={i === 0} onClick={() => void move(i, -1)} className="rounded p-0.5 text-muted-foreground hover:text-foreground disabled:opacity-25" aria-label="Yukarı"><ArrowUp className="size-3.5" /></button>
                  <button type="button" disabled={i === shown.length - 1} onClick={() => void move(i, 1)} className="rounded p-0.5 text-muted-foreground hover:text-foreground disabled:opacity-25" aria-label="Aşağı"><ArrowDown className="size-3.5" /></button>
                </span>
                <div className="min-w-0 flex-1 space-y-1.5">
                  <p className={cn("text-sm font-semibold", !r.active && "text-muted-foreground")}>{r.name}</p>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    <Word tone="sky">{TRIGGER_WORD[r.trigger] ?? r.trigger}</Word>
                    {r.conditions.filter((c) => c.kind !== "after_minutes").map((c, k) => <span key={k}> ve <Word tone="amber">{condText(c)}</Word></span>)}
                    {r.trigger === "no_reply" && <span> ({r.conditions.find((c) => c.kind === "after_minutes")?.value ?? "?"} dk)</span>}
                    <span> → </span>
                    {r.actions.map((a, k) => <span key={k}>{k > 0 && ", "}<Word tone="emerald">{ACTIONS[a.kind] ?? a.kind}</Word></span>)}
                  </p>
                  <div className="flex flex-wrap items-center gap-2 text-[0.68rem] text-muted-foreground">
                    {channels.length > 1 && <DeviceChips channels={channels} ids={r.channelIds} />}
                    <span>{r.runs > 0 ? `${r.runs} kez çalıştı` : "Henüz çalışmadı"}{r.lastRunAt ? `, en son ${since(r.lastRunAt, now)} önce` : ""}</span>
                    {r.cooldownMin > 0 && <span>· aynı müşteriye {r.cooldownMin >= 60 ? `${Math.round(r.cooldownMin / 60)} saatte` : `${r.cooldownMin} dakikada`} bir</span>}
                  </div>
                </div>
                <span className="flex shrink-0 items-center gap-1">
                  <span data-tip={r.active ? "Açık" : "Kapalı"}><Switch on={r.active} onChange={() => void toggle(r)} label="Açık" /></span>
                  <button type="button" data-tip="Düzenle" onClick={() => setEdit({ ...r })} className="flex size-8 items-center justify-center rounded-xl text-muted-foreground hover:bg-accent hover:text-foreground"><Pencil className="size-3.5" /></button>
                  <button type="button" data-tip="Sil" onClick={() => setDel(r)} className="flex size-8 items-center justify-center rounded-xl text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-3.5" /></button>
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card title="Hazır örnekler" icon={Sparkles}>
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
          {RECIPES.map((x) => (
            <button key={x.title} type="button" onClick={() => setEdit(x.make(device ? [device] : allIds.slice(0, 1)))} className="rounded-2xl bg-muted/30 p-3 text-left ring-1 ring-border/50 transition-colors hover:bg-accent/60">
              <p className="text-sm font-medium">{x.title}</p>
              <p className="mt-0.5 text-xs text-muted-foreground">{x.sub}</p>
            </button>
          ))}
        </div>
      </Card>

      {edit && <RuleForm draft={edit} channels={channels} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); void load(); }} />}
      {copy && <CopyRules channels={channels} onClose={() => setCopy(false)} onDone={(n) => { setCopy(false); setMsg(null); setInfo(`${n} kural kopyalandı. Kopyalar kapalı olarak eklendi, kontrol edip açabilirsiniz.`); void load(); }} />}
      <ConfirmDialog open={!!del} title="Kural silinsin mi?" description={`"${del?.name}" silinir.`} confirmLabel="Sil" onCancel={() => setDel(null)}
        onConfirm={() => del && void waApi.deleteRule(del.id).then(() => { setDel(null); void load(); })} />
    </div>
  );
}

function condText(c: WARuleCondition): string {
  const d = CONDITIONS[c.kind];
  if (!d) return c.kind;
  if (c.kind === "status_is") return `durum ${STATUS_WORD[c.value] ?? c.value}`;
  if (d.short) return `${d.short}: ${c.value}`;
  return d.label;
}

function Word({ tone, children }: { tone: "sky" | "amber" | "emerald"; children: React.ReactNode }) {
  const t = { sky: "bg-sky-500/10 text-sky-700 dark:text-sky-400", amber: "bg-amber-500/12 text-amber-700 dark:text-amber-400", emerald: "bg-emerald-500/12 text-emerald-700 dark:text-emerald-400" }[tone];
  return <span className={cn("rounded-md px-1.5 py-px font-medium", t)}>{children}</span>;
}

function DeviceFilter({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} className={cn("rounded-full px-3 py-1 text-xs font-medium ring-1 transition-colors", active ? "bg-primary text-primary-foreground ring-primary" : "text-muted-foreground ring-border/60 hover:bg-accent")}>{children}</button>;
}

function RuleForm({ draft, channels, onClose, onSaved }: { draft: Draft & { id?: number }; channels: WAChannel[]; onClose: () => void; onSaved: () => void }) {
  const [d, setD] = useState<Draft>(() => structuredClone({ ...draft, id: undefined }) as Draft);
  const [teams, setTeams] = useState<WATeam[]>([]);
  const [people, setPeople] = useState<WAAgent[]>([]);
  const [templates, setTemplates] = useState<WATemplate[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    waApi.teams().then(setTeams).catch(() => setTeams([]));
    waApi.agents().then(setPeople).catch(() => setPeople([]));
  }, []);
  const tplChannel = d.channelIds[0] ?? channels[0]?.id ?? 0;
  useEffect(() => {
    if (tplChannel) waApi.templates(tplChannel).then((t) => setTemplates(t.filter((x) => x.status === "APPROVED"))).catch(() => setTemplates([]));
  }, [tplChannel]);

  const minutes = d.conditions.find((c) => c.kind === "after_minutes")?.value ?? "";
  const setMinutes = (v: string) => setD((c) => ({ ...c, conditions: [...c.conditions.filter((x) => x.kind !== "after_minutes"), { kind: "after_minutes", value: v }] }));
  const conds = d.conditions.filter((c) => c.kind !== "after_minutes");
  const setCond = (i: number, patch: Partial<WARuleCondition>) => setD((c) => {
    const others = c.conditions.filter((x) => x.kind === "after_minutes");
    const list = c.conditions.filter((x) => x.kind !== "after_minutes").map((x, j) => (j === i ? { ...x, ...patch } : x));
    return { ...c, conditions: [...list, ...others] };
  });
  const removeCond = (i: number) => setD((c) => {
    const others = c.conditions.filter((x) => x.kind === "after_minutes");
    return { ...c, conditions: [...c.conditions.filter((x) => x.kind !== "after_minutes").filter((_, j) => j !== i), ...others] };
  });
  const setAct = (i: number, patch: Partial<WARuleAction>) => setD((c) => ({ ...c, actions: c.actions.map((x, j) => (j === i ? { ...x, ...patch } : x)) }));

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const body = { ...d, conditions: d.trigger === "no_reply" ? d.conditions : d.conditions.filter((c) => c.kind !== "after_minutes") };
      await waApi.saveRule(draft.id ?? 0, body);
      onSaved();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={draft.id ? "Kuralı düzenle" : "Yeni kural"} size="lg" footer={<>
      {error && <span className="mr-auto max-w-md text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button onClick={() => void save()} disabled={busy || !d.name.trim() || d.actions.length === 0}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
    </>}>
      <div className="space-y-5">
        <FormField label="Kuralın adı"><input className={inputCls} value={d.name} onChange={(e) => setD((c) => ({ ...c, name: e.target.value }))} placeholder="Mesai dışı cevabı" /></FormField>

        <Step n={1} title="Ne zaman">
          <div className="grid gap-1.5 sm:grid-cols-2">
            {Object.entries(TRIGGER_WORD).map(([k, l]) => (
              <button key={k} type="button" onClick={() => setD((c) => ({ ...c, trigger: k }))} className={cn("rounded-xl px-3 py-2 text-left text-sm ring-1 transition-colors", d.trigger === k ? "bg-sky-500/10 font-medium text-sky-800 ring-sky-500/40 dark:text-sky-300" : "ring-border/60 hover:bg-accent/60")}>{l}</button>
            ))}
          </div>
          {d.trigger === "no_reply" && (
            <div className="mt-2 flex items-center gap-2 text-sm">
              <input type="number" min={1} className={cn(inputCls, "h-9 w-24")} value={minutes} onChange={(e) => setMinutes(e.target.value)} />
              <span className="text-muted-foreground">dakika cevap alamazsa (mesai saatleri içinde sayılır)</span>
            </div>
          )}
        </Step>

        <Step n={2} title="Şu şartlarda (isteğe bağlı)">
          <div className="space-y-2">
            {conds.map((c, i) => (
              <div key={i} className="flex flex-wrap items-center gap-2 rounded-xl bg-amber-500/6 p-2 ring-1 ring-amber-500/20">
                <select className={cn(inputCls, "h-9 min-w-56 flex-1")} value={c.kind} onChange={(e) => setCond(i, { kind: e.target.value, value: "" })}>
                  {Object.entries(CONDITIONS).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
                </select>
                {CONDITIONS[c.kind]?.needs === "text" && <input className={cn(inputCls, "h-9 min-w-40 flex-1")} value={c.value} onChange={(e) => setCond(i, { value: e.target.value })} placeholder={CONDITIONS[c.kind].placeholder} />}
                {CONDITIONS[c.kind]?.needs === "status" && (
                  <select className={cn(inputCls, "h-9 w-48")} value={c.value} onChange={(e) => setCond(i, { value: e.target.value })}>
                    <option value="">Seçin</option>
                    {Object.entries(STATUS_WORD).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                  </select>
                )}
                <button type="button" onClick={() => removeCond(i)} className="rounded-lg p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive" aria-label="Kaldır"><X className="size-4" /></button>
              </div>
            ))}
            <button type="button" onClick={() => setD((c) => ({ ...c, conditions: [...c.conditions, { kind: "hours_open", value: "" }] }))} className="flex h-8 items-center gap-1 rounded-full px-3 text-xs font-semibold text-amber-700 ring-1 ring-amber-500/30 hover:bg-amber-500/10 dark:text-amber-400"><Plus className="size-3.5" /> Şart ekle</button>
            {conds.length > 1 && <p className="text-[0.7rem] text-muted-foreground">Bütün şartlar birlikte tutmalı.</p>}
          </div>
        </Step>

        <Step n={3} title="Şunu yap">
          <div className="space-y-2">
            {d.actions.map((a, i) => (
              <div key={i} className="space-y-2 rounded-xl bg-emerald-500/6 p-2 ring-1 ring-emerald-500/20">
                <div className="flex items-center gap-2">
                  <select className={cn(inputCls, "h-9 flex-1")} value={a.kind} onChange={(e) => setAct(i, { kind: e.target.value, text: "", value: "", templateId: undefined, params: [], teamId: undefined, userId: undefined, url: "" })}>
                    {Object.entries(ACTIONS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                  </select>
                  <button type="button" onClick={() => setD((c) => ({ ...c, actions: c.actions.filter((_, j) => j !== i) }))} className="rounded-lg p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive" aria-label="Kaldır"><X className="size-4" /></button>
                </div>
                <ActionFields a={a} set={(p) => setAct(i, p)} teams={teams} people={people} templates={templates} />
              </div>
            ))}
            <button type="button" onClick={() => setD((c) => ({ ...c, actions: [...c.actions, { kind: "add_tag", value: "" }] }))} className="flex h-8 items-center gap-1 rounded-full px-3 text-xs font-semibold text-emerald-700 ring-1 ring-emerald-500/30 hover:bg-emerald-500/10 dark:text-emerald-400"><Plus className="size-3.5" /> Bir iş daha ekle</button>
          </div>
        </Step>

        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Aynı müşteriye en fazla ne sıklıkla" hint="0 her seferinde çalışır. 720 dakika = 12 saatte bir.">
            <div className="flex items-center gap-2">
              <input type="number" min={0} className={cn(inputCls, "w-28")} value={d.cooldownMin} onChange={(e) => setD((c) => ({ ...c, cooldownMin: Math.max(0, Number(e.target.value) || 0) }))} />
              <span className="text-sm text-muted-foreground">dakikada bir</span>
            </div>
          </FormField>
          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">Hangi cihazlarda çalışsın</p>
            <DevicePicker channels={channels} value={d.channelIds} onChange={(v) => setD((c) => ({ ...c, channelIds: v }))} />
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={d.active} onChange={(e) => setD((c) => ({ ...c, active: e.target.checked }))} /> Kural açık</label>
      </div>
    </Modal>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <section className="flex gap-3">
      <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-foreground/85 text-[0.7rem] font-bold text-background">{n}</span>
      <div className="min-w-0 flex-1 space-y-2">
        <p className="text-sm font-semibold">{title}</p>
        {children}
      </div>
    </section>
  );
}

function ActionFields({ a, set, teams, people, templates }: { a: WARuleAction; set: (p: Partial<WARuleAction>) => void; teams: WATeam[]; people: WAAgent[]; templates: WATemplate[] }) {
  const vars = <span className="text-[0.7rem] text-muted-foreground"><code className="font-mono">{"{musteri}"}</code> müşterinin adı, <code className="font-mono">{"{numara}"}</code> numarası, <code className="font-mono">{"{sohbet}"}</code> sohbet numarası olur.</span>;
  switch (a.kind) {
    case "send_text":
    case "note":
      return <><textarea className={areaCls} rows={3} value={a.text ?? ""} onChange={(e) => set({ text: e.target.value })} placeholder={a.kind === "note" ? "Ekibin göreceği not" : "Müşteriye gidecek mesaj"} />{vars}</>;
    case "send_template": {
      const t = templates.find((x) => x.id === a.templateId);
      const n = t ? Math.max(0, ...[...(t.components.find((c) => c.type === "BODY")?.text ?? "").matchAll(/\{\{(\d+)\}\}/g)].map((m) => Number(m[1]))) : 0;
      return (
        <>
          <select className={cn(inputCls, "h-9")} value={a.templateId ?? ""} onChange={(e) => set({ templateId: Number(e.target.value) || undefined, params: [] })}>
            <option value="">Şablon seçin</option>
            {templates.map((x) => <option key={x.id} value={x.id}>{x.name} ({x.language})</option>)}
          </select>
          {t && <p className="rounded-lg bg-card px-2 py-1.5 text-xs text-muted-foreground">{t.components.find((c) => c.type === "BODY")?.text}</p>}
          {Array.from({ length: n }).map((_, k) => (
            <label key={k} className="flex items-center gap-2">
              <code className="w-10 font-mono text-xs text-muted-foreground">{`{{${k + 1}}}`}</code>
              <input className={cn(inputCls, "h-9")} value={a.params?.[k] ?? ""} onChange={(e) => { const p = [...(a.params ?? [])]; p[k] = e.target.value; set({ params: p }); }} placeholder={k === 0 ? "{musteri}" : ""} />
            </label>
          ))}
          {n > 0 && vars}
        </>
      );
    }
    case "assign_team":
      return (
        <select className={cn(inputCls, "h-9")} value={a.teamId ?? ""} onChange={(e) => set({ teamId: Number(e.target.value) || undefined })}>
          <option value="">Ekip seçin</option>
          {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </select>
      );
    case "assign_user":
      return (
        <>
          <select className={cn(inputCls, "h-9")} value={a.userId ?? ""} onChange={(e) => set({ userId: Number(e.target.value) || undefined })}>
            <option value="">Kişi seçin</option>
            {people.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
          <p className="text-[0.7rem] text-muted-foreground">Sohbet zaten birindeyse ondan alınmaz.</p>
        </>
      );
    case "add_tag":
    case "set_category":
      return <input className={cn(inputCls, "h-9")} value={a.value ?? ""} onChange={(e) => set({ value: e.target.value })} placeholder={a.kind === "add_tag" ? "etiket" : "konu, örn. Fatura"} />;
    case "set_priority":
      return (
        <select className={cn(inputCls, "h-9")} value={a.value ?? ""} onChange={(e) => set({ value: e.target.value })}>
          <option value="">Seçin</option>
          {Object.entries(PRIORITY_WORD).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </select>
      );
    case "webhook":
      return <><input className={cn(inputCls, "h-9 font-mono")} value={a.url ?? ""} onChange={(e) => set({ url: e.target.value })} placeholder="https://..." /><p className="text-[0.7rem] text-muted-foreground">Bu adrese sohbet numarası, cihaz, müşteri numarası ve durum JSON olarak gönderilir.</p></>;
    case "resolve":
      return <p className="flex items-center gap-1.5 text-[0.7rem] text-muted-foreground"><Bot className="size-3.5" /> Sohbet sessizce kapanır. Müşteri yeniden yazarsa açılır.</p>;
    default:
      return null;
  }
}

function CopyRules({ channels, onClose, onDone }: { channels: WAChannel[]; onClose: () => void; onDone: (n: number) => void }) {
  const [from, setFrom] = useState(channels[0]?.id ?? 0);
  const [to, setTo] = useState(channels[1]?.id ?? 0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  return (
    <Modal open onClose={onClose} title="Kuralları başka cihaza kopyala" description="Kaynaktaki kuralların birer kopyası hedef cihaza kapalı olarak eklenir. İki cihazın kuralları bundan sonra birbirinden bağımsızdır." footer={<>
      {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button disabled={busy || from === to} onClick={() => { setBusy(true); waApi.copyToChannel(from, to, "rules").then((r) => onDone(r.copied)).catch((e) => setError(e instanceof ApiError ? e.message : "Kopyalanamadı.")).finally(() => setBusy(false)); }}>{busy ? "Kopyalanıyor..." : "Kopyala"}</Button>
    </>}>
      <div className="grid gap-3 sm:grid-cols-2">
        <FormField label="Nereden"><select className={inputCls} value={from} onChange={(e) => setFrom(Number(e.target.value))}>{channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</select></FormField>
        <FormField label="Nereye"><select className={inputCls} value={to} onChange={(e) => setTo(Number(e.target.value))}>{channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</select></FormField>
      </div>
    </Modal>
  );
}
