// The side menu and the page permissions: who sees which menu entry, and
// that the menu, the page guards in App.tsx, the links between pages and the
// server's permission list all name the same permissions. A menu entry that
// opens a page its viewer cannot see, or a page guarded by a permission the
// server does not know, fails here.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { allows, MENU, menuPermission, PAGE_PERMISSIONS, titleFor, visibleMenu, WA_BASE } from "./menu";

const holds = (...perms: string[]) => (p: string) => perms.includes(p);
const asList = (p: string | string[] | undefined) => (p === undefined ? [] : Array.isArray(p) ? p : [p]);

describe("menu permissions", () => {
  it("allows a page with no permission to everyone", () => {
    expect(allows(holds())).toBe(true);
  });

  it("asks for one permission or any one of a list", () => {
    expect(allows(holds("teams.view"), "teams.view")).toBe(true);
    expect(allows(holds("teams.view"), "user.view")).toBe(false);
    expect(allows(holds("cdr.view_own"), ["cdr.view_all", "cdr.view_own"])).toBe(true);
    expect(allows(holds("user.view"), ["cdr.view_all", "cdr.view_own"])).toBe(false);
    expect(allows(holds("user.view"), [])).toBe(false);
  });

  it("looks up a menu page's permission by its path", () => {
    expect(menuPermission("/users")).toBe("user.view");
    expect(menuPermission("/calls")).toEqual(["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"]);
    expect(menuPermission("/")).toBeUndefined();
    expect(menuPermission("/no-such-page")).toBeUndefined();
  });

  it("shows someone with no permissions only the home page", () => {
    const menu = visibleMenu(holds());
    expect(menu).toHaveLength(1);
    expect(menu[0].items.map((i) => i.path)).toEqual(["/"]);
  });

  it("drops a whole group when nothing in it is allowed", () => {
    const menu = visibleMenu(holds("teams.view", "call.view_own"));
    expect(menu.map((g) => g.title)).toEqual(["Genel"]);
    expect(menu[0].items.map((i) => i.path)).toEqual(["/", "/calls", "/followups", "/teams"]);
  });

  it("shows everything to someone who holds every permission", () => {
    const every = MENU.flatMap((g) => g.items.flatMap((i) => asList(i.permission)));
    const menu = visibleMenu(holds(...every));
    expect(menu.flatMap((g) => g.items)).toHaveLength(MENU.flatMap((g) => g.items).length);
  });

  it("names every page in the header", () => {
    for (const item of MENU.flatMap((g) => g.items)) expect(titleFor(item.path)).toBe(item.label);
    expect(titleFor("/whatsapp/settings")).toBe("WhatsApp Ayarları");
    expect(titleFor("/whatsapp/preferences")).toBe("WhatsApp Ayarlarım");
    expect(titleFor("/account")).toBe("Hesap ve Güvenlik");
    expect(titleFor("/teams/12")).toBe("Teams");
    expect(titleFor("/profile/3")).toBe("Profil");
    expect(titleFor("/somewhere-else")).toBe("Çağrı Yöneticisi");
  });
});

// ---------------------------------------------------------------- consistency

const src = join(dirname(fileURLToPath(import.meta.url)), "..");
const appSource = readFileSync(join(src, "App.tsx"), "utf8");

// The signed-in routes of App.tsx: path and the element it renders.
const routes = [...appSource.matchAll(/<Route path="([^"]+)" element=\{(.*)\} \/>/g)].map((m) => ({ path: m[1], element: m[2] }));
const routeFor = (path: string) => routes.find((r) => r.path === path);

// Pages anyone signed in may open.
const OPEN_PAGES = new Set(["/login", "*", "/", "/profile", "/profile/:id", "/account", "/preferences"]);

// What a route's guard asks for, read back from the names App.tsx uses.
function guardOf(element: string): { need: string[]; base?: string } | null {
  if (!element.includes("<RequirePermission")) return null;
  const fromMenu = /need=\{menuPermission\("([^"]+)"\)\}/.exec(element);
  const fromPage = /need=\{PAGE_PERMISSIONS\.(\w+)\}/.exec(element);
  const need = fromMenu ? asList(menuPermission(fromMenu[1])) : fromPage ? asList(PAGE_PERMISSIONS[fromPage[1] as keyof typeof PAGE_PERMISSIONS]) : [];
  return { need, base: element.includes("base={WA_BASE}") ? WA_BASE : undefined };
}

