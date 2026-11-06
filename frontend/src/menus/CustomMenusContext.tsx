import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { api } from "@/api/client";
import type { CustomMenuItem } from "@/api/types";

type CustomMenusContextValue = {
  items: CustomMenuItem[];
  loading: boolean;
  openedIds: number[];
  markOpened: (id: number) => void;
  closeFrame: (id: number) => void;
  reloadKey: Record<number, number>;
  reloadFrame: (id: number) => void;
  refresh: () => Promise<void>;
};

const CustomMenusContext = createContext<CustomMenusContextValue | null>(null);

const EMPTY: CustomMenusContextValue = {
  items: [],
  loading: false,
  openedIds: [],
  markOpened: () => {},
  closeFrame: () => {},
  reloadKey: {},
  reloadFrame: () => {},
  refresh: async () => {},
};

export function CustomMenusProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<CustomMenuItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [openedIds, setOpenedIds] = useState<number[]>([]);
  const [reloadKey, setReloadKey] = useState<Record<number, number>>({});

  const refresh = useCallback(async () => {
    try {
      const list = await api.menusVisible();
      setItems(list);
      // Drop any opened iframe whose menu was removed or deactivated.
      const validIds = new Set(list.map((m) => m.id));
      setOpenedIds((prev) => prev.filter((id) => validIds.has(id)));
    } catch {
      // Keep existing list on transient network error
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const markOpened = useCallback((id: number) => {
    setOpenedIds((prev) => (prev.includes(id) ? prev : [...prev, id]));
  }, []);

  const closeFrame = useCallback((id: number) => {
    setOpenedIds((prev) => prev.filter((x) => x !== id));
  }, []);

  const reloadFrame = useCallback((id: number) => {
    setReloadKey((prev) => ({ ...prev, [id]: (prev[id] ?? 0) + 1 }));
  }, []);

  const value = useMemo<CustomMenusContextValue>(
    () => ({
      items,
      loading,
      openedIds,
      markOpened,
      closeFrame,
      reloadKey,
      reloadFrame,
      refresh,
    }),
    [items, loading, openedIds, markOpened, closeFrame, reloadKey, reloadFrame, refresh],
  );

  return <CustomMenusContext.Provider value={value}>{children}</CustomMenusContext.Provider>;
}

export function useCustomMenus(): CustomMenusContextValue {
  return useContext(CustomMenusContext) ?? EMPTY;
}
