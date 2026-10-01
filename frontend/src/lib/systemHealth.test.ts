import { afterEach, describe, expect, it } from "vitest";
import type { SystemWarning } from "@/api/client";
import type { User } from "@/api/types";
import { setStorageUser } from "./userStorage";
import { keepCurrent, linkFor, openWarnings, readDismissed, writeDismissed } from "./systemHealth";

function warning(key: string, level: SystemWarning["level"], fingerprint: string): SystemWarning {
  return { key, level, title: key, text: "", action: "", fingerprint };
}

describe("system warnings", () => {
  afterEach(() => {
    window.localStorage.clear();
    setStorageUser(0);
  });

  it("shows the urgent ones first and hides the closed ones", () => {
    const list = [warning("inbound", "warning", "inbound:1"), warning("redis", "critical", "redis:critical"), warning("disk", "warning", "disk:warning:85")];
    const open = openWarnings(list, new Set(["disk:warning:85"]));
    expect(open.map((w) => w.key)).toEqual(["redis", "inbound"]);
  });

  it("shows a closed warning again once it changes", () => {
    const closed = new Set(["disk:warning:85"]);
    expect(openWarnings([warning("disk", "warning", "disk:warning:90")], closed)).toHaveLength(1);
  });

  it("forgets closed warnings the server no longer reports", () => {
    const closed = new Set(["disk:warning:85", "webhook:1700000000"]);
    const kept = keepCurrent(closed, [warning("webhook", "warning", "webhook:1700000000")]);
    expect([...kept]).toEqual(["webhook:1700000000"]);
  });

  it("keeps closed warnings per person", () => {
    setStorageUser(7);
    writeDismissed(new Set(["redis:critical"]));
    expect(readDismissed().has("redis:critical")).toBe(true);
    setStorageUser(8);
    expect(readDismissed().size).toBe(0);
  });

  it("links only to pages the person may open", () => {
    const w = { ...warning("webhook", "warning", "webhook:1"), link: "/whatsapp/settings?tab=events" };
    const manager = { permissions: ["system.health", "whatsapp.view"] } as User;
    const owner = { permissions: ["system.health", "whatsapp.channel_manage"] } as User;
    expect(linkFor(manager, w)).toBeUndefined();
    expect(linkFor(owner, w)).toBe("/whatsapp/settings?tab=events");
    expect(linkFor(owner, warning("disk", "warning", "disk:warning:85"))).toBeUndefined();
  });
});
