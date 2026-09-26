// NodeEditor is the side panel for the selected box: its text, choices,
// questions and rules, in the words the customer will see.

import { useRef } from "react";
import { ArrowDown, ArrowUp, Copy, Plus, Trash2, X } from "lucide-react";
import FileUpload from "@/components/whatsapp/FileUpload";
import { areaCls, FormField, inputCls, Words } from "@/components/whatsapp/settings/parts";
import { cn } from "@/lib/utils";
import type { BotData, BotNode, WAIntegration, WATeam } from "@/whatsapp/types";
import { KINDS, OP_WORD, rid } from "@/components/whatsapp/bot/nodes";
import { PRIORITY_WORD } from "@/whatsapp/util";

export default function NodeEditor({
  node,
  set,
  vars,
  teams,
  integrations,
  readOnly,
  onDelete,
  onDuplicate,
  onClose,
}: {
  node: BotNode;
  set: (patch: Partial<BotData>, key: string) => void;
  vars: string[];
  teams: WATeam[];
  integrations: WAIntegration[];
  readOnly: boolean;
  onDelete: () => void;
  onDuplicate: () => void;
  onClose: () => void;
}) {
  const k = KINDS[node.type];
  const d = node.data;
  return (
    <aside className="flex h-full w-full flex-col bg-card">
      <header className="flex items-center gap-2.5 border-b border-border/60 px-4 py-3">
        <span className={cn("flex size-8 items-center justify-center rounded-xl", k.chip)}><k.icon className="size-4" /></span>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold">{k.label}</p>
          <p className="truncate text-[0.7rem] text-muted-foreground">{k.hint}</p>
        </div>
        <button type="button" onClick={onClose} aria-label="Kapat" className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-4" /></button>
      </header>
      <fieldset disabled={readOnly} className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        {node.type === "start" && <Info>Müşteri yazınca akış buradan başlar. Başlangıçtan çıkan oku ilk kutuya bağlayın. Müşterinin adı <V>{"{musteri}"}</V>, numarası <V>{"{numara}"}</V> olarak her yerde kullanılabilir.</Info>}

        {node.type === "message" && (
          <>
            <TextWithVars label="Mesaj" value={d.text ?? ""} onChange={(v) => set({ text: v }, "text")} vars={vars} rows={5} placeholder="Merhaba {musteri}, hoş geldiniz!" />
            <MediaFields d={d} set={set} />
          </>
        )}

        {node.type === "menu" && <MenuFields d={d} set={set} vars={vars} />}

        {node.type === "ask" && (
          <>
            <TextWithVars label="Soru" value={d.text ?? ""} onChange={(v) => set({ text: v }, "text")} vars={vars} rows={3} placeholder="Sipariş numaranızı yazar mısınız?" />
            <FormField label="Cevap hangi isimle saklansın" hint="Sonraki kutularda {bu_isim} yazarak cevabı kullanabilirsiniz.">
              <span className="relative block">
                <span className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 font-mono text-sm text-muted-foreground">{"{"}</span>
                <input className={cn(inputCls, "px-6 font-mono")} value={d.var ?? ""} onChange={(e) => set({ var: cleanVar(e.target.value) }, "var")} placeholder="siparis" />
                <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 font-mono text-sm text-muted-foreground">{"}"}</span>
              </span>
            </FormField>
            <FormField label="Nasıl bir cevap bekleniyor">
              <select className={inputCls} value={d.validate || "any"} onChange={(e) => set({ validate: e.target.value }, "validate")}>
                <option value="any">Herhangi bir yazı</option>
                <option value="number">Sadece rakam</option>
                <option value="email">E-posta adresi</option>
                <option value="phone">Telefon numarası</option>
              </select>
            </FormField>
            <RetryField d={d} set={set} ask />
          </>
        )}

        {node.type === "condition" && (
          <>
            <div className="flex gap-1 rounded-xl bg-muted/60 p-1">
              <button type="button" onClick={() => set({ match: "all" }, "match")} className={cn("flex-1 rounded-lg px-2 py-1 text-xs font-medium", d.match !== "any" ? "bg-card shadow-sm" : "text-muted-foreground")}>Hepsi tutarsa</button>
              <button type="button" onClick={() => set({ match: "any" }, "match")} className={cn("flex-1 rounded-lg px-2 py-1 text-xs font-medium", d.match === "any" ? "bg-card shadow-sm" : "text-muted-foreground")}>Biri tutarsa</button>
            </div>
            {(d.rules ?? []).map((r, i) => {
              const setRule = (p: Partial<typeof r>) => set({ rules: (d.rules ?? []).map((x, j) => (j === i ? { ...x, ...p } : x)) }, `rule${i}`);
              const timeRule = r.op === "hours_open" || r.op === "hours_closed";
              return (
                <div key={i} className="space-y-2 rounded-2xl bg-orange-500/6 p-3 ring-1 ring-orange-500/20">
                  <div className="flex items-center gap-2">
                    <select className={cn(inputCls, "h-9 flex-1")} value={timeRule ? r.op : "var"} onChange={(e) => setRule(e.target.value === "var" ? { op: "equals", var: vars[0] ?? "" } : { op: e.target.value, var: "", value: "" })}>
                      <option value="hours_open">Mesai içindeyse</option>
                      <option value="hours_closed">Mesai dışındaysa</option>
                      <option value="var">Bir bilgiye göre</option>
                    </select>
                    <button type="button" onClick={() => set({ rules: (d.rules ?? []).filter((_, j) => j !== i) }, "rules")} className="rounded-lg p-1.5 text-muted-foreground hover:text-destructive" aria-label="Kaldır"><X className="size-4" /></button>
                  </div>
                  {!timeRule && (
                    <div className="grid grid-cols-2 gap-2">
                      <select className={cn(inputCls, "h-9 font-mono text-xs")} value={r.var} onChange={(e) => setRule({ var: e.target.value })}>
                        <option value="">Bilgi seçin</option>
                        {vars.map((v) => <option key={v} value={v}>{`{${v}}`}</option>)}
                      </select>
                      <select className={cn(inputCls, "h-9")} value={r.op} onChange={(e) => setRule({ op: e.target.value })}>
                        {Object.entries(OP_WORD).map(([o, w]) => <option key={o} value={o}>{w}</option>)}
                      </select>
                      {r.op !== "exists" && r.op !== "empty" && <input className={cn(inputCls, "col-span-2 h-9")} value={r.value} onChange={(e) => setRule({ value: e.target.value })} placeholder="değer" />}
                    </div>
                  )}
                </div>
              );
            })}
            <AddBtn onClick={() => set({ rules: [...(d.rules ?? []), { var: vars[0] ?? "", op: "equals", value: "" }] }, "rules")}>Şart ekle</AddBtn>
          </>
        )}

        {node.type === "api" && (
          <>
            <FormField label="Hangi sisteme sorulsun" hint="Bağlantıları Ayarlar > Dış sistemler'den eklersiniz.">
              <select className={inputCls} value={d.integration ?? ""} onChange={(e) => set({ integration: Number(e.target.value) || undefined }, "integration")}>
                <option value="">Seçin</option>
                {integrations.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
              </select>
            </FormField>
            <div className="space-y-2">
              <p className="text-xs font-medium text-muted-foreground">Cevaptan alınacak bilgiler</p>
              {(d.map ?? []).map((m, i) => (
                <div key={i} className="flex items-center gap-1.5">
                  <input className={cn(inputCls, "h-9 flex-1 font-mono text-xs")} value={m.path} onChange={(e) => set({ map: (d.map ?? []).map((x, j) => (j === i ? { ...x, path: e.target.value } : x)) }, `map${i}`)} placeholder="data.durum" />
                  <span className="text-xs text-muted-foreground">→</span>
                  <input className={cn(inputCls, "h-9 w-28 font-mono text-xs")} value={m.var} onChange={(e) => set({ map: (d.map ?? []).map((x, j) => (j === i ? { ...x, var: cleanVar(e.target.value) } : x)) }, `mapv${i}`)} placeholder="durum" />
                  <button type="button" onClick={() => set({ map: (d.map ?? []).filter((_, j) => j !== i) }, "map")} className="rounded-lg p-1 text-muted-foreground hover:text-destructive" aria-label="Kaldır"><X className="size-4" /></button>
                </div>
              ))}
              <AddBtn onClick={() => set({ map: [...(d.map ?? []), { var: "", path: "" }] }, "map")}>Bilgi ekle</AddBtn>
              <Info>Soldaki cevaptaki yeri, sağdaki bu bilginin adını gösterir. Örneğin cevap {`{"data": {"durum": "kargoda"}}`} ise <V>data.durum</V> → <V>durum</V> yazın, sonra mesajda <V>{"{durum}"}</V> kullanın. Liste içinde <V>items.0.ad</V> gibi sıra numarası verilebilir.</Info>
            </div>
          </>
        )}

        {node.type === "tag" && (
          <>
            <FormField label="Etiketler"><Words values={d.tags ?? []} onChange={(v) => set({ tags: v }, "tags")} placeholder="iade, kargo" disabled={readOnly} /></FormField>
            <FormField label="Öncelik">
              <select className={inputCls} value={d.priority ?? ""} onChange={(e) => set({ priority: e.target.value }, "priority")}>
                <option value="">Değiştirme</option>
                {Object.entries(PRIORITY_WORD).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
              </select>
            </FormField>
            <FormField label="Konu" hint="Seçim sonucu yazmak için değişken kullanabilirsiniz, örn. {konu}."><input className={inputCls} value={d.category ?? ""} onChange={(e) => set({ category: e.target.value }, "category")} placeholder="Fatura" /></FormField>
          </>
        )}

        {node.type === "handoff" && (
          <>
            <TextWithVars label="Müşteriye son mesaj (isteğe bağlı)" value={d.text ?? ""} onChange={(v) => set({ text: v }, "text")} vars={vars} rows={3} />
            <FormField label="Hangi ekibe" hint="Seçmezseniz numarada çalışan müsait kişiye verilir.">
              <select className={inputCls} value={d.teamId ?? ""} onChange={(e) => set({ teamId: Number(e.target.value) || undefined }, "teamId")}>
                <option value="">Müsait olan herhangi biri</option>
                {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
              </select>
            </FormField>
            <TextWithVars label="Temsilciye not (müşteri görmez)" value={d.note ?? ""} onChange={(v) => set({ note: v }, "note")} vars={vars} rows={3} placeholder="Konu: {konu}, sipariş: {siparis}" />
          </>
        )}

        {node.type === "callback" && (
          <>
            <TextWithVars label="Müşteriye mesaj" value={d.text ?? ""} onChange={(v) => set({ text: v }, "text")} vars={vars} rows={3} />
            <TextWithVars label="Talebe eklenecek not" value={d.note ?? ""} onChange={(v) => set({ note: v }, "note")} vars={vars} rows={2} placeholder="{konu} için aranmak istiyor" />
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={!!d.resolve} onChange={(e) => set({ resolve: e.target.checked }, "resolve")} /> Başka kutu bağlı değilse sohbeti kapat</label>
            <Info>Talep, Geri Arama Talepleri sayfasına düşer ve yetkili kişilere haber verilir.</Info>
          </>
        )}

        {node.type === "survey" && <Info>Müşteriye 1'den 5'e puan listesi gider. Puan sohbetin kaydına yazılır. Başka kutu bağlı değilse chatbot biter ve sohbet kapanır.</Info>}

        {node.type === "end" && (
          <>
            <TextWithVars label="Son mesaj (isteğe bağlı)" value={d.text ?? ""} onChange={(v) => set({ text: v }, "text")} vars={vars} rows={3} placeholder="Görüşmek üzere, iyi günler!" />
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={!!d.resolve} onChange={(e) => set({ resolve: e.target.checked }, "resolve")} /> Sohbeti de çözüldü olarak kapat</label>
            <Info>Kapatmazsanız sohbet açık kalır ve temsilcilerin önüne düşer.</Info>
          </>
        )}
      </fieldset>
      {!readOnly && node.type !== "start" && (
        <footer className="flex items-center gap-2 border-t border-border/60 px-4 py-3">
          <button type="button" onClick={onDuplicate} className="flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-foreground"><Copy className="size-3.5" /> Çoğalt</button>
          <button type="button" onClick={onDelete} className="ml-auto flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium text-destructive hover:bg-destructive/10"><Trash2 className="size-3.5" /> Kutuyu sil</button>
        </footer>
      )}
    </aside>
  );
}

function MediaFields({ d, set }: { d: BotData; set: (p: Partial<BotData>, key: string) => void }) {
  const on = !!d.fileId || !!d.mediaUrl || (d.mediaKind !== undefined && d.mediaKind !== "");
  const byLink = !d.fileId && d.mediaUrl !== undefined && d.mediaUrl !== "" ? true : d.mediaKind === "link";
  return (
    <div className="space-y-2.5 rounded-2xl bg-muted/30 p-3">
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" className="size-4 accent-primary" checked={on} onChange={(e) => set(e.target.checked ? { mediaKind: "image", mediaUrl: "", fileId: undefined, fileName: undefined } : { mediaKind: "", mediaUrl: "", fileId: undefined, fileName: undefined }, "media")} />
        Görsel, video ya da belge de gönder
      </label>
      {on && (
        <>
          <div className="flex gap-1 rounded-xl bg-muted/60 p-1">
            <button type="button" onClick={() => set({ mediaKind: d.fileId ? d.mediaKind : "image", mediaUrl: "" }, "mediaMode")} className={cn("flex-1 rounded-lg px-2 py-1 text-xs font-medium", !byLink ? "bg-card shadow-sm" : "text-muted-foreground")}>Bilgisayardan yükle</button>
            <button type="button" onClick={() => set({ mediaKind: "link", fileId: undefined, fileName: undefined }, "mediaMode")} className={cn("flex-1 rounded-lg px-2 py-1 text-xs font-medium", byLink ? "bg-card shadow-sm" : "text-muted-foreground")}>İnternet adresinden</button>
          </div>
          {!byLink ? (
            <FileUpload
              value={d.fileId ? { id: d.fileId, name: d.fileName ?? "dosya", kind: d.mediaKind } : null}
              onChange={(f) => set(f ? { fileId: f.id, fileName: f.name, mediaKind: f.kind ?? "document", mediaUrl: "" } : { fileId: undefined, fileName: undefined, mediaKind: "image" }, "file")}
              accept="image/jpeg,image/png,video/mp4,application/pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt"
              hint="Görsel en fazla 5 MB, video 16 MB, belge 100 MB. Mesaj metni dosyanın altında açıklama olarak gider."
            />
          ) : (
            <>
              <div className="flex gap-1 rounded-xl bg-muted/60 p-1">
                {([["image", "Görsel"], ["video", "Video"], ["document", "Belge"]] as const).map(([v, l]) => {
                  const kind = d.mediaKind === "link" || !d.mediaKind ? "image" : d.mediaKind;
                  return <button key={v} type="button" onClick={() => set({ mediaKind: v }, "mediaKind")} className={cn("flex-1 rounded-lg px-2 py-1 text-xs font-medium", kind === v ? "bg-card shadow-sm" : "text-muted-foreground")}>{l}</button>;
                })}
              </div>
              <FormField label="Dosyanın internet adresi" hint="Herkesin açabildiği bir https adresi olmalı.">
                <input className={cn(inputCls, "font-mono text-xs")} value={d.mediaUrl ?? ""} onChange={(e) => set({ mediaUrl: e.target.value, mediaKind: !e.target.value ? "link" : d.mediaKind === "link" ? "image" : d.mediaKind }, "mediaUrl")} placeholder="https://..." />
              </FormField>
            </>
          )}
        </>
      )}
    </div>
  );
}

function MenuFields({ d, set, vars }: { d: BotData; set: (p: Partial<BotData>, key: string) => void; vars: string[] }) {
  const list = d.style === "list";
  const opts = d.options ?? [];
  const max = list ? 10 : 3;
  const limit = list ? 24 : 20;
  const setOpt = (i: number, p: Partial<(typeof opts)[number]>) => set({ options: opts.map((o, j) => (j === i ? { ...o, ...p } : o)) }, `opt${i}`);
  const move = (i: number, dir: -1 | 1) => {
    const j = i + dir;
    if (j < 0 || j >= opts.length) return;
    const n = [...opts];
    [n[i], n[j]] = [n[j], n[i]];
    set({ options: n }, "options");
  };
  return (
    <>
      <TextWithVars label="Soru" value={d.text ?? ""} onChange={(v) => set({ text: v }, "text")} vars={vars} rows={3} placeholder="Hangi konuda yardım istersiniz?" />
      <div className="space-y-1.5">
        <p className="text-xs font-medium text-muted-foreground">Görünüm</p>
        <div className="grid grid-cols-2 gap-1.5">
          <button type="button" onClick={() => set({ style: "buttons" }, "style")} className={cn("rounded-xl px-3 py-2 text-left ring-1", !list ? "bg-primary/10 ring-primary/40" : "ring-border/60 hover:bg-accent/60")}>
            <span className="block text-sm font-medium">Düğmeler</span>
            <span className="block text-[0.68rem] text-muted-foreground">En fazla 3 seçenek, tek dokunuş</span>
          </button>
          <button type="button" onClick={() => set({ style: "list" }, "style")} className={cn("rounded-xl px-3 py-2 text-left ring-1", list ? "bg-primary/10 ring-primary/40" : "ring-border/60 hover:bg-accent/60")}>
            <span className="block text-sm font-medium">Liste</span>
            <span className="block text-[0.68rem] text-muted-foreground">En fazla 10 seçenek, açılır liste</span>
          </button>
        </div>
      </div>
      {list && <FormField label="Listeyi açan düğmenin yazısı"><input className={inputCls} maxLength={20} value={d.buttonLabel ?? ""} onChange={(e) => set({ buttonLabel: e.target.value }, "buttonLabel")} placeholder="Seçenekler" /></FormField>}
      <div className="space-y-1.5">
        <p className="flex items-center text-xs font-medium text-muted-foreground">Seçenekler <span className={cn("ml-auto tabular-nums", opts.length > max && "text-destructive")}>{opts.length} / {max}</span></p>
        {opts.map((o, i) => (
          <div key={o.id} className="space-y-1 rounded-xl bg-violet-500/6 p-2 ring-1 ring-violet-500/20">
            <div className="flex items-center gap-1">
              <span className="w-5 text-center text-xs font-semibold text-muted-foreground tabular-nums">{i + 1}</span>
              <input className={cn(inputCls, "h-8 flex-1", o.label.length > limit && "border-destructive")} value={o.label} onChange={(e) => setOpt(i, { label: e.target.value })} placeholder="Seçenek" />
              <button type="button" onClick={() => move(i, -1)} disabled={i === 0} className="rounded p-1 text-muted-foreground disabled:opacity-25" aria-label="Yukarı"><ArrowUp className="size-3.5" /></button>
              <button type="button" onClick={() => move(i, 1)} disabled={i === opts.length - 1} className="rounded p-1 text-muted-foreground disabled:opacity-25" aria-label="Aşağı"><ArrowDown className="size-3.5" /></button>
              <button type="button" onClick={() => set({ options: opts.filter((_, j) => j !== i) }, "options")} className="rounded p-1 text-muted-foreground hover:text-destructive" aria-label="Kaldır"><X className="size-3.5" /></button>
            </div>
            {list && <input className={cn(inputCls, "ml-6 h-8 w-[calc(100%-1.5rem)] text-xs")} maxLength={72} value={o.description ?? ""} onChange={(e) => setOpt(i, { description: e.target.value })} placeholder="Kısa açıklama (isteğe bağlı)" />}
            {o.label.length > limit && <p className="ml-6 text-[0.65rem] text-destructive">En fazla {limit} karakter olabilir.</p>}
          </div>
        ))}
        {opts.length < max && <AddBtn onClick={() => set({ options: [...opts, { id: rid(), label: "" }] }, "options")}>Seçenek ekle</AddBtn>}
        {!list && opts.length >= 3 && <p className="text-[0.7rem] text-muted-foreground">Daha fazla seçenek için Liste görünümünü seçin.</p>}
      </div>
      <FormField label="Seçilen cevap bir isimle saklansın (isteğe bağlı)" hint="Örn. konu yazarsanız sonraki kutularda {konu} ile kullanılır.">
        <input className={cn(inputCls, "font-mono")} value={d.var ?? ""} onChange={(e) => set({ var: cleanVar(e.target.value) }, "var")} placeholder="konu" />
      </FormField>
      <RetryField d={d} set={set} />
    </>
  );
}

function RetryField({ d, set, ask }: { d: BotData; set: (p: Partial<BotData>, key: string) => void; ask?: boolean }) {
  return (
    <FormField label="Anlaşılmazsa ne yazılsın" hint={'Müşteri iki kez anlaşılmayan bir şey yazarsa "Anlaşılmazsa" yolundan gider. O yol bağlı değilse temsilciye aktarılır.'}>
      <input className={inputCls} value={d.retry ?? ""} onChange={(e) => set({ retry: e.target.value }, "retry")} placeholder={ask ? "Anlayamadım, lütfen tekrar yazar mısınız?" : "Anlayamadım, lütfen seçeneklerden birini seçin."} />
    </FormField>
  );
}

function TextWithVars({ label, value, onChange, vars, rows, placeholder }: { label: string; value: string; onChange: (v: string) => void; vars: string[]; rows: number; placeholder?: string }) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const insert = (v: string) => {
    const el = ref.current;
    const tag = `{${v}}`;
    if (!el) return onChange(value + tag);
    const s = el.selectionStart ?? value.length, e = el.selectionEnd ?? value.length;
    onChange(value.slice(0, s) + tag + value.slice(e));
    requestAnimationFrame(() => { el.focus(); el.setSelectionRange(s + tag.length, s + tag.length); });
  };
  return (
    <div className="space-y-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <textarea ref={ref} className={areaCls} rows={rows} value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
      {vars.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {vars.map((v) => <button key={v} type="button" onClick={() => insert(v)} className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-[0.65rem] text-muted-foreground hover:bg-primary/10 hover:text-primary">{`{${v}}`}</button>)}
        </div>
      )}
    </div>
  );
}

function AddBtn({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} className="flex h-8 items-center gap-1 rounded-full px-3 text-xs font-semibold text-primary ring-1 ring-primary/30 hover:bg-primary/10"><Plus className="size-3.5" /> {children}</button>;
}

function Info({ children }: { children: React.ReactNode }) {
  return <p className="rounded-xl bg-muted/40 px-3 py-2 text-xs leading-relaxed text-muted-foreground">{children}</p>;
}

function V({ children }: { children: React.ReactNode }) {
  return <code className="rounded bg-muted px-1 font-mono text-[0.68rem] text-foreground">{children}</code>;
}

function cleanVar(s: string): string {
  return s.toLocaleLowerCase("tr").replace(/\s+/g, "_").replace(/[^a-z0-9_ğüşıöç]/g, "").slice(0, 32);
}
