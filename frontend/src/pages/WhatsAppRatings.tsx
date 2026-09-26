// WhatsAppRatings ("Puanlamalar"): every score customers gave, at the end
// of a WhatsApp conversation or after a phone call, with who they rated,
// what they wrote and a way into the conversation. Totals, the spread of
// scores and each person's average sit on top; everything filters.

import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, Download, MessageCircle, MessageSquareQuote, PhoneCall, Search, Star, ThumbsDown, UsersRound, X } from "lucide-react";
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

      <div className="grid items-start gap-4 lg:grid-cols-[18rem_1fr]">
        <Card title="Kişiler" icon={UsersRound}>
          {!data || data.agents.length === 0 ? <p className="py-3 text-center text-sm text-muted-foreground">Puanlanan kimse yok.</p> : (
            <div className="-mx-1 space-y-0.5">
              {data.agents.map((a) => {
                const on = agent === a.agent.id;
                return (
                  <button key={a.agent.id} type="button" onClick={() => setAgent(on ? 0 : a.agent.id)} className={cn("flex w-full items-center gap-2.5 rounded-xl px-2 py-1.5 text-left transition-colors", on ? "bg-primary/10 ring-1 ring-primary/30" : "hover:bg-accent/60")}>
                    <UserAvatar userId={a.agent.id} name={a.agent.name} hasAvatar={a.agent.hasAvatar} version={a.agent.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-[0.65rem] text-primary" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">{a.agent.name}</span>
                      <span className="block text-[0.68rem] text-muted-foreground">{a.count} puan{a.low > 0 ? ` · ${a.low} düşük` : ""}</span>
                    </span>
                    <span className={cn("flex items-center gap-1 text-sm font-semibold tabular-nums", a.average < 3 ? "text-destructive" : a.average < 4 ? "text-warning" : "text-foreground")}><Star className="size-3.5 fill-warning text-warning" />{a.average.toFixed(1)}</span>
                  </button>
                );
              })}
            </div>
          )}
        </Card>

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
              {items.map((r, i) => <RatingRow key={`${r.source}-${r.at}-${i}`} r={r} />)}
              {data && items.length < data.total && (
                <div className="flex justify-center pt-1">
                  <Button variant="secondary" onClick={() => void loadMore()} disabled={more}>{more ? "Yükleniyor..." : `Daha fazla göster (${data.total - items.length})`}</Button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function RatingRow({ r }: { r: WARating }) {
  const Src = r.source === "call" ? PhoneCall : MessageCircle;
  return (
    <div className="flex gap-3 rounded-2xl bg-card p-3.5 ring-1 ring-border/60">
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
        {r.comment && <p className="rounded-xl bg-muted/50 px-3 py-2 text-sm leading-relaxed whitespace-pre-wrap">“{r.comment}”</p>}
      </div>
      {r.conversationId && (
        <Link to={`/whatsapp/${r.conversationId}`} data-tip="Sohbeti aç" className="flex size-9 shrink-0 items-center justify-center self-center rounded-xl text-muted-foreground hover:bg-accent hover:text-foreground"><MessageCircle className="size-4" /></Link>
      )}
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
