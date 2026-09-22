// ProfileDialog: the agent's own photo. Pick a file, crop it, save; or
// remove it. The cropped webp goes to the server, which checks it again.

import { useEffect, useRef, useState } from "react";
import { Camera, Trash2 } from "lucide-react";
import { api, ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import AvatarCropper from "@/components/profile/AvatarCropper";
import { Button, Modal } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import { AVATAR_TYPES, loadAvatarFile, type AvatarSource } from "@/lib/avatar";

export default function ProfileDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { user, setUser } = useAuth();
  const picker = useRef<HTMLInputElement>(null);
  const [cropping, setCropping] = useState<AvatarSource | null>(null);
  // null: unchanged, "": removed, data URI: new photo waiting to be saved
  const [draft, setDraft] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(null);
      setError(null);
      setCropping(null);
    }
  }, [open]);

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
    if (draft === null) {
      onClose();
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const u = await api.setMyAvatar(draft);
      setUser(u);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  }

  const showing = draft === null ? (user?.hasAvatar ?? false) : draft !== "";
  const dirty = draft !== null;

  return (
    <>
      {cropping && (
        <AvatarCropper
          image={cropping}
          onCancel={() => setCropping(null)}
          onApply={(dataUrl) => {
            setDraft(dataUrl);
            setCropping(null);
          }}
        />
      )}
      <Modal
        open={open && !cropping}
        onClose={onClose}
        title="Profil fotoğrafı"
        description="Üst çubukta, ekip performansında ve kullanıcı listesinde görünür."
        footer={
          <>
            {showing && (
              <Button variant="ghost" onClick={() => setDraft("")} disabled={busy} className="mr-auto h-9 px-3 text-xs text-destructive">
                <Trash2 className="size-3.5" /> Fotoğrafı kaldır
              </Button>
            )}
            <Button variant="secondary" onClick={onClose} className="h-9" disabled={busy}>Vazgeç</Button>
            <Button onClick={save} className="h-9" disabled={busy || !dirty}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
          </>
        }
      >
        <div className="flex flex-col items-center gap-4 py-2">
          <UserAvatar
            userId={user?.id}
            name={user?.name}
            hasAvatar={showing && draft === null}
            version={user?.avatarVersion}
            src={draft || undefined}
            className="size-32 ring-4 ring-border/60"
            fallbackClassName="bg-primary/10 text-3xl text-primary"
          />
          <div className="text-center">
            <div className="font-semibold">{user?.name}</div>
            <div className="text-xs text-muted-foreground">{user?.email}</div>
          </div>
          <input ref={picker} type="file" accept={AVATAR_TYPES.join(",")} className="hidden" onChange={(e) => { void pick(e.target.files?.[0] ?? null); e.target.value = ""; }} />
          <Button variant="secondary" onClick={() => picker.current?.click()} disabled={busy} className="h-10">
            <Camera className="size-4" /> {showing ? "Fotoğrafı değiştir" : "Fotoğraf seç"}
          </Button>
          <p className="text-center text-xs text-muted-foreground">PNG, JPEG, WEBP veya GIF, en fazla 5 MB. Kırpılır, küçültülür ve sıkıştırılır, boyut derdi yok.</p>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
      </Modal>
    </>
  );
}