describe("menu and page guards agree", () => {
  it("reads the routes out of App.tsx", () => {
    expect(routes.length).toBeGreaterThan(15);
  });

  it("guards every menu page with the permission its menu entry asks", () => {
    for (const item of MENU.flatMap((g) => g.items)) {
      const route = routeFor(item.path);
      expect(route, `no route for menu entry ${item.path}`).toBeDefined();
      if (!item.permission) continue;
      const guard = guardOf(route!.element);
      expect(guard, `${item.path} is in the menu behind a permission but its page is not guarded`).not.toBeNull();
      expect(guard!.need, item.path).toEqual(asList(item.permission));
    }
  });

  it("guards a page's inner routes like the page itself", () => {
    for (const r of routes) {
      const parent = "/" + r.path.split("/")[1];
      if (r.path === parent || OPEN_PAGES.has(r.path)) continue;
      const menuNeed = menuPermission(parent);
      // Pages with their own entry below, such as WhatsApp settings, are
      // checked there.
      if (!menuNeed || r.element.includes("PAGE_PERMISSIONS.")) continue;
      expect(guardOf(r.element)?.need, r.path).toEqual(asList(menuNeed));
    }
  });

  it("guards every page outside the menu with its PAGE_PERMISSIONS entry", () => {
    for (const [key, need] of Object.entries(PAGE_PERMISSIONS)) {
      const route = routes.find((r) => r.element.includes(`PAGE_PERMISSIONS.${key}}`));
      expect(route, `PAGE_PERMISSIONS.${key} guards no page`).toBeDefined();
      expect(guardOf(route!.element)!.need).toEqual(asList(need));
      // WhatsApp pages also need the module itself, as the server does.
      if (route!.path.startsWith("/whatsapp/")) expect(guardOf(route!.element)!.base, route!.path).toBe(WA_BASE);
    }
  });

  it("leaves no page unguarded except the ones everyone may open", () => {
    for (const r of routes) {
      if (OPEN_PAGES.has(r.path)) continue;
      expect(guardOf(r.element), `${r.path} has no permission guard`).not.toBeNull();
      expect(guardOf(r.element)!.need.length, `${r.path} asks for nothing`).toBeGreaterThan(0);
    }
  });

  it("shows a link to a guarded page only to someone the page lets in", () => {
    // Lines such as: can(user, "whatsapp.reports") && <Link to="/whatsapp/reports" ...
    const files: string[] = [];
    const walk = (dir: string) => {
      for (const name of readdirSync(dir)) {
        const p = join(dir, name);
        if (statSync(p).isDirectory()) {
          if (name !== "dev") walk(p);
        } else if (p.endsWith(".tsx") && !p.endsWith(".test.tsx")) files.push(p);
      }
    };
    walk(src);
    let checked = 0;
    for (const file of files) {
      for (const line of readFileSync(file, "utf8").split("\n")) {
        const perm = /can\(user, "([^"]+)"\) &&/.exec(line);
        const link = /to="(\/[^"?]*)/.exec(line);
        if (!perm || !link) continue;
        const route = routeFor(link[1]);
        const guard = route && guardOf(route.element);
        if (!guard) continue;
        expect(guard.need, `${file}: link to ${link[1]} is shown for ${perm[1]}`).toContain(perm[1]);
        checked++;
      }
    }
    expect(checked).toBeGreaterThanOrEqual(3);
  });

  it("names only permissions the server knows", () => {
    const catalogue = readFileSync(join(src, "../../backend/pkg/enums/permission.go"), "utf8");
    const known = new Set([...catalogue.matchAll(/Permission = "([^"]+)"/g)].map((m) => m[1]));
    expect(known.size).toBeGreaterThan(30);
    const named = [
      WA_BASE,
      ...MENU.flatMap((g) => g.items.flatMap((i) => asList(i.permission))),
      ...Object.values(PAGE_PERMISSIONS).flatMap(asList),
    ];
    for (const p of named) expect(known, `${p} is not in the server's permission list`).toContain(p);
  });
});
