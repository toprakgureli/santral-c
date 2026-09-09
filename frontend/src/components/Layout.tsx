import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { can, canAny } from "../lib/permissions";
import { Button } from "./ui";

interface NavItem {
  to: string;
  label: string;
  show: (perms: (p: string) => boolean, permsAny: (p: string[]) => boolean) => boolean;
}

const items: NavItem[] = [
  { to: "/", label: "Panel", show: () => true },
  { to: "/calls", label: "Çağrılar", show: (_p, any) => any(["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"]) },
  { to: "/contacts", label: "Kişiler", show: (p) => p("contact.view") },
  { to: "/users", label: "Kullanıcılar", show: (p) => p("user.view") },
];

export function Layout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const p = (perm: string) => can(user, perm);
  const any = (perms: string[]) => canAny(user, perms);

  return (
    <div className="flex min-h-full flex-col">
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-6 py-3">
        <div className="flex items-center gap-8">
          <span className="text-lg font-bold text-brand-600">santral-c</span>
          <nav className="flex gap-1">
            {items.filter((i) => i.show(p, any)).map((i) => (
              <NavLink
                key={i.to}
                to={i.to}
                end={i.to === "/"}
                className={({ isActive }) =>
                  `rounded-lg px-3 py-1.5 text-sm font-medium ${isActive ? "bg-brand-50 text-brand-700" : "text-slate-600 hover:bg-slate-100"}`
                }
              >
                {i.label}
              </NavLink>
            ))}
          </nav>
        </div>
        <div className="flex items-center gap-3">
          <div className="text-right">
            <div className="text-sm font-medium text-slate-700">{user?.name}</div>
            <div className="text-xs text-slate-400">{user?.roles.join(", ")}</div>
          </div>
          <Button
            variant="secondary"
            onClick={async () => {
              await logout();
              navigate("/login");
            }}
          >
            Çıkış
          </Button>
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-6 py-6">
        <Outlet />
      </main>
    </div>
  );
}
