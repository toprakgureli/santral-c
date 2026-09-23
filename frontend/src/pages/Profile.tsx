// Profil: one person's page, laid out like Devtrack's. /profile is the
// caller's own (editable), /profile/:id a teammate's. The card holds the
// photo, name, headline, roles and biography; below it the call-centre
// record for today and this month.

import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, Camera, Clock, Pencil, Trash2, UserRound } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { Profile as ProfileData } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import AvatarCropper from "../components/profile/AvatarCropper";
import { Badge, Button, CharCount, EmptyState, Input, Skeleton } from "../components/ui";
import UserAvatar from "../components/ui/UserAvatar";
import { AVATAR_TYPES, loadAvatarFile, type AvatarSource } from "../lib/avatar";
import { cn } from "../lib/utils";

const HEADLINE_MAX = 120;
const BIO_MAX = 2000;

function joined(iso: string) {
  const [y, m, d] = iso.split("-");
  return `${d}.${m}.${y}`;
}

function clock(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function hours(seconds: number) {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h === 0) return `${m} dk`;
  return `${h} sa ${String(m).padStart(2, "0")} dk`;
}

export function Profile() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { setUser, user } = useAuth();
  const [profile, setProfile] = useState<ProfileData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    (id ? api.profileOf(Number(id)) : api.myProfile())
      .then((p) => {
        setProfile(p);
        setError(null);
      })
      .catch((e) => setError(e instanceof ApiError ? e.message : "Profil yüklenemedi."))
      .finally(() => setLoading(false));
  }, [id]);

  useEffect(load, [load]);

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => (window.history.length > 1 ? navigate(-1) : navigate("/"))}
          aria-label="Geri"
          className="flex size-9 shrink-0 items-center justify-center rounded-xl border border-border/70 bg-card text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
        </button>
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold tracking-tight">{id ? (profile?.name ?? "Profil") : "Profilim"}</h1>
          <p className="text-sm text-muted-foreground">{id ? "Ekip arkadaşının profili ve çağrı karnesi" : "Tanıtımını, fotoğrafını ve karneni gör"}</p>
        </div>
      </div>

      {loading ? (
        <div className="space-y-3">
          <Skeleton className="h-40 w-full rounded-2xl" />
          <Skeleton className="h-24 w-full rounded-2xl" />
        </div>
      ) : error || !profile ? (
        <EmptyState icon={<UserRound />} title="Profil bulunamadı" description={error ?? undefined} />
      ) : (
        <ProfileView
          profile={profile}
          onSaved={(saved) => {
            setProfile(saved);
            // The topbar photo and name come from the auth user; keep them in step.
            if (saved.editable && user) setUser({ ...user, name: saved.name, hasAvatar: saved.hasAvatar, avatarVersion: saved.avatarVersion });
          }}
        />
      )}
    </div>
  );
}

function Stat({ label, value, hint, tone }: { label: string; value: string; hint?: string; tone?: string }) {
  return (
    <div className="rounded-xl border border-border/60 bg-muted/25 p-3.5">
      <p className="text-[0.6875rem] text-muted-foreground">{label}</p>
      <p className={cn("mt-0.5 text-lg font-semibold tabular-nums", tone)}>{value}</p>
      {hint && <p className="mt-0.5 text-[0.6875rem] text-muted-foreground/70">{hint}</p>}
    </div>
  );
}

