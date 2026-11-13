import { useState } from "react";
import { Terminal } from "lucide-react";
import { Link } from "react-router-dom";
import { ApiError } from "@/api/client";
import { apiTesterApi } from "@/apiTester/api";
import { useApiTesterFeature } from "@/apiTester/ApiTesterContext";
import { useAuth } from "@/auth/AuthContext";
import { Badge, Card } from "@/components/ui";
import { Switch } from "@/components/whatsapp/settings/parts";
import { can } from "@/lib/permissions";

export default function ApiTesterCard() {
  const { user } = useAuth();
  const { enabled, loading, refresh, setEnabledLocal } = useApiTesterFeature();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canManageCollections = can(user, "api_tester.manage");

  const toggle = async (next: boolean) => {
    setBusy(true);
    setError(null);
    try {
      const res = await apiTesterApi.setStatus(next);
      setEnabledLocal(res.enabled);
      await refresh();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Ayar güncellenemedi.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card
      title="API Test Koleksiyonları"
      icon={Terminal}
      actions={<Badge tone={enabled ? "green" : "slate"}>{enabled ? "Aktif" : "Pasif"}</Badge>}
    >
      <div className="space-y-3">
        <div className="flex items-start justify-between gap-4 rounded-xl border border-border/60 p-3.5">
          <div className="min-w-0 space-y-1">
            <p className="text-sm font-medium">Genel menüsü altında API Testleri butonunu göster</p>
            <p className="text-xs leading-relaxed text-muted-foreground">
              Teknik destek uzmanlarının yetkisine bağlı olarak müşteri projelerine ait API test koleksiyonlarını kendi IP adreslerinden çalıştırabildiği ekranı açar veya gizler.
            </p>
          </div>
          <Switch
            on={enabled}
            disabled={busy || loading}
            onChange={(next) => void toggle(next)}
            label="API Testleri menüsünü aç veya kapat"
          />
        </div>
        {canManageCollections && (
          <Link
            to="/api-tester"
            className="inline-flex h-9 items-center rounded-xl bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
          >
            API koleksiyonlarını aç
          </Link>
        )}
        {error && <p className="text-sm text-destructive">{error}</p>}
      </div>
    </Card>
  );
}
