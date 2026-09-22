// User management. Role assignment, deactivation, password reset and the SIP
// account are all edited from the row's form. Deactivation and password reset
// also end the user's open sessions.

import { useEffect, useRef, useState } from "react";
import { Plus, Search, Users as UsersIcon } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { Role, User } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Badge, Button, Card, EmptyState, Input, Pagination, Select, Skeleton } from "../components/ui";
import UserForm from "../components/user/UserForm";
import CredentialsHandoff, { type Handoff } from "../components/user/CredentialsHandoff";
import UserAvatar from "../components/ui/UserAvatar";
import { cn, formatDateTime } from "../lib/utils";

const PER_PAGE = 25;

export function Users() {
  const { user } = useAuth();
  const canCreate = can(user, "user.create");
  const canUpdate = can(user, "user.update");
  const canSip = canUpdate || can(user, "agent.manage");

  const [roles, setRoles] = useState<Role[]>([]);
  const [query, setQuery] = useState("");
  const [roleId, setRoleId] = useState("");
  const [active, setActive] = useState("");
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<User[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<User | null>(null);
  const [creating, setCreating] = useState(false);
  const [handoff, setHandoff] = useState<Handoff | null>(null);
  const [tick, setTick] = useState(0);
  // A fast filter change can let an older, slower response land last; the
  // sequence number makes sure only the newest request paints.
  const seq = useRef(0);

  useEffect(() => {
    const id = ++seq.current;
    setLoading(true);
    const wait = query ? 350 : 0;
    const timer = window.setTimeout(() => {
      api
        .listUsers({ query: query.trim() || undefined, roleId: roleId || undefined, active: active || undefined, page, perPage: PER_PAGE })
        .then((r) => {
          if (id !== seq.current) return;
          setItems(r.items);
          setTotal(r.total);
        })
        .catch(() => {
          if (id !== seq.current) return;
          setItems([]);
          setTotal(0);
        })
        .finally(() => {
          if (id === seq.current) setLoading(false);
        });
    }, wait);
    return () => window.clearTimeout(timer);
  }, [query, roleId, active, page, tick]);

  useEffect(() => {
    api.listRoles().then(setRoles).catch(() => setRoles([]));
  }, []);

  const reload = () => setTick((n) => n + 1);
  const roleName = (id: number) => roles.find((r) => r.id === id)?.displayName;

  const closeAndReload = () => {
    setEditing(null);
    setCreating(false);
    reload();
  };

  return (
    <div className="space-y-6">
      <Card
        title="Kullanıcılar"
        actions={
          <div className="flex flex-wrap items-center justify-end gap-2">
            <span className="text-xs text-muted-foreground">{total} kayıt</span>
            {canSip && <SyncSipButton onDone={reload} />}
            {canCreate && (
              <Button onClick={() => setCreating(true)}>
                <Plus />
                Yeni Kullanıcı
              </Button>
            )}
          </div>
        }
      >
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <Select value={roleId} onChange={(e) => { setRoleId(e.target.value); setPage(1); }} className="w-auto min-w-40">
            <option value="">Tüm roller</option>
            {roles.map((r) => (
              <option key={r.id} value={r.id}>{r.displayName}</option>
            ))}
          </Select>
          <Select value={active} onChange={(e) => { setActive(e.target.value); setPage(1); }} className="w-auto min-w-36">
            <option value="">Tüm durumlar</option>
            <option value="true">Aktif</option>
            <option value="false">Pasif</option>
          </Select>
          <div className="relative ml-auto w-full sm:w-64">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input value={query} onChange={(e) => { setQuery(e.target.value); setPage(1); }} placeholder="İsim veya e-posta ara..." className="pl-9" />
          </div>
        </div>

        {loading && items.length === 0 ? (
          <div className="space-y-2">
            {[0, 1, 2, 3].map((row) => (
              <Skeleton key={row} className="h-12 w-full rounded-xl" />
            ))}
          </div>
        ) : !items.length ? (
          <EmptyState icon={<UsersIcon />} title="Kullanıcı bulunamadı" description="Filtreleri değiştirip tekrar deneyin." />
        ) : (
          <div className={cn("overflow-x-auto transition-opacity", loading && "opacity-60")}>
            <table className="w-full min-w-[44rem] text-sm">
              <thead>
                <tr className="text-left text-xs text-muted-foreground">
                  <th className="pb-2">Kullanıcı</th>
                  <th className="pb-2">Roller</th>
                  <th className="pb-2">Dahili</th>
                  <th className="pb-2">Durum</th>
                  <th className="pb-2">Son Giriş</th>
                </tr>
              </thead>
              <tbody>
                {items.map((u) => (
                  <tr
                    key={u.id}
                    onClick={() => canUpdate && setEditing(u)}
                    className={cn("border-t border-border/60", canUpdate && "cursor-pointer transition-colors hover:bg-accent/50")}
                  >
                    <td className="py-2.5">
                      <span className="flex items-center gap-2.5">
                        <UserAvatar userId={u.id} name={u.name} hasAvatar={u.hasAvatar} version={u.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
                        <span className="min-w-0">
                          <span className="block font-medium">{u.name}</span>
                          <span className="block text-xs text-muted-foreground">{u.email}</span>
                        </span>
                      </span>
                    </td>
                    <td className="py-2.5">
                      <div className="flex flex-wrap gap-1">
                        {u.roleIds.map((id, i) => (
                          <Badge key={id} tone="blue">{roleName(id) ?? u.roles[i] ?? id}</Badge>
                        ))}
                      </div>
                    </td>
                    <td className="py-2.5 tabular-nums">{u.sipExtension ?? "—"}</td>
                    <td className="py-2.5">
                      <Badge tone={u.active ? "green" : "red"}>{u.active ? "Aktif" : "Pasif"}</Badge>
                    </td>
                    <td className="py-2.5 whitespace-nowrap text-muted-foreground">{formatDateTime(u.lastLoginAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <Pagination page={page} perPage={PER_PAGE} total={total} onChange={setPage} />
      </Card>

      {(creating || editing) && (
        <UserForm
          user={editing}
          roles={roles}
          onClose={() => { setEditing(null); setCreating(false); }}
          onSaved={closeAndReload}
          onHandoff={setHandoff}
        />
      )}

      {handoff && <CredentialsHandoff handoff={handoff} onClose={() => setHandoff(null)} />}
    </div>
  );
}

function SyncSipButton({ onDone }: { onDone: () => void }) {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  async function run() {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api.syncAllSip();
      let fails = "";
      if (r.failures?.length) {
        fails = " · " + r.failures.map((f) => `${f.extension}: ${f.reason}`).join(" · ");
      } else if (r.failedExtensions?.length) {
        fails = ` (dahili ${r.failedExtensions.join(", ")})`;
      }
      setMsg(`${r.synced} çekildi${r.failed ? ` · ${r.failed} başarısız${fails}` : ""}`);
      onDone();
    } catch (e) {
      setMsg(e instanceof ApiError ? e.message : "Hata");
    } finally {
      setBusy(false);
    }
  }
  return (
    <span className="flex items-center gap-2">
      {msg && <span className="max-w-md text-xs text-muted-foreground">{msg}</span>}
      <Button variant="secondary" onClick={run} disabled={busy}>{busy ? "Senkronize..." : "SIP Senkronize (Verimor)"}</Button>
    </span>
  );
}
