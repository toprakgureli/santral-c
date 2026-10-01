import { useEffect, useState } from "react";
import {
  ArrowLeftRight,
  Copy,
  Delete,
  Grid3x3,
  Headset,
  Mic,
  MicOff,
  Pause,
  Phone,
  PhoneOff,
  Play,
  Settings2,
} from "lucide-react";
import VoiceMixer from "../layout/VoiceMixer";
import { useSoftphoneContext } from "../../softphone/SoftphoneContext";
import { useShift } from "../../shift/ShiftContext";
import WhatsAppTemplateDialog from "../WhatsAppTemplateDialog";
import WhatsAppIcon from "../icons/WhatsAppIcon";
import { whatsappNumber, type WhatsAppKind } from "../../lib/whatsapp";
import { useWhatsAppWrite } from "../../whatsapp/useWhatsAppWrite";
import { displayNumber, normalizeDial } from "../../softphone/dial";
import { tones } from "../../softphone/tones";
import { Button, Card } from "../ui";
import { cn } from "../../lib/utils";
import { formatDuration } from "../../pages/callFormat";
import { clockTime } from "../../lib/time";

const keypadKeys = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "*", "0", "#"];

function Round({ onClick, tone = "muted", title, disabled, size = "md", children }: { onClick?: () => void; tone?: "muted" | "on" | "call" | "hang"; title?: string; disabled?: boolean; size?: "md" | "lg"; children: React.ReactNode }) {
  const toneClass: Record<string, string> = {
    muted: "bg-muted/60 text-foreground ring-1 ring-border/40 hover:bg-accent",
    on: "bg-primary text-primary-foreground",
    call: "bg-success text-white hover:opacity-90",
    hang: "bg-destructive text-white hover:opacity-90",
  };
  const sizeClass = size === "lg" ? "size-16 [&_svg]:size-6" : "size-12 [&_svg]:size-5";
  return (
    <button onClick={onClick} disabled={disabled} data-tip={title} className={cn("flex items-center justify-center rounded-full shadow-sm transition active:scale-95 disabled:opacity-40", sizeClass, toneClass[tone])}>
      {children}
    </button>
  );
}