function ProfileView({ profile, onSaved }: { profile: ProfileData; onSaved: (p: ProfileData) => void }) {
  const picker = useRef<HTMLInputElement>(null);
  const [editing, setEditing] = useState(false);
  const [headline, setHeadline] = useState(profile.headline ?? "");
  const [bio, setBio] = useState(profile.bio ?? "");
  // null: photo unchanged, "": removed, data URI: new photo
  const [avatar, setAvatar] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [cropping, setCropping] = useState<AvatarSource | null>(null);
  const [expanded, setExpanded] = useState(false);

  const stats = profile.stats;
  const showsPhoto = avatar === null ? profile.hasAvatar : avatar !== "";

  function start() {
    setHeadline(profile.headline ?? "");
    setBio(profile.bio ?? "");
    setAvatar(null);
    setError(null);
    setEditing(true);
  }

  async function pick(file: File | null) {
    if (!file) return;
    setError(null);
    try {
      setCropping(await loadAvatarFile(file));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Görsel işlenemedi.");
    }
  }

  async function save() {
    setBusy(true);
    setError(null);
    try {
      if (avatar !== null) await api.setMyAvatar(avatar);
      const saved = await api.updateMyProfile({ headline: headline.trim(), bio: bio.trim() });
      onSaved(saved);
      setEditing(false);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Profil kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  }

  const bioLong = (profile.bio ?? "").length > 400 || (profile.bio ?? "").split("\n").length > 5;

  return (
    <div className="space-y-6">
      {cropping && (
        <AvatarCropper
          image={cropping}
          onCancel={() => setCropping(null)}
          onApply={(dataUrl) => {
            setAvatar(dataUrl);
            setCropping(null);
          }}
        />
      )}

      {error && <p className="rounded-xl bg-destructive/10 px-4 py-2.5 text-sm text-destructive">{error}</p>}

      <div className="overflow-hidden rounded-2xl border border-border/60 bg-card">
      <div className="h-24 bg-gradient-to-r from-primary/70 via-violet-500/60 to-primary/40" />
      <div className="-mt-12 flex flex-col gap-5 p-5 sm:flex-row sm:items-start">
        <div className="relative shrink-0 self-center rounded-2xl ring-4 ring-card sm:self-start">
          <UserAvatar
            userId={profile.id}
            name={profile.name}
            hasAvatar={showsPhoto && avatar === null}
            version={profile.avatarVersion}
            src={avatar || undefined}
            className="size-28 rounded-2xl [&>*]:rounded-2xl"
            fallbackClassName="bg-muted text-3xl text-muted-foreground"
          />
          {editing && (
            <>
              <button
                type="button"
                onClick={() => picker.current?.click()}
                aria-label="Fotoğraf seç"
                className="absolute -right-2 -bottom-2 flex size-8 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-sm"
              >
                <Camera className="size-4" />
              </button>
              <input
                ref={picker}
                type="file"
                accept={AVATAR_TYPES.join(",")}
                className="hidden"
                aria-label="Profil fotoğrafı"
                onChange={(e) => {
                  void pick(e.target.files?.[0] ?? null);
                  e.target.value = "";
                }}
              />
            </>
          )}
        </div>

        <div className="min-w-0 flex-1 space-y-3 sm:pt-12">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <h2 className="text-xl font-semibold tracking-tight">{profile.name}</h2>
              {!editing && profile.headline && <p className="text-sm text-muted-foreground">{profile.headline}</p>}
              {!editing && !profile.headline && <p className="text-sm text-muted-foreground">{profile.email}</p>}
            </div>
            {profile.editable && !editing && (
              <Button variant="secondary" onClick={start} className="h-9">
                <Pencil className="size-4" /> Düzenle
              </Button>
            )}
          </div>

          {editing ? (
            <div className="space-y-3">
              <div className="space-y-1.5">
                <label htmlFor="profile-headline" className="block text-xs text-muted-foreground">Kısa tanıtım</label>
                <Input
                  id="profile-headline"
                  value={headline}
                  maxLength={HEADLINE_MAX}
                  disabled={busy}
                  placeholder="Örn: Teknik destek · Faturalama ve entegrasyon"
                  onChange={(e) => setHeadline(e.target.value)}
                />
                <CharCount value={headline} max={HEADLINE_MAX} className="block text-right" />
              </div>
              <div className="space-y-1.5">
                <label htmlFor="profile-bio" className="block text-xs text-muted-foreground">Biyografi</label>
                <textarea
                  id="profile-bio"
                  rows={5}
                  value={bio}
                  maxLength={BIO_MAX}
                  disabled={busy}
                  placeholder="Neler yapıyorsun, hangi konularda yardım edebilirsin?"
                  onChange={(e) => setBio(e.target.value)}
                  className="w-full resize-none rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2.5 text-sm outline-none transition focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20 disabled:opacity-50"
                />
                <CharCount value={bio} max={BIO_MAX} className="block text-right" />
              </div>
              <div className="flex flex-wrap justify-end gap-2">
                {showsPhoto && (
                  <Button variant="ghost" disabled={busy} className="mr-auto h-9 px-3 text-destructive" onClick={() => setAvatar("")}>
                    <Trash2 className="size-4" /> Fotoğrafı kaldır
                  </Button>
                )}
                <Button variant="ghost" disabled={busy} className="h-9" onClick={() => setEditing(false)}>Vazgeç</Button>
                <Button disabled={busy} className="h-9" onClick={save}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
              </div>
            </div>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-1.5">
                {profile.roles.map((role) => (
                  <Badge key={role} tone="slate">{role}</Badge>
                ))}
                {profile.extension && <Badge tone="blue">Dahili {profile.extension}</Badge>}
                {!profile.active && <Badge tone="red">Pasif</Badge>}
                <span className="text-xs text-muted-foreground/70">{joined(profile.joinedAt)} tarihinde katıldı</span>
              </div>
              {profile.bio ? (
                <div className="min-w-0">
                  <p className={cn("whitespace-pre-wrap break-words text-sm leading-relaxed", !expanded && bioLong && "line-clamp-5")}>{profile.bio}</p>
                  {bioLong && (
                    <button type="button" onClick={() => setExpanded((v) => !v)} className="mt-0.5 text-[0.6875rem] font-medium text-muted-foreground underline underline-offset-2 hover:text-foreground">
                      {expanded ? "Daha az göster" : "Devamını gör"}
                    </button>
                  )}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground/70">Henüz bir tanıtım yazılmamış.</p>
              )}
            </>
          )}
        </div>
      </div>
      </div>

      <div className="space-y-2">
        <p className="text-xs font-medium text-muted-foreground">Çağrı karnesi</p>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <Stat label="Bugün gerçek çağrı" value={String(stats.todayReal)} hint="30 sn ve üstü görüşmeler" tone={stats.todayReal > 0 ? "text-success" : undefined} />
          <Stat label="Bugün cevapsız" value={String(stats.todayUnanswered)} hint="Bağlanmayan çağrılar" tone={stats.todayUnanswered > 0 ? "text-warning" : undefined} />
          <Stat label="Bu ay gerçek çağrı" value={String(stats.monthReal)} />
          <Stat label="Bu ay görüşme süresi" value={hours(stats.monthTalkSeconds)} hint="Cevaplanan çağrıların toplamı" />
        </div>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <Stat label="Ortalama çağrı süresi" value={stats.weekAvgTalkSeconds > 0 ? clock(stats.weekAvgTalkSeconds) : "—"} hint={`Son 7 gün · ${stats.weekReal} gerçek çağrı`} tone={stats.weekAvgTalkSeconds > 0 ? "text-primary" : undefined} />
          <Stat label="Son 7 gün gerçek çağrı" value={String(stats.weekReal)} />
          <Stat label="Bu ay eskalasyon" value={String(stats.monthEscalations)} hint="Kaydettiği eskalasyon sayısı" />
          <Stat label="Toplam eskalasyon" value={String(stats.totalEscalations)} />
        </div>
      </div>

      {stats.monthReal === 0 && stats.todayUnanswered === 0 && stats.totalEscalations === 0 && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground/70">
          <Clock className="size-4" />
          Bu kişinin henüz kayıtlı çağrısı yok.
        </p>
      )}
    </div>
  );
}
