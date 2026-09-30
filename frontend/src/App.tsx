import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth/AuthContext";
import AppShell from "./components/layout/AppShell";
import RequirePermission from "./components/layout/RequirePermission";
import { menuPermission, PAGE_PERMISSIONS } from "./lib/menu";
import { Spinner } from "./components/ui";
import { Audit } from "./pages/Audit";
import { Calls } from "./pages/Calls";
import { Dashboard } from "./pages/Dashboard";
import { EscalationSearch } from "./pages/EscalationSearch";
import { Escalations } from "./pages/Escalations";
import { Login } from "./pages/Login";
import { Profile } from "./pages/Profile";
import { Teams } from "./pages/Teams";
import { GamesAdmin } from "./pages/GamesAdmin";
import { Roles } from "./pages/Roles";
import { Settings } from "./pages/Settings";
import { TeamPerformance } from "./pages/TeamPerformance";
import { Users } from "./pages/Users";
import { WhatsApp } from "./pages/WhatsApp";
import { Preferences } from "./pages/Preferences";
import { WhatsAppSettings } from "./pages/WhatsAppSettings";
import { WhatsAppBot } from "./pages/WhatsAppBot";
import { WhatsAppReports } from "./pages/WhatsAppReports";
import { WhatsAppRatings } from "./pages/WhatsAppRatings";
import { WhatsAppCallbacks } from "./pages/WhatsAppCallbacks";

export function App() {
  const { user, loading } = useAuth();

  if (loading) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Spinner />
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
        <Route path="/preferences" element={<Preferences />} />
        <Route path="/profile/:id" element={<Profile />} />
        <Route path="/audit" element={<RequirePermission need={menuPermission("/audit")}><Audit /></RequirePermission>} />
        <Route path="/whatsapp/settings" element={<RequirePermission need={PAGE_PERMISSIONS.whatsappSettings}><WhatsAppSettings /></RequirePermission>} />
        <Route path="/whatsapp/bots/:id" element={<RequirePermission need={PAGE_PERMISSIONS.whatsappBot}><WhatsAppBot /></RequirePermission>} />
        <Route path="/whatsapp/reports" element={<RequirePermission need={PAGE_PERMISSIONS.whatsappReports}><WhatsAppReports /></RequirePermission>} />
        <Route path="/whatsapp/ratings" element={<RequirePermission need={PAGE_PERMISSIONS.whatsappRatings}><WhatsAppRatings /></RequirePermission>} />
        <Route path="/whatsapp/callbacks" element={<RequirePermission need={PAGE_PERMISSIONS.whatsappCallbacks}><WhatsAppCallbacks /></RequirePermission>} />
        <Route path="/whatsapp" element={<RequirePermission need={menuPermission("/whatsapp")}><WhatsApp /></RequirePermission>} />
        <Route path="/whatsapp/:id" element={<RequirePermission need={menuPermission("/whatsapp")}><WhatsApp /></RequirePermission>} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
