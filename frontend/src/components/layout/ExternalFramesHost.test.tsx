import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CustomMenuEmbedCheck, CustomMenuItem } from "@/api/types";
import { CustomMenusProvider } from "@/menus/CustomMenusContext";
import ExternalFramesHost from "./ExternalFramesHost";

const mockMenus: CustomMenuItem[] = [
  {
    id: 1,
    label: "Telegram",
    icon: "telegram",
    url: "https://web.telegram.org/k/",
    openMode: "iframe",
    active: true,
    sortOrder: 0,
    roleIds: [],
    roles: [],
  },
  {
    id: 2,
    label: "CRM Sistemi",
    icon: "briefcase",
    url: "https://crm.example.com/app",
    openMode: "iframe",
    active: true,
    sortOrder: 1,
    roleIds: [],
    roles: [],
  },
  {
    id: 3,
    label: "Kısıtlı Site",
    icon: "shield",
    url: "https://blocked.example.com",
    openMode: "iframe",
    active: true,
    sortOrder: 2,
    roleIds: [],
    roles: [],
  },
];

const mockChecks: Record<number, CustomMenuEmbedCheck> = {
  1: { embeddable: true },
  2: { embeddable: true },
  3: {
    embeddable: false,
    reason: "Hedef web sitesi (X-Frame-Options: DENY) nedeniyle iframe içinde açılmaya izin vermiyor.",
    xFrameOptions: "DENY",
  },
};

vi.mock("@/api/client", () => ({
  api: {
    menusVisible: vi.fn(() => Promise.resolve(mockMenus)),
    menuCheck: vi.fn((id: number) => Promise.resolve(mockChecks[id] ?? { embeddable: true })),
  },
}));

let navigateFn: (to: string) => void = () => {};

function NavBridge() {
  const nav = useNavigate();
  navigateFn = (to: string) => nav(to);
  return null;
}

describe("ExternalFramesHost persistent iframes", () => {
  let container: HTMLDivElement | null = null;
  let root: Root | null = null;

  afterEach(() => {
    if (root) {
      act(() => root!.unmount());
      root = null;
    }
    if (container) {
      container.remove();
      container = null;
    }
  });

  async function mountAt(initialPath: string) {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    await act(async () => {
      root!.render(
        <MemoryRouter initialEntries={[initialPath]}>
          <CustomMenusProvider>
            <NavBridge />
            <ExternalFramesHost />
          </CustomMenusProvider>
        </MemoryRouter>,
      );
    });
    await act(async () => {
      await Promise.resolve();
    });
  }

  it("keeps opened iframes mounted across menu navigation to preserve session state", async () => {
    await mountAt("/app/1");

    const telegramIframe = container!.querySelector('[data-testid="external-iframe-1"]') as HTMLIFrameElement | null;
    expect(telegramIframe).not.toBeNull();
    expect(telegramIframe!.getAttribute("src")).toBe("https://web.telegram.org/k/");

    // Switch to an internal Santral route (/calls): iframe must stay in the DOM
    await act(async () => {
      navigateFn("/calls");
    });
    const hostWhenOnCalls = container!.querySelector('[data-testid="external-frames-host"]') as HTMLElement;
    expect(hostWhenOnCalls.className).toContain("hidden");
    expect(container!.querySelector('[data-testid="external-iframe-1"]')).toBe(telegramIframe);

    // Switch to a second external app (/app/2): both iframes stay mounted independently
    await act(async () => {
      navigateFn("/app/2");
    });
    await act(async () => {
      await Promise.resolve();
    });

    const crmIframe = container!.querySelector('[data-testid="external-iframe-2"]') as HTMLIFrameElement | null;
    expect(crmIframe).not.toBeNull();
    expect(crmIframe!.getAttribute("src")).toBe("https://crm.example.com/app");
    // The exact same Telegram iframe DOM node is still mounted
    expect(container!.querySelector('[data-testid="external-iframe-1"]')).toBe(telegramIframe);

    // Navigate back to Telegram (/app/1): exact same DOM instance, no reload
    await act(async () => {
      navigateFn("/app/1");
    });
    expect(container!.querySelector('[data-testid="external-iframe-1"]')).toBe(telegramIframe);
    const panel1 = container!.querySelector('[data-testid="external-frame-panel-1"]') as HTMLElement;
    const panel2 = container!.querySelector('[data-testid="external-frame-panel-2"]') as HTMLElement;
    expect(panel1.className).toContain("flex");
    expect(panel2.className).toContain("hidden");
  });

  it("shows fallback card when site blocks iframe embedding via X-Frame-Options", async () => {
    await mountAt("/app/3");

    expect(container!.textContent).toContain("X-Frame-Options: DENY");
    expect(container!.textContent).toContain("Yeni Sekmede Aç");
    expect(container!.querySelector('[data-testid="external-iframe-3"]')).toBeNull();
  });
});
