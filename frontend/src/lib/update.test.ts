import { afterEach, describe, expect, it, vi } from "vitest";
import { isUpdateError, reloadOnce } from "./update";

describe("new panel version", () => {
  afterEach(() => {
    window.sessionStorage.clear();
    vi.restoreAllMocks();
  });

  it("tells a missing page file from a real crash", () => {
    expect(isUpdateError(new TypeError("Failed to fetch dynamically imported module: https://cm.example/assets/Calls-abc.js"))).toBe(true);
    expect(isUpdateError(new TypeError("Importing a module script failed."))).toBe(true);
    expect(isUpdateError(new Error("Cannot read properties of undefined (reading 'map')"))).toBe(false);
    expect(isUpdateError("not an error")).toBe(false);
  });

  it("reloads once, not in a loop", () => {
    const reload = vi.fn();
    vi.spyOn(window, "location", "get").mockReturnValue({ ...window.location, reload } as Location);
    expect(reloadOnce()).toBe(true);
    expect(reloadOnce()).toBe(false);
    expect(reload).toHaveBeenCalledTimes(1);
  });
});
