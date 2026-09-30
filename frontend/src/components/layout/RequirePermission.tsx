import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Lock } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import { Card, EmptyState } from "@/components/ui";
import { allows } from "@/lib/menu";

// RequirePermission shows a page only to someone who holds one of the
// permissions it needs. Anyone else, for example after opening a saved
// link, gets a clear notice instead of an empty or broken page.
export default function RequirePermission({ need, children }: { need?: string | string[]; children: ReactNode }) {
  const { can } = useAuth();
  if (allows(can, need)) return <>{children}</>;
  return (
    <Card>
      <EmptyState
        icon={<Lock />}
        title="Bu sayfayı görme yetkin yok"
        description="Bu sayfa için gereken yetki rolünde tanımlı değil. Erişmen gerekiyorsa yöneticinden isteyebilirsin."
        action={
          <Link to="/" className="inline-flex h-9 items-center rounded-xl bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90">
            Ana sayfaya dön
          </Link>
        }
      />
    </Card>
  );
}
