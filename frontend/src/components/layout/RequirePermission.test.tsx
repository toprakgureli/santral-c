// The page guard: a page shows only to someone holding the permission it
// needs (and its area's base permission, when it has one); anyone else gets
// the notice with a way back home and never sees the page itself.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { menuPermission, PAGE_PERMISSIONS, WA_BASE } from "@/lib/menu";
import RequirePermission from "./RequirePermission";

const auth = vi.hoisted(() => ({ permissions: [] as string[] }));
vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({ can: (p: string) => auth.permissions.includes(p) }),
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const NOTICE = "Bu sayfayı görme yetkin yok";
let root: Root | null = null;
let box: HTMLDivElement;

// open signs someone in with perms and opens path in a small panel holding
// the home page, the user list and the WhatsApp reports.
function open(path: string, ...perms: string[]) {
  auth.permissions = perms;
  box = document.createElement("div");
  document.body.appendChild(box);
  root = createRoot(box);
  act(() =>
    root!.render(
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/" element={<p>Ana sayfa</p>} />
          <Route path="/users" element={<RequirePermission need={menuPermission("/users")}><p>Kullanıcı listesi</p></RequirePermission>} />
          <Route path="/calls" element={<RequirePermission need={menuPermission("/calls")}><p>Çağrı listesi</p></RequirePermission>} />
          <Route path="/whatsapp/reports" element={<RequirePermission base={WA_BASE} need={PAGE_PERMISSIONS.whatsappReports}><p>WhatsApp raporları</p></RequirePermission>} />
          <Route path="/free" element={<RequirePermission><p>Herkese açık</p></RequirePermission>} />
        </Routes>
      </MemoryRouter>,
    ),
  );
  return box;
}

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  box?.remove();
});

describe("page guard", () => {
  it("shows the page to someone with the permission", () => {
    const page = open("/users", "user.view");
    expect(page.textContent).toContain("Kullanıcı listesi");
    expect(page.textContent).not.toContain(NOTICE);
  });

  it("shows the notice, not the page, to someone without it", () => {
    const page = open("/users", "teams.view", "call.view_own");
    expect(page.textContent).toContain(NOTICE);
    expect(page.textContent).not.toContain("Kullanıcı listesi");
  });

  it("lets in anyone holding one of the listed permissions", () => {
    expect(open("/calls", "call.view_own").textContent).toContain("Çağrı listesi");
  });

  it("asks for the area's base permission as well", () => {
    expect(open("/whatsapp/reports", "whatsapp.reports").textContent).toContain(NOTICE);
    act(() => root?.unmount());
    box.remove();
    expect(open("/whatsapp/reports", "whatsapp.view").textContent).toContain(NOTICE);
    act(() => root?.unmount());
    box.remove();
    expect(open("/whatsapp/reports", "whatsapp.view", "whatsapp.reports").textContent).toContain("WhatsApp raporları");
  });

  it("shows a page that needs nothing to everyone", () => {
    expect(open("/free").textContent).toContain("Herkese açık");
  });

  it("takes someone without the permission back home", () => {
    const page = open("/users");
    const back = [...page.querySelectorAll("a")].find((a) => a.textContent === "Ana sayfaya dön");
    expect(back).toBeDefined();
    act(() => back!.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 })));
    expect(page.textContent).toContain("Ana sayfa");
    expect(page.textContent).not.toContain(NOTICE);
  });
});
