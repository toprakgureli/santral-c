// WhatsAppRatings ("Puanlamalar"): every score customers gave, at the end
// of a WhatsApp conversation or after a phone call, with who they rated,
// what they wrote and a way into the conversation. Totals and the spread
// of scores sit on top, then each survey question on its own, then a table
// of each person by question, so it shows where each one should improve.
// Everything filters.

import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, ChevronRight, Download, ListChecks, MessageCircle, MessageSquareQuote, PhoneCall, Search, Star, ThumbsDown, TrendingDown, UsersRound, X } from "lucide-react";
import { ApiError } from "@/api/client";
import RangePicker, { useRange } from "@/components/RangePicker";
import { Button, Card } from "@/components/ui";
import { IconChip, Toolbar, type ChipTone } from "@/components/ui/rows";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WARating, WARatingFilter, WARatings } from "@/whatsapp/types";
import { prettyPhone } from "@/whatsapp/util";

const SCORE_TONE = ["", "bg-destructive/12 text-destructive", "bg-destructive/10 text-destructive", "bg-warning/14 text-warning", "bg-success/12 text-success", "bg-success/15 text-success"];
const SCORE_BAR = ["", "bg-destructive", "bg-destructive/70", "bg-warning", "bg-success/70", "bg-success"];
const SCORE_WORD = ["", "Çok kötü", "Kötü", "Orta", "İyi", "Çok iyi"];

const SINGLE = "Tek soruluk anket";
// A weakest question or area is only named once enough answers back it;
// with one or two answers it would be chance, not a pattern.
const MIN_FOR_WEAKEST = 5;

// avgTone colours an average: green when good, amber in the middle, red
// when it needs work.
function avgTone(v: number): string {
  if (v >= 4.5) return "bg-success/15 text-success";
  if (v >= 4) return "bg-success/10 text-success";
  if (v >= 3) return "bg-warning/14 text-warning";
  return "bg-destructive/12 text-destructive";
}

function talk(sec?: number): string {
  if (!sec) return "";
  return sec < 60 ? `${sec} sn görüşme` : `${Math.round(sec / 60)} dk görüşme`;
}

function when(iso: string): string {
  const d = new Date(iso);
  const today = new Date();
  const day = d.toDateString() === today.toDateString() ? "Bugün" : d.toLocaleDateString("tr-TR", { day: "numeric", month: "short", year: d.getFullYear() === today.getFullYear() ? undefined : "numeric" });
  return `${day} ${d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" })}`;
}