export function Softphone({ hasExtension, canCall }: { hasExtension: boolean; canCall: boolean }) {
  const phone = useSoftphoneContext();
  const shift = useShift();
  const [waOpen, setWaOpen] = useState<WhatsAppKind | null>(null);
  // Idle: the follow-up targets the last real number (never an extension).
  // In a call: the "we are on the phone" text goes to the customer on the line.
  const waNumber = whatsappNumber(phone.lastPeer ?? "");
  const waLiveNumber = whatsappNumber(phone.peer ?? "");
  // The company number with a template, or the agent's own WhatsApp; the
  // whatsapp.write_business permission decides.
  const { business: viaBusiness, write } = useWhatsAppWrite();
  function openWhatsApp(kind: WhatsAppKind) {
    const raw = kind === "live" ? phone.peer : phone.lastPeer;
    const num = kind === "live" ? waLiveNumber : waNumber;
    if (!num) return;
    write(raw ?? num, kind);
  }
  const [target, setTarget] = useState("");
  const [showKeypad, setShowKeypad] = useState(false);
  const [nowTick, setNowTick] = useState<number>(() => Date.now());

  const idle = phone.status === "registered" || phone.status === "error" || phone.status === "connecting";
  const outgoing = phone.status === "calling" || phone.status === "ringing";
  const active = phone.status === "in-call" || phone.status === "held";
  // Answer time lives in the global softphone, so the duration survives menu
  // switches instead of restarting from zero.
  const dur = phone.answeredAt ? Math.max(0, Math.floor((nowTick - phone.answeredAt) / 1000)) : 0;

  useEffect(() => {
    const t = window.setInterval(() => setNowTick(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);

  function callNow() {
    if (!canCall) return;
    const n = normalizeDial(target);
    if (n) phone.call(n).catch(() => undefined);
  }

  // Clicking the number on the call face copies it bare (5551234567).
  const [peerCopied, setPeerCopied] = useState(false);
  function copyPeer() {
    const val = displayNumber(phone.peer || "");
    if (!val) return;
    navigator.clipboard?.writeText(val).catch(() => undefined);
    setPeerCopied(true);
    window.setTimeout(() => setPeerCopied(false), 1200);
  }

  return (
    <Card title="Softphone" icon={Headset}>
      <WhatsAppTemplateDialog open={waOpen !== null} initialKind={waOpen ?? "unreached"} onClose={() => setWaOpen(null)} previewNumber={displayNumber((waOpen === "live" ? phone.peer : phone.lastPeer) ?? "")} />
      {!hasExtension ? (
        <p className="text-sm text-muted-foreground">Hesabınıza bir dahili numara atanmamış. Yöneticinizle görüşün.</p>
      ) : phone.secondary ? (
        /* Another tab (maybe a forgotten one) holds the softphone; offer to pull it here. */
        <div className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
            <Phone className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium">Softphone başka bir sekmede açık</p>
            <p className="mx-auto max-w-xs text-xs leading-relaxed text-muted-foreground">
              Çağrılar o sekmede yönetiliyor. Açık kalmış eski bir sekme olabilir, buradan devam edersen softphone bu sekmeye geçer, diğer sekme devre dışı kalır.
            </p>
          </div>
          <Button onClick={phone.takeOver} className="mt-1">Bu tarayıcıdan devam et</Button>
        </div>
      ) : !shift.active && (idle || phone.status === "disabled") ? (
        /* Off shift the softphone is not registered at all: nothing rings, nothing dials. */
        <div className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
            <Phone className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium">Mesai başlatılmadı</p>
            <p className="mx-auto max-w-xs text-xs leading-relaxed text-muted-foreground">
              Mesai başlamadan çağrı gelmez ve arama yapılamaz.
              {shift.status?.reminderAt && shift.status.autoEndAt && (
                <> Mesai bitiş saati {clockTime(shift.status.reminderAt)}. Bitirilmezse sistem saat {clockTime(shift.status.autoEndAt)} olunca kapatır.</>
              )}
            </p>
          </div>
          {shift.error && <p className="text-xs text-destructive">{shift.error}</p>}
          <Button onClick={() => void shift.start()} disabled={shift.busy || shift.loading} className="mt-1">
            Mesai Başlat
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          {phone.error && <p className="text-sm text-destructive">{phone.error}</p>}

          {/* Idle: number entry + dialpad (only for agents allowed to place calls) */}
          {idle && !canCall && (
            <div className="py-8 text-center">
              <p className="text-sm text-muted-foreground">Giden çağrı yetkiniz yok.</p>
              <p className="mt-1 text-xs text-muted-foreground/70">Gelen çağrıları cevaplayabilirsiniz.</p>
            </div>
          )}
          {idle && canCall && (
            <>
              <div className="space-y-1">
                <input
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && callNow()}
                  placeholder="Numara veya dahili"
                  inputMode="tel"
                  className="w-full bg-transparent text-center text-3xl font-semibold tracking-wide text-foreground outline-none placeholder:text-muted-foreground/40"
                />
                <p className="h-4 text-center text-xs text-muted-foreground">{phone.endReason ? `Son çağrı: ${phone.endReason}` : ""}</p>
              </div>

              {/* WhatsApp follow-up for the last number: one click opens the chat with the agent's own message. */}
              {waNumber && (
                <div className="flex items-stretch gap-2">
                  <button
                    type="button"
                    onClick={() => openWhatsApp("unreached")}
                    data-tip={`${displayNumber(phone.lastPeer ?? "")} numarasına WhatsApp'tan yaz`}
                    className="flex min-w-0 flex-1 items-center gap-3 rounded-2xl bg-[#25D366]/10 px-3.5 py-2.5 text-left ring-1 ring-[#25D366]/30 transition-colors hover:bg-[#25D366]/18 hover:ring-[#25D366]/50"
                  >
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-[#25D366] text-black">
                      <WhatsAppIcon className="size-5" />
                    </span>
                    <span className="min-w-0">
                      <span className="block text-sm font-semibold leading-tight">WhatsApp'tan yaz</span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {viaBusiness ? "Şirket numarasından" : "Son çağrı"} · <span className="font-mono tabular-nums">{displayNumber(phone.lastPeer ?? "")}</span>
                      </span>
                    </span>
                  </button>
                  {!viaBusiness && <button
                    type="button"
                    onClick={() => setWaOpen("unreached")}
                    aria-label="WhatsApp mesajını düzenle"
                    data-tip="Mesajı düzenle"
                    className="flex w-12 shrink-0 items-center justify-center rounded-2xl bg-muted/60 text-muted-foreground ring-1 ring-border/40 transition-colors hover:bg-accent hover:text-foreground"
                  >
                    <Settings2 className="size-4" />
                  </button>}
                </div>
              )}
              <div className="mx-auto grid max-w-[15rem] grid-cols-3 gap-2">
                {keypadKeys.map((k) => (
                  <button
                    key={k}
                    onClick={() => { setTarget((t) => t + k); tones.dtmf(k); }}
                    className="h-12 rounded-2xl bg-muted/60 text-lg font-semibold text-foreground shadow-sm ring-1 ring-border/40 transition hover:bg-accent active:scale-95"
                  >
                    {k}
                  </button>
                ))}
              </div>
              <div className="mx-auto flex max-w-[15rem] items-center justify-between">
                <span className="size-12" />
                <Round tone="call" size="lg" data-tip="Ara" onClick={callNow} disabled={phone.status !== "registered" || !target}>
                  <Phone />
                </Round>
                <Round data-tip="Sil" onClick={() => setTarget((t) => t.slice(0, -1))} disabled={!target}>
                  <Delete />
                </Round>
              </div>
            </>
          )}

          {/* Ringing / incoming / in-call: a centred call face */}
          {!idle && (
            <div className="flex flex-col items-center gap-5 py-2">
              <div className="text-center">
                <button
                  type="button"
                  onClick={copyPeer}
                  data-tip="Numarayı kopyala"
                  className="inline-flex items-center gap-2 rounded-lg px-2 py-0.5 text-2xl font-semibold tracking-wide transition-colors hover:bg-accent"
                >
                  {displayNumber(phone.peer || "") || phone.peer || "—"}
                  {peerCopied ? <span className="text-xs font-medium text-success">Kopyalandı</span> : <Copy className="size-4 text-muted-foreground" />}
                </button>
                <div className="mt-1 text-sm text-muted-foreground">
                  {phone.status === "incoming" ? "Gelen çağrı" : outgoing ? "Aranıyor..." : phone.held ? "Beklemede" : "Görüşme"}
                </div>
              </div>

              {active && (
                <div className="font-mono text-5xl font-semibold tabular-nums tracking-tight">{formatDuration(dur)}</div>
              )}

              {active && <VoiceMixer size="large" className="w-full max-w-sm" />}

              {/* Mid-call: open WhatsApp with the customer on the line, "we are talking right now" text. */}
              {active && waLiveNumber && (
                <div className="flex w-full max-w-[18rem] items-stretch gap-2">
                  <button
                    type="button"
                    onClick={() => openWhatsApp("live")}
                    data-tip="Görüşmekte olduğun müşteriye WhatsApp'tan yaz"
                    className="flex min-w-0 flex-1 items-center gap-3 rounded-2xl bg-[#25D366]/10 px-3.5 py-2 text-left ring-1 ring-[#25D366]/30 transition-colors hover:bg-[#25D366]/18 hover:ring-[#25D366]/50"
                  >
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-[#25D366] text-black">
                      <WhatsAppIcon className="size-4" />
                    </span>
                    <span className="min-w-0">
                      <span className="block text-sm font-semibold leading-tight">WhatsApp'tan yaz</span>
                      <span className="block truncate text-xs text-muted-foreground">{viaBusiness ? "Şirket numarasından, şablonla" : "Görüşme sırasında mesajı"}</span>
                    </span>
                  </button>
                  {!viaBusiness && <button
                    type="button"
                    onClick={() => setWaOpen("live")}
                    aria-label="Görüşme sırasında mesajını düzenle"
                    data-tip="Mesajı düzenle"
                    className="flex w-11 shrink-0 items-center justify-center rounded-2xl bg-muted/60 text-muted-foreground ring-1 ring-border/40 transition-colors hover:bg-accent hover:text-foreground"
                  >
                    <Settings2 className="size-4" />
                  </button>}
                </div>
              )}

              {phone.status === "incoming" && (
                <div className="flex items-center justify-center gap-12 pt-1">
                  <Round tone="call" size="lg" data-tip="Cevapla" onClick={() => phone.answer().catch(() => undefined)}><Phone /></Round>
                  <Round tone="hang" size="lg" data-tip="Reddet" onClick={() => phone.hangup().catch(() => undefined)}><PhoneOff /></Round>
                </div>
              )}

              {active && (
                <div className="flex flex-col items-center gap-3">
                  <div className="flex items-center justify-center gap-3">
                    <Round tone={phone.muted ? "on" : "muted"} data-tip="Sustur" onClick={phone.toggleMute}>{phone.muted ? <MicOff /> : <Mic />}</Round>
                    <Round tone={phone.held ? "on" : "muted"} data-tip="Beklet" onClick={() => phone.toggleHold().catch(() => undefined)}>{phone.held ? <Play /> : <Pause />}</Round>
                    <Round tone={showKeypad ? "on" : "muted"} data-tip="Tuşlar" onClick={() => setShowKeypad((v) => !v)}><Grid3x3 /></Round>
                    <Round tone="hang" size="lg" data-tip="Kapat" onClick={() => phone.hangup().catch(() => undefined)}><PhoneOff /></Round>
                  </div>
                  {showKeypad && (
                    <div className="grid w-full max-w-[15rem] grid-cols-3 gap-2">
                      {keypadKeys.map((k) => (
                        <button key={k} onClick={() => { tones.dtmf(k); phone.sendDtmf(k); }} className="h-12 rounded-2xl bg-muted/60 text-lg font-semibold text-foreground shadow-sm ring-1 ring-border/40 transition hover:bg-accent active:scale-95">
                          {k}
                        </button>
                      ))}
                    </div>
                  )}
                  <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <ArrowLeftRight className="size-3.5" /> Aktarmak için sağdaki listeye sağ tıkla
                  </p>
                </div>
              )}

              {outgoing && (
                <Round tone="hang" size="lg" data-tip="Kapat" onClick={() => phone.hangup().catch(() => undefined)}><PhoneOff /></Round>
              )}
            </div>
          )}
        </div>
      )}
    </Card>
  );
}
