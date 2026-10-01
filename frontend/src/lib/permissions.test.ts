import { describe, expect, it } from "vitest";
import type { User } from "../api/types";
import { can, canAny } from "./permissions";

const userWith = (...permissions: string[]) => ({ id: 1, name: "Ayşe", permissions }) as unknown as User;

describe("permission checks", () => {
  it("lets nobody through when no one is signed in", () => {
    expect(can(null, "teams.view")).toBe(false);
    expect(canAny(null, ["teams.view", "user.view"])).toBe(false);
  });

  it("asks for the exact permission", () => {
    const u = userWith("teams.view", "cdr.view_own");
    expect(can(u, "teams.view")).toBe(true);
    expect(can(u, "cdr.view_own")).toBe(true);
    expect(can(u, "cdr.view_all")).toBe(false);
    // No prefix or module match: "teams.view" does not grant "teams".
    expect(can(u, "teams")).toBe(false);
    expect(can(u, "teams.view_all")).toBe(false);
  });

  it("lets through anyone who holds one of the listed permissions", () => {
    const u = userWith("call.view_own");
    expect(canAny(u, ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"])).toBe(true);
    expect(canAny(u, ["cdr.view_all", "user.view"])).toBe(false);
  });

  it("lets nobody through an empty list", () => {
    expect(canAny(userWith("teams.view"), [])).toBe(false);
  });

  it("lets nobody through with a role that has no permissions", () => {
    const u = userWith();
    expect(can(u, "teams.view")).toBe(false);
    expect(canAny(u, ["teams.view"])).toBe(false);
  });
});
