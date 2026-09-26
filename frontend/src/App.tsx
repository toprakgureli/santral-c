import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth/AuthContext";
import AppShell from "./components/layout/AppShell";
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
        <Route path="/calls" element={<Calls />} />
        <Route path="/performance" element={<TeamPerformance />} />
        <Route path="/escalation-search" element={<EscalationSearch />} />
        <Route path="/escalations" element={<Escalations />} />
        <Route path="/roles" element={<Roles />} />
        <Route path="/users" element={<Users />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="/teams" element={<Teams />} />
        <Route path="/teams/:id" element={<Teams />} />
        <Route path="/games/admin" element={<GamesAdmin />} />
        <Route path="/profile" element={<Profile />} />
        <Route path="/preferences" element={<Preferences />} />
        <Route path="/profile/:id" element={<Profile />} />
        <Route path="/audit" element={<Audit />} />
        <Route path="/whatsapp/settings" element={<WhatsAppSettings />} />
        <Route path="/whatsapp/bots/:id" element={<WhatsAppBot />} />
        <Route path="/whatsapp/reports" element={<WhatsAppReports />} />
        <Route path="/whatsapp/ratings" element={<WhatsAppRatings />} />
        <Route path="/whatsapp/callbacks" element={<WhatsAppCallbacks />} />
        <Route path="/whatsapp" element={<WhatsApp />} />
        <Route path="/whatsapp/:id" element={<WhatsApp />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
