import { useCallback, useState } from "react";
import { Outlet, useLocation } from "react-router-dom";
import Sidebar from "./Sidebar";
import Topbar from "./Topbar";
import CallBar from "./CallBar";
import UnloadGuard from "./UnloadGuard";
import { SoftphoneProvider } from "@/softphone/SoftphoneContext";
import { ShiftProvider } from "@/shift/ShiftContext";
import { cn } from "@/lib/utils";
import { titleFor } from "@/lib/menu";

const COLLAPSE_KEY = "santral.sidebar";

function readCollapsed() {
  try {
    return window.localStorage.getItem(COLLAPSE_KEY) === "collapsed";
  } catch {
    return false;
  }
}

export default function AppShell() {
  const [menuOpen, setMenuOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const { pathname } = useLocation();

  const toggleCollapse = useCallback(() => {
    setCollapsed((prev) => {
      const next = !prev;
      try {
        window.localStorage.setItem(COLLAPSE_KEY, next ? "collapsed" : "expanded");
      } catch {
        return next;
      }
      return next;
    });
  }, []);

  return (
    <SoftphoneProvider>
    <ShiftProvider>
    <UnloadGuard />
    <div className="min-h-svh bg-background">
      <Sidebar
        open={menuOpen}
        collapsed={collapsed}
        onNavigate={() => setMenuOpen(false)}
        onClose={() => setMenuOpen(false)}
        onToggleCollapse={toggleCollapse}
      />

      {menuOpen && (
        <div className="animate-in fade-in fixed inset-0 z-40 bg-black/40 duration-200 lg:hidden" onClick={() => setMenuOpen(false)} />
      )}

      <div
        className={cn(
          "flex min-h-svh flex-col transition-[padding] duration-300 ease-out",
          collapsed ? "lg:pl-[4.75rem]" : "lg:pl-[16.5rem]",
        )}
      >
        <Topbar title={titleFor(pathname)} onMenuClick={() => setMenuOpen((v) => !v)} />
        <main className="flex-1 px-4 py-6 md:px-6 lg:px-8">
          {/* Keyed by path so the entrance animation replays on each navigation. */}
          <div key={pathname} className="animate-in fade-in slide-in-from-bottom-2 duration-300 ease-out">
            <Outlet />
          </div>
        </main>
      </div>
      <CallBar />
    </div>
    </ShiftProvider>
    </SoftphoneProvider>
  );
}
