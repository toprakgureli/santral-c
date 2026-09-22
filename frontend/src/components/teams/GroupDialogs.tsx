// The group dialogs: create a group, edit its settings (name, description,
// photo, posting policy), add or invite people, start a direct message.

import { useEffect, useRef, useState } from "react";
import { Camera, Megaphone, Trash2, Users } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { api, ApiError } from "@/api/client";
import type { TeamsGroupDetail } from "@/api/types";
import AvatarCropper from "@/components/profile/AvatarCropper";
import GroupAvatar from "@/components/teams/GroupAvatar";
import PeoplePicker from "@/components/teams/PeoplePicker";
import { Button, CharCount, Input, Modal } from "@/components/ui";
import { AVATAR_TYPES, loadAvatarFile, type AvatarSource } from "@/lib/avatar";
import { cn } from "@/lib/utils";

const NAME_MAX = 120;
const DESC_MAX = 500;

function PolicyPicker({ value, onChange }: { value: "everyone" | "admins"; onChange: (v: "everyone" | "admins") => void }) {
  const opts: { key: "everyone" | "admins"; label: string; hint: string; icon: typeof Users }[] = [
    { key: "everyone", label: "Herkes yazabilir", hint: "Yazma hakkı üye bazında kapatılabilir", icon: Users },
    { key: "admins", label: "Duyuru grubu", hint: "Sadece sahip ve yöneticiler yazar", icon: Megaphone },
  ];
  return (
    <div className="grid grid-cols-2 gap-2">
      {opts.map((o) => (
        <button
          key={o.key}
          type="button"
          onClick={() => onChange(o.key)}
          className={cn("flex items-start gap-2.5 rounded-xl border px-3 py-2.5 text-left transition-colors", value === o.key ? "border-primary bg-primary/10" : "border-border/70 hover:bg-accent")}
        >
          <o.icon className={cn("mt-0.5 size-4 shrink-0", value === o.key ? "text-primary" : "text-muted-foreground")} />
          <span className="min-w-0">
            <span className="block text-sm font-medium leading-tight">{o.label}</span>
            <span className="block text-xs text-muted-foreground">{o.hint}</span>
          </span>
        </button>
      ))}
    </div>
  );
}

export function NewGroupDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (g: TeamsGroupDetail) => void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [policy, setPolicy] = useState<"everyone" | "admins">("everyone");
  const [members, setMembers] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName("");
      setDescription("");
      setPolicy("everyone");
      setMembers([]);
      setError(null);
    }
  }, [open]);

  async function create() {
    setBusy(true);
    setError(null);
    try {
      const g = await api.teamsCreateGroup({ name: name.trim(), description: description.trim(), postPolicy: policy, memberIds: members });
      onCreated(g);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Grup oluşturulamadı.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Yeni grup"
      description="Bir ad ver, kimlerin yazabileceğini seç, üyeleri ekle."
      footer={
        <>
          <Button variant="secondary" onClick={onClose} className="h-9" disabled={busy}>Vazgeç</Button>
          <Button onClick={create} className="h-9" disabled={busy || name.trim().length < 2}>{busy ? "Oluşturuluyor..." : "Grubu oluştur"}</Button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="space-y-1.5">
          <label className="block text-xs text-muted-foreground">Grup adı</label>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={NAME_MAX} placeholder="Örn: Teknik Ekip" autoFocus />
        </div>
        <div className="space-y-1.5">
          <label className="block text-xs text-muted-foreground">Açıklama</label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            maxLength={DESC_MAX}
            rows={2}
            placeholder="Bu grup ne için?"
            className="w-full resize-none rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2.5 text-sm outline-none transition focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
          />
          <CharCount value={description} max={DESC_MAX} className="text-right" />
        </div>
        <div className="space-y-1.5">
          <label className="block text-xs text-muted-foreground">Kimler yazabilir</label>
          <PolicyPicker value={policy} onChange={setPolicy} />
        </div>
        <div className="space-y-1.5">
          <label className="block text-xs text-muted-foreground">Üyeler {members.length > 0 && <span className="text-foreground">· {members.length} kişi</span>}</label>
          <PeoplePicker selected={members} onChange={setMembers} height={200} />
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
      </div>
    </Modal>
  );
}

export function GroupSettingsDialog({ group, open, onClose, onSaved }: { group: TeamsGroupDetail; open: boolean; onClose: () => void; onSaved: (g: TeamsGroupDetail) => void }) {
  const navigate = useNavigate();
  const picker = useRef<HTMLInputElement>(null);
  const [name, setName] = useState(group.name);
  const [description, setDescription] = useState(group.description);
  const [policy, setPolicy] = useState<"everyone" | "admins">(group.postPolicy);
  const [avatar, setAvatar] = useState<string | null>(null);
  const [cropping, setCropping] = useState<AvatarSource | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName(group.name);
      setDescription(group.description);
      setPolicy(group.postPolicy);
      setAvatar(null);
      setConfirmDelete(false);
      setError(null);
    }
  }, [open, group]);

  async function pick(file: File | null) {
    if (!file) return;
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
      let g = await api.teamsUpdateGroup(group.id, { name: name.trim(), description: description.trim(), postPolicy: policy });
      if (avatar !== null) g = await api.teamsSetGroupAvatar(group.id, avatar);
      onSaved(g);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      await api.teamsDeleteGroup(group.id);
      onClose();
      navigate("/teams");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Silinemedi.");
      setBusy(false);
    }
  }

  const showsPhoto = avatar === null ? group.hasAvatar : avatar !== "";

  return (
    <>
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
      <Modal
        open={open && !cropping}
        onClose={onClose}
        title="Grup ayarları"
        description="Ad, açıklama, fotoğraf ve kimlerin yazabileceği."
        footer={
          <>
            {group.canDelete && (
              confirmDelete ? (
                <span className="mr-auto flex items-center gap-2 text-xs">
                  <span className="text-muted-foreground">Grup ve tüm mesajlar silinecek.</span>
                  <button type="button" onClick={remove} disabled={busy} className="font-medium text-destructive hover:underline">Evet, sil</button>
                  <button type="button" onClick={() => setConfirmDelete(false)} className="text-muted-foreground hover:underline">Vazgeç</button>
                </span>
              ) : (
                <Button variant="ghost" onClick={() => setConfirmDelete(true)} disabled={busy} className="mr-auto h-9 px-3 text-xs text-destructive">
                  <Trash2 className="size-3.5" /> Grubu sil
                </Button>
              )
            )}
            <Button variant="secondary" onClick={onClose} className="h-9" disabled={busy}>Vazgeç</Button>
            <Button onClick={save} className="h-9" disabled={busy || name.trim().length < 2}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="flex items-center gap-4">
            <div className="relative">
              <GroupAvatar group={{ ...group, hasAvatar: showsPhoto && avatar === null }} src={avatar || undefined} className="size-20 text-2xl" />
              <button
                type="button"
                onClick={() => picker.current?.click()}
                aria-label="Fotoğraf seç"
                className="absolute -right-2 -bottom-2 flex size-8 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-sm"
              >
                <Camera className="size-4" />
              </button>
              <input ref={picker} type="file" accept={AVATAR_TYPES.join(",")} className="hidden" onChange={(e) => { void pick(e.target.files?.[0] ?? null); e.target.value = ""; }} />
            </div>
            <div className="min-w-0 flex-1 space-y-1.5">
              <label className="block text-xs text-muted-foreground">Grup adı</label>
              <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={NAME_MAX} />
              {showsPhoto && (
                <button type="button" onClick={() => setAvatar("")} className="text-xs text-destructive underline-offset-2 hover:underline">Fotoğrafı kaldır</button>
              )}
            </div>
          </div>
          <div className="space-y-1.5">
            <label className="block text-xs text-muted-foreground">Açıklama</label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              maxLength={DESC_MAX}
              rows={2}
              className="w-full resize-none rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2.5 text-sm outline-none transition focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
            />
            <CharCount value={description} max={DESC_MAX} className="text-right" />
          </div>
          <div className="space-y-1.5">
            <label className="block text-xs text-muted-foreground">Kimler yazabilir</label>
            <PolicyPicker value={policy} onChange={setPolicy} />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
      </Modal>
    </>
  );
}

