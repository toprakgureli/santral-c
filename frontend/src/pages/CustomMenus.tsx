import { useCallback, useEffect, useState } from "react";
import {
  ArrowDown,
  ArrowUp,
  ExternalLink,
  Eye,
  EyeOff,
  Globe,
  Info,
  LayoutGrid,
  Loader2,
  Plus,
  Trash2,
} from "lucide-react";
import { api } from "@/api/client";
import type { CustomMenuInput, CustomMenuItem, CustomMenuOpenMode, CustomMenuRole } from "@/api/types";
import AuthError from "@/components/auth/AuthError";
import { errorMessage } from "@/components/auth/messages";
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  Field,
  FieldGroup,
  FieldHint,
  Input,
  Modal,
  Notice,
  Select,
  Skeleton,
  useDirty,
} from "@/components/ui";
import { ListRow } from "@/components/ui/rows";
import { CUSTOM_MENU_ICONS, customMenuIcon } from "@/lib/menuIcons";
import { cn } from "@/lib/utils";
import { useCustomMenus } from "@/menus/CustomMenusContext";

export function CustomMenus() {
  const { refresh: refreshSidebar } = useCustomMenus();
  const [items, setItems] = useState<CustomMenuItem[]>([]);
  const [roles, setRoles] = useState<CustomMenuRole[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [editing, setEditing] = useState<CustomMenuItem | null>(null);
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    const [menuList, roleList] = await Promise.all([
      api.menusList().catch(() => [] as CustomMenuItem[]),
      api.menusRoles().catch(() => [] as CustomMenuRole[]),
    ]);
    setItems(menuList);
    setRoles(roleList);
    setLoading(false);
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const syncAll = async () => {
    await Promise.all([load(), refreshSidebar()]);
  };

  const toggleActive = async (item: CustomMenuItem) => {
    setBusyId(item.id);
    try {
      await api.menuUpdate(item.id, {
        label: item.label,
        icon: item.icon,
        url: item.url,
        openMode: item.openMode,
        active: !item.active,
        roleIds: item.roleIds,
      });
      await syncAll();
    } finally {
      setBusyId(null);
    }
  };

  const moveItem = async (index: number, direction: -1 | 1) => {
    const target = index + direction;
    if (target < 0 || target >= items.length) return;
    const next = [...items];
    const [moved] = next.splice(index, 1);
    next.splice(target, 0, moved);
    setItems(next);
    try {
      await api.menusReorder(next.map((m) => m.id));
      await refreshSidebar();
    } catch {
      await load();
    }
  };

  return (
    <div className="space-y-6">
      <Card
        title="Özel Menüler ve Harici Uygulamalar"
        icon={LayoutGrid}
        actions={
          <div className="flex items-center gap-3">
            <span className="text-xs text-muted-foreground">{items.length} menü</span>
            <Button onClick={() => setCreating(true)}>
              <Plus />
              Yeni Menü Ekle
            </Button>
          </div>
        }
      >
        <div className="mb-4">
          <Notice icon={<Info />}>
            Sol menüye Telegram Web, CRM, proje yönetim aracı veya şirket içi web uygulamalarını ekleyebilirsin.
            Uygulama içi pencerede (iframe) açılan siteler menüler arasında geçiş yapılırken arka planda korunur ve tekrar giriş gerektirmez.
          </Notice>
        </div>

        {loading ? (
          <div className="space-y-2">
            {[0, 1, 2].map((row) => (
              <Skeleton key={row} className="h-14 w-full rounded-xl" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <EmptyState
            icon={<Globe />}
            title="Henüz özel menü eklenmemiş"
            description="Sol menüye Telegram, CRM veya herhangi bir harici web uygulaması eklemek için Yeni Menü Ekle butonunu kullan."
            action={
              <Button onClick={() => setCreating(true)}>
                <Plus />
                İlk Menüyü Oluştur
              </Button>
            }
          />
        ) : (
          <div className="space-y-1.5">
            {items.map((item, idx) => {
              const Icon = customMenuIcon(item.icon);
              const isBusy = busyId === item.id;
              return (
                <ListRow
                  key={item.id}
                  icon={Icon}
                  tone={item.active ? "primary" : "muted"}
                  onClick={() => setEditing(item)}
                  title={
                    <span className="flex flex-wrap items-center gap-2">
                      <span>{item.label}</span>
                      <Badge tone={item.active ? "green" : "slate"}>
                        {item.active ? "Aktif" : "Pasif"}
                      </Badge>
                      <Badge tone={item.openMode === "new_tab" ? "amber" : "blue"}>
                        {item.openMode === "new_tab" ? "Yeni Sekme" : "Uygulama İçi (Iframe)"}
                      </Badge>
                    </span>
                  }
                  sub={
                    <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                      <span className="truncate font-mono text-[0.7rem]">{item.url}</span>
                      <span>·</span>
                      <span>
                        {item.roles.length === 0
                          ? "Tüm roller görebilir"
                          : `Roller: ${item.roles.map((r) => r.displayName).join(", ")}`}
                      </span>
                    </span>
                  }
                  trailing={
                    <div
                      className="flex items-center gap-1"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Button
                        variant="ghost"
                        className="size-8 p-0"
                        disabled={idx === 0 || isBusy}
                        onClick={() => void moveItem(idx, -1)}
                        aria-label="Yukarı taşı"
                        data-tip="Yukarı taşı"
                      >
                        <ArrowUp className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        className="size-8 p-0"
                        disabled={idx === items.length - 1 || isBusy}
                        onClick={() => void moveItem(idx, 1)}
                        aria-label="Aşağı taşı"
                        data-tip="Aşağı taşı"
                      >
                        <ArrowDown className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        className="h-8 px-2.5 text-xs"
                        disabled={isBusy}
                        onClick={() => void toggleActive(item)}
                        data-tip={item.active ? "Menüyü pasife al" : "Menüyü aktifleştir"}
                      >
                        {item.active ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                        <span className="hidden sm:inline">{item.active ? "Pasife Al" : "Aktifleştir"}</span>
                      </Button>
                    </div>
                  }
                />
              );
            })}
          </div>
        )}
      </Card>

      {(creating || editing) && (
        <CustomMenuModal
          item={editing}
          roles={roles}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
          onSaved={async () => {
            setCreating(false);
            setEditing(null);
            await syncAll();
          }}
        />
      )}
    </div>
  );
}

type CustomMenuModalProps = {
  item: CustomMenuItem | null;
  roles: CustomMenuRole[];
  onClose: () => void;
  onSaved: () => Promise<void>;
};

function CustomMenuModal({ item, roles, onClose, onSaved }: CustomMenuModalProps) {
  const [label, setLabel] = useState(item?.label ?? "");
  const [icon, setIcon] = useState(item?.icon ?? "globe");
  const [url, setUrl] = useState(item?.url ?? "https://");
  const [openMode, setOpenMode] = useState<CustomMenuOpenMode>(item?.openMode ?? "iframe");
  const [active, setActive] = useState(item?.active ?? true);
  const [selectedRoles, setSelectedRoles] = useState<Set<number>>(() => new Set(item?.roleIds ?? []));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const dirty = useDirty([label, icon, url, openMode, active, [...selectedRoles].sort()]);

  const toggleRole = (roleId: number) => {
    setSelectedRoles((prev) => {
      const next = new Set(prev);
      if (next.has(roleId)) next.delete(roleId);
      else next.add(roleId);
      return next;
    });
  };

  const save = async () => {
    const trimmedLabel = label.trim();
    const trimmedUrl = url.trim();
    if (!trimmedLabel) {
      setError("Menü adı zorunludur.");
      return;
    }
    if (!trimmedUrl.startsWith("http://") && !trimmedUrl.startsWith("https://")) {
      setError("Açılacak URL http:// veya https:// ile başlamalıdır.");
      return;
    }

    setBusy(true);
    setError(null);
    const payload: CustomMenuInput = {
      label: trimmedLabel,
      icon,
      url: trimmedUrl,
      openMode,
      active,
      roleIds: [...selectedRoles],
    };

    try {
      if (item) {
        await api.menuUpdate(item.id, payload);
      } else {
        await api.menuCreate(payload);
      }
      await onSaved();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!item) return;
    setConfirmDelete(false);
    setBusy(true);
    setError(null);
    try {
      await api.menuDelete(item.id);
      await onSaved();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      dirty={dirty && !busy}
      onClose={onClose}
      size="lg"
      title={item ? `${item.label} Menüsünü Düzenle` : "Yeni Özel Menü Oluştur"}
      description="Sol menüde görünecek harici uygulama butonunu yapılandır"
      footer={
        <>
          {item && (
            <Button
              variant="ghost"
              className="mr-auto text-destructive hover:text-destructive"
              onClick={() => setConfirmDelete(true)}
              disabled={busy}
            >
              <Trash2 />
              Sil
            </Button>
          )}
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            Vazgeç
          </Button>
          <Button onClick={() => void save()} disabled={busy}>
            {busy && <Loader2 className="animate-spin" />}
            {busy ? "Kaydediliyor..." : "Kaydet"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {error && <AuthError message={error} />}

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Menü Adı">
            <Input
              value={label}
              maxLength={120}
              onChange={(e) => {
                setLabel(e.target.value);
                setError(null);
              }}
              placeholder="Örn: Telegram, CRM, Jira..."
              autoFocus
            />
          </Field>

          <Field label="Açılış Yöntemi">
            <Select
              value={openMode}
              onChange={(e) => setOpenMode(e.target.value as CustomMenuOpenMode)}
            >
              <option value="iframe">Santral İçinde Aç (Iframe - Oturum Korumalı)</option>
              <option value="new_tab">Yeni Tarayıcı Sekmesinde Aç</option>
            </Select>
          </Field>
        </div>

        <div className="space-y-1.5">
          <Field label="Hedef URL">
            <Input
              value={url}
              onChange={(e) => {
                setUrl(e.target.value);
                setError(null);
              }}
              placeholder="https://web.telegram.org/k/"
            />
          </Field>
          <FieldHint>
            Örn: Telegram için <code>https://web.telegram.org/k/</code> veya şirket içi uygulama adresi.
          </FieldHint>
        </div>

        <div className="flex items-center justify-between rounded-xl border border-border/60 bg-muted/25 px-3.5 py-2.5">
          <div>
            <p className="text-xs font-semibold">Menü Durumu</p>
            <p className="text-[0.7rem] text-muted-foreground">
              Pasif menüler sol menüde gizlenir ancak ayarları korunur.
            </p>
          </div>
          <label className="inline-flex cursor-pointer items-center gap-2 text-xs font-medium">
            <input
              type="checkbox"
              checked={active}
              onChange={(e) => setActive(e.target.checked)}
              className="size-4 accent-primary"
            />
            <span>{active ? "Aktif" : "Pasif"}</span>
          </label>
        </div>

        <FieldGroup label="Menü İkonu">
          <div className="grid grid-cols-3 gap-2 sm:grid-cols-5">
            {CUSTOM_MENU_ICONS.map((entry) => {
              const IconComp = entry.icon;
              const isSelected = icon === entry.key;
              return (
                <button
                  key={entry.key}
                  type="button"
                  onClick={() => setIcon(entry.key)}
                  className={cn(
                    "flex flex-col items-center gap-1.5 rounded-xl border p-2.5 text-center transition-all",
                    isSelected
                      ? "border-primary bg-primary/10 text-primary shadow-sm"
                      : "border-border/60 bg-muted/20 text-muted-foreground hover:border-border hover:bg-accent/50 hover:text-foreground",
                  )}
                >
                  <IconComp className="size-4 shrink-0" />
                  <span className="w-full truncate text-[0.65rem] font-medium">{entry.label}</span>
                </button>
              );
            })}
          </div>
        </FieldGroup>

        <FieldGroup label="Görebilecek Roller">
          <p className="mb-2 text-xs text-muted-foreground">
            Hiçbir rol seçilmezse bu menüyü oturum açmış tüm kullanıcılar görebilir. Sadece belirli rollerin görmesini istiyorsan aşağıdan seçebilirsin.
          </p>
          <div className="grid gap-2 sm:grid-cols-2">
            {roles.map((r) => {
              const checked = selectedRoles.has(r.id);
              return (
                <label
                  key={r.id}
                  className={cn(
                    "flex cursor-pointer items-center gap-2.5 rounded-xl border px-3 py-2 text-xs transition-colors",
                    checked
                      ? "border-primary/50 bg-primary/10 font-medium text-foreground"
                      : "border-border/60 bg-muted/20 text-muted-foreground hover:bg-accent/40 hover:text-foreground",
                  )}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggleRole(r.id)}
                    className="size-3.5 shrink-0 accent-primary"
                  />
                  <span className="truncate">{r.displayName}</span>
                </label>
              );
            })}
          </div>
        </FieldGroup>

        <div className="flex items-center gap-2 rounded-xl bg-muted/40 px-3 py-2 text-[0.7rem] text-muted-foreground">
          <ExternalLink className="size-3.5 shrink-0" />
          <span>
            Bazı harici siteler (<code>X-Frame-Options: DENY</code> veya <code>SameSite</code> çerez politikası kullananlar) iframe içinde oturum açmaya izin vermeyebilir. Bu durumda sistem otomatik olarak yeni sekmede açma seçeneği sunar.
          </span>
        </div>
      </div>

      <ConfirmDialog
        open={confirmDelete}
        title="Özel menü silinsin mi?"
        description={`${item?.label ?? "Bu menü"} sol menüden kalıcı olarak kaldırılır.`}
        confirmLabel="Sil"
        onConfirm={() => void remove()}
        onCancel={() => setConfirmDelete(false)}
      />
    </Modal>
  );
}
