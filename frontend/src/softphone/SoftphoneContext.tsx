import { createContext, useContext, type ReactNode } from "react";
import { useAuth } from "@/auth/AuthContext";
import { useSoftphone, type Phone } from "./useSoftphone";

const Ctx = createContext<Phone | null>(null);

// SoftphoneProvider runs the SIP user agent once for the whole session, so the
// registration and any active call survive navigation between pages. The audio
// sink lives here too, not inside a page, for the same reason.
export function SoftphoneProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const phone = useSoftphone(!!user?.sipExtension);

  return (
    <Ctx.Provider value={phone}>
      {children}
      <audio ref={phone.audioRef} autoPlay className="hidden" />
    </Ctx.Provider>
  );
}

export function useSoftphoneContext(): Phone {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useSoftphoneContext must be used within SoftphoneProvider");
  return ctx;
}
