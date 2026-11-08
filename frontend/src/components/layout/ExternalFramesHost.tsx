import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  AppWindow,
  ArrowLeft,
  ArrowRight,
  ExternalLink,
  Globe,
  Home,
  Info,
  PowerOff,
  RefreshCw,
  ShieldAlert,
} from "lucide-react";
import { api } from "@/api/client";
import type { CustomMenuEmbedCheck, CustomMenuItem } from "@/api/types";
import { Badge, Button, Card, EmptyState, Spinner } from "@/components/ui";
import { customMenuIcon } from "@/lib/menuIcons";
import { cn } from "@/lib/utils";
import { useCustomMenus } from "@/menus/CustomMenusContext";

function parseAppId(pathname: string): number | null {
  const m = /^\/app\/(\d+)\/?$/.exec(pathname);
  if (!m) return null;
  const n = Number(m[1]);
  return Number.isFinite(n) && n > 0 ? n : null;
}

function hostOf(rawUrl: string): string {
  try {
    return new URL(rawUrl).host;
  } catch {
    return rawUrl;
  }
}

function openPopupWindow(menu: CustomMenuItem) {
  const width = Math.min(1260, Math.round(window.screen.availWidth * 0.85));
  const height = Math.min(840, Math.round(window.screen.availHeight * 0.85));
  const left = Math.max(20, Math.round((window.screen.availWidth - width) / 2));
  const top = Math.max(20, Math.round((window.screen.availHeight - height) / 2));
  window.open(
    menu.url,
    `santral_popup_${menu.id}`,
    `popup=yes,width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,location=no,status=no,resizable=yes,scrollbars=yes`,
  );
}

