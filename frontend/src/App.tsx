import { lazy } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth/AuthContext";
import AppShell from "./components/layout/AppShell";
import RequirePermission from "./components/layout/RequirePermission";
import { menuPermission, PAGE_PERMISSIONS, WA_BASE } from "./lib/menu";
import { Button, Spinner } from "./components/ui";
import { Dashboard } from "./pages/Dashboard";
import { Login } from "./pages/Login";

// Pages load when they are first opened, so the panel starts with what the
// home page needs; the softphone's SIP library loads only for those with a
// phone line.
const Audit = lazy(() => import("./pages/Audit").then((m) => ({ default: m.Audit })));
const Calls = lazy(() => import("./pages/Calls").then((m) => ({ default: m.Calls })));
const EscalationSearch = lazy(() => import("./pages/EscalationSearch").then((m) => ({ default: m.EscalationSearch })));
const Escalations = lazy(() => import("./pages/Escalations").then((m) => ({ default: m.Escalations })));
const Profile = lazy(() => import("./pages/Profile").then((m) => ({ default: m.Profile })));
const Teams = lazy(() => import("./pages/Teams").then((m) => ({ default: m.Teams })));
const GamesAdmin = lazy(() => import("./pages/GamesAdmin").then((m) => ({ default: m.GamesAdmin })));
const Roles = lazy(() => import("./pages/Roles").then((m) => ({ default: m.Roles })));
const Settings = lazy(() => import("./pages/Settings").then((m) => ({ default: m.Settings })));
const TeamPerformance = lazy(() => import("./pages/TeamPerformance").then((m) => ({ default: m.TeamPerformance })));
const Users = lazy(() => import("./pages/Users").then((m) => ({ default: m.Users })));
const WhatsApp = lazy(() => import("./pages/WhatsApp").then((m) => ({ default: m.WhatsApp })));
const Account = lazy(() => import("./pages/Account").then((m) => ({ default: m.Account })));
const WhatsAppPreferences = lazy(() => import("./pages/WhatsAppPreferences").then((m) => ({ default: m.WhatsAppPreferences })));
const WhatsAppSettings = lazy(() => import("./pages/WhatsAppSettings").then((m) => ({ default: m.WhatsAppSettings })));
const WhatsAppBot = lazy(() => import("./pages/WhatsAppBot").then((m) => ({ default: m.WhatsAppBot })));
const WhatsAppReports = lazy(() => import("./pages/WhatsAppReports").then((m) => ({ default: m.WhatsAppReports })));
const WhatsAppRatings = lazy(() => import("./pages/WhatsAppRatings").then((m) => ({ default: m.WhatsAppRatings })));
const WhatsAppCallbacks = lazy(() => import("./pages/WhatsAppCallbacks").then((m) => ({ default: m.WhatsAppCallbacks })));

export function App() {
  const { user, loading, waiting, retryNow } = useAuth();

  if (loading) {
    return (
      <div className="flex min-h-svh items-center justify-center px-4">
        {waiting ? (
          // The server cannot be reached yet (a deploy, a network blip): keep
          // trying calmly instead of showing the sign-in page.
          <div className="flex max-w-sm flex-col items-center gap-3 text-center" role="status" aria-live="polite">
            <Spinner />
            <p className="text-sm font-medium">Bağlantı bekleniyor</p>
            <p className="text-xs leading-relaxed text-muted-foreground">
              Sunucuya şu an ulaşılamıyor. Birkaç saniye içinde kendiliğinden yeniden deniyoruz; oturumun açık kalır.
            </p>
            <Button variant="secondary" onClick={retryNow} className="mt-1">
              Şimdi dene
            </Button>
          </div>
        ) : (
          <Spinner />
        )}
      </div>
    );
  }

  if (!user) {
    return (
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );
  }

  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route path="/" element={<Dashboard />} />
        <Route path="/calls" element={<RequirePermission need={menuPermission("/calls")}><Calls /></RequirePermission>} />
        <Route path="/performance" element={<RequirePermission need={menuPermission("/performance")}><TeamPerformance /></RequirePermission>} />
        <Route path="/escalation-search" element={<RequirePermission need={menuPermission("/escalation-search")}><EscalationSearch /></RequirePermission>} />
        <Route path="/escalations" element={<RequirePermission need={menuPermission("/escalations")}><Escalations /></RequirePermission>} />
        <Route path="/roles" element={<RequirePermission need={menuPermission("/roles")}><Roles /></RequirePermission>} />
        <Route path="/users" element={<RequirePermission need={menuPermission("/users")}><Users /></RequirePermission>} />
        <Route path="/settings" element={<RequirePermission need={menuPermission("/settings")}><Settings /></RequirePermission>} />
        <Route path="/teams" element={<RequirePermission need={menuPermission("/teams")}><Teams /></RequirePermission>} />
        <Route path="/teams/:id" element={<RequirePermission need={menuPermission("/teams")}><Teams /></RequirePermission>} />
        <Route path="/games/admin" element={<RequirePermission need={PAGE_PERMISSIONS.gamesAdmin}><GamesAdmin /></RequirePermission>} />
        <Route path="/profile" element={<Profile />} />
        <Route path="/account" element={<Account />} />
        {/* The old address of the WhatsApp preferences, kept for bookmarks. */}
        <Route path="/preferences" element={<Navigate to="/whatsapp/preferences" replace />} />
        <Route path="/profile/:id" element={<Profile />} />
        <Route path="/audit" element={<RequirePermission need={menuPermission("/audit")}><Audit /></RequirePermission>} />
        <Route path="/whatsapp/settings" element={<RequirePermission base={WA_BASE} need={PAGE_PERMISSIONS.whatsappSettings}><WhatsAppSettings /></RequirePermission>} />
        <Route path="/whatsapp/bots/:id" element={<RequirePermission base={WA_BASE} need={PAGE_PERMISSIONS.whatsappBot}><WhatsAppBot /></RequirePermission>} />
        <Route path="/whatsapp/reports" element={<RequirePermission base={WA_BASE} need={PAGE_PERMISSIONS.whatsappReports}><WhatsAppReports /></RequirePermission>} />
        <Route path="/whatsapp/ratings" element={<RequirePermission base={WA_BASE} need={PAGE_PERMISSIONS.whatsappRatings}><WhatsAppRatings /></RequirePermission>} />
        <Route path="/whatsapp/callbacks" element={<RequirePermission base={WA_BASE} need={PAGE_PERMISSIONS.whatsappCallbacks}><WhatsAppCallbacks /></RequirePermission>} />
        <Route path="/whatsapp/preferences" element={<RequirePermission need={menuPermission("/whatsapp")}><WhatsAppPreferences /></RequirePermission>} />
        <Route path="/whatsapp" element={<RequirePermission need={menuPermission("/whatsapp")}><WhatsApp /></RequirePermission>} />
        <Route path="/whatsapp/:id" element={<RequirePermission need={menuPermission("/whatsapp")}><WhatsApp /></RequirePermission>} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