export function WhatsAppRatings() {
  const { preset, range, choose, setFrom, setTo } = useRange("last30");
  const [channels, setChannels] = useState<WAChannel[]>([]);
  const [channel, setChannel] = useState(0);
  const [agent, setAgent] = useState(0);
  const [source, setSource] = useState<"" | "chat" | "call">("");
  const [score, setScore] = useState("");
  const [comment, setComment] = useState(false);
  const [q, setQ] = useState("");
  const [search, setSearch] = useState("");
  const [data, setData] = useState<WARatings | null>(null);
  const [items, setItems] = useState<WARating[]>([]);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [more, setMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState<WARating | null>(null);
  const seq = useRef(0);

  useEffect(() => {
    waApi.channels().then(setChannels).catch(() => setChannels([]));
  }, []);
  useEffect(() => {
    const t = window.setTimeout(() => setSearch(q.trim()), 300);
    return () => window.clearTimeout(t);
  }, [q]);

  const filter = useMemo<WARatingFilter>(() => ({ from: range.from, to: range.to, channel, agent, source, score, comment, q: search }), [range.from, range.to, channel, agent, source, score, comment, search]);

  useEffect(() => {
    if (!filter.from || !filter.to) return;
    const my = ++seq.current;
    setLoading(true);
    setError(null);
    waApi.ratings({ ...filter, page: 1 }).then((r) => {
      if (my !== seq.current) return;
      setData(r);
      setItems(r.items);
      setPage(1);
    }).catch((e) => my === seq.current && setError(e instanceof ApiError ? e.message : "Puanlamalar alınamadı.")).finally(() => my === seq.current && setLoading(false));
  }, [filter]);

  const loadMore = async () => {
    setMore(true);
    try {
      const r = await waApi.ratings({ ...filter, page: page + 1 });
      setItems((cur) => [...cur, ...r.items]);
      setPage((p) => p + 1);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Alınamadı.");
    } finally {
      setMore(false);
    }
  };

  const filtered = !!(channel || agent || source || score || comment || search);
  const clear = () => {
    setChannel(0);
    setAgent(0);
    setSource("");
    setScore("");
    setComment(false);
    setQ("");
  };
  const peak = Math.max(1, ...(data?.dist ?? [0]));
  const low = (data?.dist[0] ?? 0) + (data?.dist[1] ?? 0);

  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <div className="flex items-center gap-3">
        <Link to="/whatsapp" data-tip="Gelen kutusuna dön" className="flex size-9 items-center justify-center rounded-xl bg-card text-muted-foreground shadow-sm ring-1 ring-border/60 hover:text-foreground"><ArrowLeft className="size-4" /></Link>
        <div className="min-w-0 flex-1">
          <h1 className="text-lg font-semibold tracking-tight">Puanlamalar</h1>
          <p className="text-xs text-muted-foreground">Müşterilerin WhatsApp sohbeti sonunda ve telefon görüşmesinden sonra verdiği bütün puanlar.</p>
        </div>
        <Button variant="secondary" onClick={() => void waApi.exportRatings(filter).catch((e) => setError(e instanceof ApiError ? e.message : "İndirilemedi."))} disabled={!data?.count}><Download /> Excel'e indir</Button>
      </div>

      <Toolbar>
        <RangePicker preset={preset} range={range} onPreset={choose} onFrom={setFrom} onTo={setTo} />
        <span className="flex rounded-xl bg-muted/60 p-0.5">
          {([["", "Hepsi"], ["chat", "WhatsApp sohbeti"], ["call", "Telefon görüşmesi"]] as const).map(([k, l]) => (
            <button key={k} type="button" onClick={() => setSource(k)} className={cn("rounded-lg px-3 py-1.5 text-xs font-medium transition-colors", source === k ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}>{l}</button>
          ))}
        </span>
        {channels.length > 1 && (
          <select value={channel} onChange={(e) => setChannel(Number(e.target.value))} className="h-9 rounded-xl border border-border/60 bg-card px-3 text-sm">
            <option value={0}>Tüm numaralar</option>
            {channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        )}
        <span className="relative min-w-52 flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Müşteri, numara ya da yorumda ara" className="h-9 w-full rounded-xl border border-border/60 bg-card pr-3 pl-9 text-sm outline-none focus:border-ring/50" />
        </span>
      </Toolbar>

      {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}

      <div className={cn("grid gap-3 lg:grid-cols-[1fr_1.2fr]", loading && "opacity-60 transition-opacity")}>
        <div className="grid gap-3 sm:grid-cols-2">
          <Figure icon={Star} tone="warning" label="Ortalama" value={data?.count ? data.average.toFixed(2) : "–"} sub={data?.count ? `${data.count} puan, 5 üzerinden` : "Bu aralıkta puan yok"} />
          <Figure icon={MessageSquareQuote} tone="primary" label="Yorum yazan" value={String(data?.withComment ?? 0)} sub={data?.count ? `Puanların %${Math.round(((data.withComment ?? 0) / data.count) * 100)}'i yorumlu` : "–"} onClick={() => setComment((v) => !v)} active={comment} />
          <Figure icon={ThumbsDown} tone="destructive" label="Düşük puan (1-2)" value={String(low)} sub={data?.count ? `Puanların %${Math.round((low / data.count) * 100)}'i` : "–"} onClick={() => setScore((v) => (v === "low" ? "" : "low"))} active={score === "low"} />
          <Figure icon={UsersRound} tone="violet" label="Puanlanan kişi" value={String(data?.agents.length ?? 0)} sub="Sohbeti kapatan ya da görüşmeyi yapan" />
        </div>
        <Card title="Puan dağılımı" icon={Star}>
          <div className="space-y-1.5">
            {[5, 4, 3, 2, 1].map((n) => {
              const v = data?.dist[n - 1] ?? 0;
              const on = score === String(n);
              return (
                <button key={n} type="button" onClick={() => setScore(on ? "" : String(n))} className={cn("flex w-full items-center gap-3 rounded-lg px-2 py-1 text-left transition-colors", on ? "bg-accent" : "hover:bg-accent/60")} data-tip={`Sadece ${n} puanları göster`}>
                  <span className="flex w-24 shrink-0 items-center gap-1 text-sm font-medium tabular-nums">{n} <Star className="size-3.5 fill-warning text-warning" /> <span className="text-xs font-normal text-muted-foreground">{SCORE_WORD[n]}</span></span>
                  <span className="h-2.5 flex-1 overflow-hidden rounded-full bg-muted">
                    <span className={cn("block h-full rounded-full transition-all", SCORE_BAR[n])} style={{ width: `${(v / peak) * 100}%` }} />
                  </span>
                  <span className="w-10 text-right text-sm tabular-nums text-muted-foreground">{v}</span>
                </button>
              );
            })}
          </div>
        </Card>
      </div>

      {data && data.questions.length > 0 && <Questions questions={data.questions} />}
      {data && data.agents.length > 0 && <People data={data} agent={agent} onAgent={setAgent} />}

      <div>
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2 px-1 text-xs text-muted-foreground">
            <span>{data ? `${data.total} puan` : "…"}</span>
            {filtered && <button type="button" onClick={clear} className="flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 font-medium hover:text-foreground"><X className="size-3" /> Filtreleri temizle</button>}
          </div>
          {!loading && items.length === 0 ? (
            <div className="rounded-2xl bg-card px-6 py-14 text-center ring-1 ring-border/60">
              <Star className="mx-auto mb-2 size-8 text-muted-foreground/40" />
              <p className="text-sm font-medium">{filtered ? "Bu filtreye uyan puan yok" : "Bu aralıkta puan yok"}</p>
              <p className="mt-1 text-xs text-muted-foreground">Anketler Ayarlar &gt; Cihaz ayarları &gt; Memnuniyet anketi ve Çağrı sonrası anket bölümünden açılır.</p>
            </div>
          ) : (
            <div className="space-y-2">
              {items.map((r, i) => <RatingRow key={`${r.source}-${r.at}-${i}`} r={r} onOpen={() => setOpen(r)} />)}
              {data && items.length < data.total && (
                <div className="flex justify-center pt-1">
                  <Button variant="secondary" onClick={() => void loadMore()} disabled={more}>{more ? "Yükleniyor..." : `Daha fazla göster (${data.total - items.length})`}</Button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
      {open && <RatingDetail r={open} onClose={() => setOpen(null)} />}
    </div>
  );
}

// writtenAnswers gives the written answers as question and answer. Newer
// scores keep them apart; older ones kept one text, which is split back
// where a line reads "question? answer" or "question: answer".
function writtenAnswers(r: WARating): { question: string; text: string }[] {
  if (r.texts?.length) return r.texts;
  const c = r.comment?.trim();
  if (!c) return [];
  const lines = c.split("\n").map((l) => l.trim()).filter(Boolean);
  return lines.map((l) => {
    // a single line is the customer's own words, whatever marks it has
    const m = lines.length > 1 ? (/^(.+\?)\s*:?\s+(.+)$/.exec(l) ?? /^([^:]{3,120}):\s+(.+)$/.exec(l)) : null;
    return m ? { question: m[1], text: m[2] } : { question: "", text: l };
  });
}

// Written shows written answers as a short form: the question small and
// grey, the answer under it.
function Written({ r, compact }: { r: WARating; compact?: boolean }) {
  const list = writtenAnswers(r);
  if (list.length === 0) return null;
  return (
    <div className={cn("space-y-2 rounded-xl bg-muted/50 px-3.5 py-2.5", compact && "space-y-1.5 py-2")}>
      {list.map((t, i) => (
        <div key={i}>
          {t.question && <p className="text-[0.7rem] leading-snug font-medium text-muted-foreground">{t.question}</p>}
          <p className={cn("text-sm leading-relaxed whitespace-pre-wrap", compact && "line-clamp-2")}>{t.text}</p>
        </div>
      ))}
    </div>
  );
}

// RatingDetail is one survey answer on its own: every question with its
// score, what the customer wrote, and who and where it was about.
function RatingDetail({ r, onClose }: { r: WARating; onClose: () => void }) {
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [onClose]);
  const Src = r.source === "call" ? PhoneCall : MessageCircle;
  const answers = r.answers.filter((a) => !(r.answers.length === 1 && a.question === SINGLE));
  return (
    <div className="fixed inset-0 z-[70] flex justify-end bg-black/30" onClick={onClose}>
      <aside onClick={(e) => e.stopPropagation()} className="animate-in slide-in-from-right-4 fade-in flex h-full w-full max-w-lg flex-col bg-background shadow-2xl duration-200">
        <header className="flex items-center gap-3 border-b border-border/60 px-5 py-4">
          <div className={cn("flex size-14 shrink-0 flex-col items-center justify-center rounded-2xl", SCORE_TONE[r.score])}>
            <span className="text-2xl leading-none font-bold tabular-nums">{r.score}</span>
            <span className="mt-1 flex">{[1, 2, 3, 4, 5].map((n) => <Star key={n} className={cn("size-2", n <= r.score ? "fill-current" : "opacity-30")} />)}</span>
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-base font-semibold">{r.customer || prettyPhone(r.phone)}</p>
            <p className="font-mono text-xs text-muted-foreground tabular-nums">{prettyPhone(r.phone)}</p>
            <p className="text-xs text-muted-foreground">{when(r.at)} · {SCORE_WORD[r.score]}</p>
          </div>
          <button type="button" onClick={onClose} aria-label="Kapat" className="flex size-9 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-5" /></button>
        </header>
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-5">
          {answers.length > 0 && (
            <section className="space-y-2">
              <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Puanlar</h3>
              <div className="space-y-2">
                {answers.map((a, i) => (
                  <div key={i} className="rounded-xl bg-card p-3 ring-1 ring-border/60">
                    <div className="flex items-start gap-3">
                      <p className="min-w-0 flex-1 text-sm leading-snug">{a.question}</p>
                      <span className={cn("shrink-0 rounded-lg px-2 py-0.5 text-sm font-bold tabular-nums", avgTone(a.score))}>{a.score}/5</span>
                    </div>
                    <div className="mt-2 flex gap-1">{[1, 2, 3, 4, 5].map((n) => <span key={n} className={cn("h-1.5 flex-1 rounded-full", n <= a.score ? SCORE_BAR[a.score] : "bg-muted")} />)}</div>
                  </div>
                ))}
              </div>
            </section>
          )}
          {writtenAnswers(r).length > 0 && (
            <section className="space-y-2">
              <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Müşterinin yazdıkları</h3>
              <Written r={r} />
            </section>
          )}
          <section className="space-y-2">
            <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Bilgiler</h3>
            <div className="divide-y divide-border/50 rounded-xl bg-card px-3.5 text-sm ring-1 ring-border/60">
              <Info label="Nereden">
                <span className="flex items-center gap-1.5"><Src className="size-3.5 text-muted-foreground" />{r.source === "call" ? "Telefon görüşmesinden sonra" : "WhatsApp sohbetinden sonra"}</span>
              </Info>
              {r.ticketNumber != null && <Info label="Sohbet">#{r.ticketNumber}</Info>}
              {r.talkSeconds ? <Info label="Görüşme süresi">{talk(r.talkSeconds).replace(" görüşme", "")}</Info> : null}
              {r.channel && <Info label="Numara">{r.channel}</Info>}
              {r.agent && (
                <Info label={r.source === "call" ? "Görüşmeyi yapan" : "Sohbeti kapatan"}>
                  <span className="flex items-center gap-2">
                    <UserAvatar userId={r.agent.id} name={r.agent.name} hasAvatar={r.agent.hasAvatar} version={r.agent.avatarVersion} className="size-6" fallbackClassName="bg-primary/10 text-[0.55rem] text-primary" />
                    {r.agent.name}
                  </span>
                </Info>
              )}
            </div>
          </section>
        </div>
        {r.conversationId && (
          <footer className="border-t border-border/60 px-5 py-3">
            <Link to={`/whatsapp/${r.conversationId}`} className="flex h-10 w-full items-center justify-center gap-2 rounded-xl bg-primary text-sm font-semibold text-primary-foreground hover:bg-primary/90"><MessageCircle className="size-4" /> Sohbeti aç</Link>
          </footer>
        )}
      </aside>
    </div>
  );
}

function Info({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 py-2.5">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-right font-medium">{children}</span>
    </div>
  );
}

function RatingRow({ r, onOpen }: { r: WARating; onOpen: () => void }) {
  const Src = r.source === "call" ? PhoneCall : MessageCircle;
  return (
    <div role="button" tabIndex={0} onClick={onOpen} onKeyDown={(e) => { if (e.key === "Enter") onOpen(); }} className="group flex cursor-pointer gap-3 rounded-2xl bg-card p-3.5 ring-1 ring-border/60 transition-colors hover:bg-accent/30 hover:ring-border">
      <div className={cn("flex w-14 shrink-0 flex-col items-center justify-center rounded-xl py-2", SCORE_TONE[r.score])}>
        <span className="text-2xl leading-none font-bold tabular-nums">{r.score}</span>
        <span className="mt-1 flex">{[1, 2, 3, 4, 5].map((n) => <Star key={n} className={cn("size-2", n <= r.score ? "fill-current" : "opacity-30")} />)}</span>
      </div>
      <div className="min-w-0 flex-1 space-y-1.5">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="truncate text-sm font-semibold">{r.customer || prettyPhone(r.phone)}</span>
          {r.customer && <span className="font-mono text-xs text-muted-foreground tabular-nums">{prettyPhone(r.phone)}</span>}
          <span className="ml-auto shrink-0 text-xs text-muted-foreground tabular-nums">{when(r.at)}</span>
        </div>
        <div className="flex flex-wrap items-center gap-1.5 text-[0.7rem]">
          <span className={cn("flex items-center gap-1 rounded-full px-2 py-0.5 font-medium", r.source === "call" ? "bg-sky-500/10 text-sky-700 dark:text-sky-400" : "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400")}>
            <Src className="size-3" /> {r.source === "call" ? "Telefon görüşmesi" : "WhatsApp sohbeti"}
          </span>
          {r.ticketNumber != null && <span className="rounded-full bg-muted px-2 py-0.5 text-muted-foreground">Sohbet #{r.ticketNumber}</span>}
          {r.talkSeconds ? <span className="rounded-full bg-muted px-2 py-0.5 text-muted-foreground">{talk(r.talkSeconds)}</span> : null}
          {r.channel && <span className="rounded-full bg-muted px-2 py-0.5 text-muted-foreground">{r.channel}</span>}
          {r.agent && (
            <span className="flex items-center gap-1 rounded-full bg-muted py-0.5 pr-2 pl-0.5 text-muted-foreground" data-tip={r.source === "call" ? "Görüşmeyi yapan" : "Sohbeti kapatan"}>
              <UserAvatar userId={r.agent.id} name={r.agent.name} hasAvatar={r.agent.hasAvatar} version={r.agent.avatarVersion} className="size-4" fallbackClassName="bg-primary/10 text-[0.4rem] text-primary" />
              {r.agent.name}
            </span>
          )}
        </div>
        {r.answers.length > 0 && !(r.answers.length === 1 && r.answers[0].question === SINGLE) && (
          <div className="flex flex-wrap gap-1.5">
            {r.answers.map((a, i) => (
              <span key={i} data-tip={a.question} className={cn("flex items-center gap-1.5 rounded-lg px-2 py-1 text-[0.72rem]", avgTone(a.score))}>
                <span className="max-w-48 truncate font-medium">{a.question}</span>
                <b className="tabular-nums">{a.score}</b>
              </span>
            ))}
          </div>
        )}
        <Written r={r} compact />
      </div>
      <ChevronRight className="size-5 shrink-0 self-center text-muted-foreground/50 transition-transform group-hover:translate-x-0.5 group-hover:text-muted-foreground" />
    </div>
  );
}

function Figure({ icon, tone, label, value, sub, onClick, active }: { icon: typeof Star; tone: ChipTone; label: string; value: string; sub: string; onClick?: () => void; active?: boolean }) {
  const body = (
    <>
      <IconChip icon={icon} tone={tone} size="lg" />
      <div className="min-w-0 text-left">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        <p className="text-2xl font-semibold tracking-tight tabular-nums">{value}</p>
        <p className="truncate text-[0.7rem] text-muted-foreground">{sub}</p>
      </div>
    </>
  );
  const cls = cn("flex items-start gap-3 rounded-2xl bg-card p-4 shadow-sm ring-1 transition-colors", active ? "ring-primary/40 bg-primary/5" : "ring-border/60");
  return onClick ? <button type="button" onClick={onClick} className={cn(cls, "hover:bg-accent/40")} data-tip={active ? "Filtreyi kaldır" : "Sadece bunları göster"}>{body}</button> : <div className={cls}>{body}</div>;
}

// Questions shows each survey question on its own: its average, how many
// answered and how the scores spread. The weakest one is marked.
function Questions({ questions }: { questions: WARatings["questions"] }) {
  const solid = questions.filter((q) => q.count >= MIN_FOR_WEAKEST);
  const weakest = solid.length > 1 ? solid.reduce((a, b) => (b.average < a.average ? b : a)) : null;
  return (
    <Card title="Sorulara göre" icon={ListChecks}>
      <p className="-mt-1 mb-3 text-xs text-muted-foreground">Anket formundaki her soru ayrı ayrı. Tek soruluk anketler (WhatsApp içindeki puan listesi, görüşme sonrası düğmeler) ve soru bazlı kayıttan önceki puanlar "{SINGLE}" altında toplanır. En zayıf alan, her soruda en az {MIN_FOR_WEAKEST} cevap birikince işaretlenir.</p>
      <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
        {questions.map((q) => {
          const total = Math.max(1, q.dist.reduce((a, b) => a + b, 0));
          const weak = weakest?.question === q.question;
          return (
            <div key={q.question} className={cn("rounded-2xl p-3.5 ring-1", weak ? "bg-destructive/5 ring-destructive/30" : "bg-muted/25 ring-border/50")}>
              <div className="flex items-start gap-2">
                <p className="min-w-0 flex-1 text-sm leading-snug font-medium">{q.question}</p>
                <span className={cn("flex shrink-0 items-center gap-1 rounded-lg px-2 py-0.5 text-lg font-bold tabular-nums", avgTone(q.average))}>{q.average.toFixed(2)}</span>
              </div>
              <div className="mt-2.5 flex h-2 overflow-hidden rounded-full bg-muted" data-tip={q.dist.map((v, i) => `${i + 1}: ${v}`).join(" · ")}>
                {[4, 3, 2, 1, 0].map((i) => q.dist[i] > 0 && <span key={i} className={SCORE_BAR[i + 1]} style={{ width: `${(q.dist[i] / total) * 100}%` }} />)}
              </div>
              <p className="mt-1.5 flex items-center gap-2 text-[0.7rem] text-muted-foreground">
                <span>{q.count} cevap</span>
                {weak && <span className="flex items-center gap-1 font-semibold text-destructive"><TrendingDown className="size-3" /> En çok gelişmesi gereken alan</span>}
              </p>
            </div>
          );
        })}
      </div>
    </Card>
  );
}

// People is each person by question: their average on every question,
// the weakest marked, so it is clear what each one should work on.
function People({ data, agent, onAgent }: { data: WARatings; agent: number; onAgent: (id: number) => void }) {
  const cols = data.questions.map((q) => q.question);
  const multi = cols.length > 1;
  return (
    <Card title="Kişilere göre" icon={UsersRound}>
      <p className="-mt-1 mb-3 text-xs text-muted-foreground">{multi ? `Her kişinin her sorudaki ortalaması. Bir soruda en az ${MIN_FOR_WEAKEST} cevabı olan kişinin en düşük puan aldığı soru çerçeveyle işaretlenir; geliştirmesi gereken alan odur. Bir kişiye tıklayınca aşağıda sadece onun puanları kalır.` : "Bir kişiye tıklayınca aşağıda sadece onun puanları kalır."}</p>
      <div className="-mx-4 overflow-x-auto px-4">
        <table className="w-full min-w-[36rem] border-separate border-spacing-y-1 text-sm">
          <thead>
            <tr className="text-left text-[0.7rem] text-muted-foreground">
              <th className="px-2 pb-1 font-medium">Kişi</th>
              <th className="px-2 pb-1 text-center font-medium">Genel</th>
              {multi && cols.map((c) => <th key={c} className="max-w-40 px-2 pb-1 text-center font-medium" title={c}><span className="line-clamp-2">{c}</span></th>)}
              <th className="px-2 pb-1 text-right font-medium">Puan</th>
            </tr>
          </thead>
          <tbody>
            {data.agents.map((a) => {
              const on = agent === a.agent.id;
              const byQ = new Map(a.questions.map((q) => [q.question, q]));
              const answered = a.questions.filter((q) => q.count >= MIN_FOR_WEAKEST && q.question !== SINGLE);
              const weakest = multi && answered.length > 1 ? answered.reduce((x, y) => (y.average < x.average ? y : x)).question : "";
              return (
                <tr key={a.agent.id} onClick={() => onAgent(on ? 0 : a.agent.id)} className={cn("cursor-pointer transition-colors [&>td]:py-1.5 [&>td:first-child]:rounded-l-xl [&>td:last-child]:rounded-r-xl", on ? "[&>td]:bg-primary/10" : "hover:[&>td]:bg-accent/50")}>
                  <td className="px-2">
                    <span className="flex items-center gap-2.5">
                      <UserAvatar userId={a.agent.id} name={a.agent.name} hasAvatar={a.agent.hasAvatar} version={a.agent.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-[0.65rem] text-primary" />
                      <span className="min-w-0">
                        <span className="block truncate font-medium">{a.agent.name}</span>
                        {weakest && <span className="block max-w-72 truncate text-[0.68rem] text-destructive" data-tip={`En düşük puan aldığı soru: ${weakest}`}>En düşük puanı: {weakest}</span>}
                      </span>
                    </span>
                  </td>
                  <td className="px-2 text-center"><span className={cn("inline-block min-w-12 rounded-lg px-2 py-1 font-bold tabular-nums", avgTone(a.average))}>{a.average.toFixed(1)}</span></td>
                  {multi && cols.map((c) => {
                    const q = byQ.get(c);
                    return (
                      <td key={c} className="px-2 text-center">
                        {q ? <span data-tip={`${q.count} cevap`} className={cn("inline-block min-w-12 rounded-lg px-2 py-1 font-semibold tabular-nums", avgTone(q.average), weakest === c && "ring-2 ring-destructive/50")}>{q.average.toFixed(1)}</span> : <span className="text-muted-foreground/50">–</span>}
                      </td>
                    );
                  })}
                  <td className="px-2 text-right text-xs text-muted-foreground tabular-nums">{a.count}{a.low > 0 && <span className="ml-1 text-destructive">({a.low} düşük)</span>}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </Card>
  );
}