export default function ExternalFramesHost() {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const { items, loading, openedIds, markOpened, closeFrame, reloadKey, reloadFrame } = useCustomMenus();
  const frameRefs = useRef<Record<number, HTMLIFrameElement | null>>({});

  const sendFrameNav = (menu: CustomMenuItem, cmd: "back" | "forward" | "home") => {
    const frame = frameRefs.current[menu.id];
    if (cmd === "home") {
      if (frame) {
        frame.src = menu.url;
      }
      return;
    }
    try {
      frame?.contentWindow?.postMessage({ santralc: "frame-nav", cmd }, "*");
    } catch {
      /* cross-origin frame handles message via extension or Electron preload */
    }
    try {
      window.postMessage(
        { santralc: "panel", type: "frame-nav", menuId: menu.id, url: menu.url, cmd },
        window.location.origin,
      );
    } catch {
      /* ignore */
    }
  };

  const activeId = useMemo(() => parseAppId(pathname), [pathname]);
  const activeMenu = useMemo(
    () => (activeId !== null ? items.find((m) => m.id === activeId) ?? null : null),
    [activeId, items],
  );

  const [checks, setChecks] = useState<Record<number, CustomMenuEmbedCheck>>({});
  const [checking, setChecking] = useState<Record<number, boolean>>({});
  const [forceEmbed, setForceEmbed] = useState<Record<number, boolean>>({});

  useEffect(() => {
    if (activeMenu) {
      markOpened(activeMenu.id);
    }
  }, [activeMenu, markOpened]);

  useEffect(() => {
    if (!activeMenu) return;
    const id = activeMenu.id;
    if (checks[id] !== undefined || checking[id]) return;

    setChecking((prev) => ({ ...prev, [id]: true }));
    api
      .menuCheck(id)
      .then((res) => {
        setChecks((prev) => ({ ...prev, [id]: res }));
      })
      .catch(() => {
        setChecks((prev) => ({ ...prev, [id]: { embeddable: true } }));
      })
      .finally(() => {
        setChecking((prev) => ({ ...prev, [id]: false }));
      });
  }, [activeMenu, checks, checking]);

  const openedMenus = useMemo(
    () =>
      openedIds
        .map((id) => items.find((m) => m.id === id))
        .filter((m): m is CustomMenuItem => Boolean(m)),
    [openedIds, items],
  );

  // If no custom app has ever been opened and user is not on /app/:id, render nothing.
  if (openedMenus.length === 0 && activeId === null) {
    return null;
  }

  return (
    <div
      data-testid="external-frames-host"
      className={cn(
        "flex-1 flex-col px-3 py-3 md:px-5 md:py-4",
        activeId !== null ? "flex min-h-[calc(100svh-4rem)]" : "hidden",
      )}
    >
      {activeId !== null && loading && !activeMenu && (
        <div className="flex flex-1 items-center justify-center">
          <Spinner />
        </div>
      )}

      {activeId !== null && !loading && !activeMenu && (
        <div className="my-auto">
          <Card title="Uygulama Bulunamadı" icon={Globe}>
            <EmptyState
              icon={<Globe />}
              title="Bu menü mevcut değil veya görüntüleme yetkin yok"
              description="Menü kaldırılmış, pasife alınmış ya da rolüne tanımlanmamış olabilir."
              action={
                <Button variant="secondary" onClick={() => navigate("/")}>
                  Ana Sayfaya Dön
                </Button>
              }
            />
          </Card>
        </div>
      )}

      {openedMenus.map((menu) => {
        const isCurrent = activeMenu?.id === menu.id;
        const Icon = customMenuIcon(menu.icon);
        const check = checks[menu.id];
        const hasExtBridge =
          typeof document !== "undefined" &&
          Boolean(document.documentElement.getAttribute("data-santralc-ext"));
        const blockedByHeader = !hasExtBridge && check && !check.embeddable;
        const preferNewTab = menu.openMode === "new_tab" || Boolean(blockedByHeader);
        const showFallback = preferNewTab && !forceEmbed[menu.id];
        const frameInstanceKey = `${menu.id}-${reloadKey[menu.id] ?? 0}`;

        return (
          <div
            key={menu.id}
            data-testid={`external-frame-panel-${menu.id}`}
            aria-hidden={!isCurrent}
            className={cn(
              "flex-1 flex-col overflow-hidden rounded-2xl border border-border/70 bg-card shadow-sm",
              isCurrent ? "flex" : "hidden",
            )}
          >
            {/* Compact Workspace Header */}
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border/60 bg-muted/30 px-3.5 py-2">
              <div className="flex min-w-0 items-center gap-2.5">
                <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/12 text-primary">
                  <Icon className="size-4" />
                </span>
                <span className="truncate text-sm font-semibold">{menu.label}</span>
                <Badge tone="slate">{hostOf(menu.url)}</Badge>
                {!showFallback && (
                  <Badge tone="green">
                    {hasExtBridge ? "Eklenti Köprüsü Aktif" : "Oturum Korunuyor"}
                  </Badge>
                )}
              </div>

              <div className="flex flex-wrap items-center gap-1.5">
                {!showFallback && (
                  <>
                    <div className="mr-1 flex items-center rounded-lg border border-border/60 bg-background/60 p-0.5">
                      <Button
                        variant="ghost"
                        className="h-7 px-2 text-xs"
                        onClick={() => sendFrameNav(menu, "back")}
                        data-tip="Uygulama içinde önceki sayfaya dön"
                      >
                        <ArrowLeft className="size-3.5" />
                        <span className="hidden lg:inline">Geri</span>
                      </Button>
                      <Button
                        variant="ghost"
                        className="h-7 px-2 text-xs"
                        onClick={() => sendFrameNav(menu, "forward")}
                        data-tip="Uygulama içinde sonraki sayfaya ilerle"
                      >
                        <ArrowRight className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        className="h-7 px-2 text-xs"
                        onClick={() => sendFrameNav(menu, "home")}
                        data-tip="Menünün başlangıç adresine dön"
                      >
                        <Home className="size-3.5" />
                      </Button>
                    </div>
                    <Button
                      variant="ghost"
                      className="h-8 px-2.5 text-xs"
                      onClick={() => reloadFrame(menu.id)}
                      data-tip="Sadece bu uygulama penceresini yeniden yükle"
                    >
                      <RefreshCw className="size-3.5" />
                      <span className="hidden sm:inline">Yenile</span>
                    </Button>
                    <Button
                      variant="ghost"
                      className="h-8 px-2.5 text-xs text-muted-foreground hover:text-destructive"
                      onClick={() => {
                        closeFrame(menu.id);
                        navigate("/");
                      }}
                      data-tip="Bu uygulamanın arka plan penceresini kapat"
                    >
                      <PowerOff className="size-3.5" />
                      <span className="hidden md:inline">Kapat</span>
                    </Button>
                  </>
                )}
                <Button
                  variant="secondary"
                  className="h-8 px-2.5 text-xs"
                  onClick={() => openPopupWindow(menu)}
                  data-tip="Harici web sitesini bağımsız uygulama penceresi (Popup) olarak aç"
                >
                  <AppWindow className="size-3.5" />
                  <span className="hidden sm:inline">Popup Aç</span>
                </Button>
                <Button
                  variant="secondary"
                  className="h-8 px-3 text-xs"
                  onClick={() => window.open(menu.url, "_blank", "noopener,noreferrer")}
                  data-tip="Harici web sitesini yeni tarayıcı sekmesinde aç"
                >
                  <ExternalLink className="size-3.5" />
                  <span>Yeni Sekmede Aç</span>
                </Button>
              </div>
            </div>

            {showFallback ? (
              <div className="flex flex-1 flex-col items-center justify-center p-6 text-center">
                <div className="max-w-md space-y-4">
                  <div className="mx-auto flex size-14 items-center justify-center rounded-2xl bg-amber-500/12 text-amber-600 dark:text-amber-400">
                    <ShieldAlert className="size-7" />
                  </div>
                  <div className="space-y-1.5">
                    <h2 className="text-base font-semibold">{menu.label}</h2>
                    <p className="text-xs leading-relaxed text-muted-foreground">
                      {menu.openMode === "new_tab"
                        ? "Bu uygulama yönetici ayarları gereği yeni sekmede açılacak şekilde yapılandırılmış."
                        : check?.reason ||
                          "Bu web sitesi tarayıcı güvenlik politikaları (X-Frame-Options / CSP) nedeniyle uygulama içi pencerede gömülmeye izin vermiyor."}
                    </p>
                  </div>

                  <div className="flex flex-wrap items-center justify-center gap-2.5 pt-1">
                    <Button onClick={() => openPopupWindow(menu)}>
                      <AppWindow />
                      Popup Pencere Olarak Aç
                    </Button>
                    <Button variant="secondary" onClick={() => window.open(menu.url, "_blank", "noopener,noreferrer")}>
                      <ExternalLink />
                      Yeni Sekmede Aç
                    </Button>
                    <Button
                      variant="ghost"
                      onClick={() => setForceEmbed((prev) => ({ ...prev, [menu.id]: true }))}
                    >
                      Yine de Uygulama İçinde Dene
                    </Button>
                  </div>
                </div>
              </div>
            ) : (
              <div className="relative flex flex-1 flex-col bg-background">
                <iframe
                  key={frameInstanceKey}
                  ref={(el) => {
                    frameRefs.current[menu.id] = el;
                  }}
                  data-testid={`external-iframe-${menu.id}`}
                  src={menu.url}
                  title={menu.label}
                  allow="clipboard-read; clipboard-write; microphone; camera; fullscreen; geolocation; autoplay; display-capture"
                  className="h-full min-h-[calc(100svh-8.5rem)] w-full flex-1 border-0 bg-white"
                />
                <div className="flex items-center justify-between gap-2 border-t border-border/50 bg-muted/20 px-3 py-1.5 text-[0.6875rem] text-muted-foreground">
                  <span className="flex items-center gap-1.5 truncate">
                    <Info className="size-3.5 shrink-0 text-muted-foreground/70" />
                    <span>
                      Menüler arasında geçiş yaptığında bu pencere arka planda açık kalır ve oturumun korunur. Sayfa açılmazsa veya çerez (SameSite) kısıtlaması varsa sağ üstten yeni sekmede açabilirsin.
                    </span>
                  </span>
                </div>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

// Placeholder route element for /app/:id; the actual rendering is handled by
// ExternalFramesHost in AppShell so iframes never unmount across navigations.
export function ExternalAppPlaceholder() {
  return null;
}