// AddPeopleDialog adds directly or invites, whichever the caller may do.
export function AddPeopleDialog({ group, mode, open, onClose, onDone }: { group: TeamsGroupDetail; mode: "add" | "invite"; open: boolean; onClose: () => void; onDone: (g: TeamsGroupDetail) => void }) {
  const [selected, setSelected] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (open) {
      setSelected([]);
      setError(null);
    }
  }, [open]);
  const exclude = [...group.members.map((m) => m.id), ...group.invited.map((p) => p.id)];

  async function go() {
    setBusy(true);
    setError(null);
    try {
      const g = mode === "add" ? await api.teamsAddMembers(group.id, selected) : await api.teamsInvite(group.id, selected);
      onDone(g);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İşlem tamamlanamadı.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={mode === "add" ? "Üye ekle" : "Davet gönder"}
      description={mode === "add" ? "Seçtiklerin hemen gruba katılır." : "Seçtiklerine davet gider, kabul edince katılırlar."}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} className="h-9" disabled={busy}>Vazgeç</Button>
          <Button onClick={go} className="h-9" disabled={busy || selected.length === 0}>
            {busy ? "Gönderiliyor..." : mode === "add" ? `Ekle (${selected.length})` : `Davet gönder (${selected.length})`}
          </Button>
        </>
      }
    >
      <PeoplePicker selected={selected} onChange={setSelected} exclude={exclude} height={300} />
      {error && <p className="mt-3 text-sm text-destructive">{error}</p>}
    </Modal>
  );
}

// NewDMDialog picks one person and opens the direct message.
export function NewDMDialog({ open, onClose, onOpened, selfId }: { open: boolean; onClose: () => void; onOpened: (g: TeamsGroupDetail) => void; selfId: number }) {
  const [selected, setSelected] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (open) {
      setSelected([]);
      setError(null);
    }
  }, [open]);

  async function go() {
    if (selected.length !== 1) return;
    setBusy(true);
    setError(null);
    try {
      const g = await api.teamsOpenDM(selected[0]);
      onOpened(g);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Sohbet açılamadı.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Yeni mesaj"
      description="Kime yazmak istiyorsun?"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} className="h-9" disabled={busy}>Vazgeç</Button>
          <Button onClick={go} className="h-9" disabled={busy || selected.length !== 1}>{busy ? "Açılıyor..." : "Sohbeti aç"}</Button>
        </>
      }
    >
      <PeoplePicker selected={selected} onChange={setSelected} exclude={[selfId]} single height={300} />
      {error && <p className="mt-3 text-sm text-destructive">{error}</p>}
    </Modal>
  );
}
