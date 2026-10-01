// A page's code is fetched the first time the page opens. After a new
// version of the panel is published, a panel that has been open since
// before it may ask for a file that is no longer there.

const CHUNK_ERROR = /dynamically imported module|Importing a module script failed|error loading dynamically imported module|Unable to preload CSS/i;

// isUpdateError tells a missing page file apart from a real crash.
export function isUpdateError(error: unknown): boolean {
  return error instanceof Error && CHUNK_ERROR.test(error.message);
}

const RELOADED_KEY = "santral.updateReload";

// reloadOnce reloads the page for a new version, but not again within a
// minute: if the file is still missing after a reload, the error shows
// instead of the page reloading for ever.
export function reloadOnce(): boolean {
  try {
    const last = Number(window.sessionStorage.getItem(RELOADED_KEY) ?? 0);
    if (Date.now() - last < 60_000) return false;
    window.sessionStorage.setItem(RELOADED_KEY, String(Date.now()));
  } catch {
    // no storage: reload anyway
  }
  window.location.reload();
  return true;
}
