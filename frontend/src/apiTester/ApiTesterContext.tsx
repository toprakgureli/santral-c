import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { apiTesterApi } from "./api";

type ApiTesterContextValue = {
  enabled: boolean;
  loading: boolean;
  refresh: () => Promise<void>;
  setEnabledLocal: (val: boolean) => void;
};

const ApiTesterContext = createContext<ApiTesterContextValue | null>(null);

const FALLBACK: ApiTesterContextValue = {
  enabled: true,
  loading: false,
  refresh: async () => {},
  setEnabledLocal: () => {},
};

export function ApiTesterProvider({ children }: { children: ReactNode }) {
  const [enabled, setEnabled] = useState(true);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const res = await apiTesterApi.status();
      setEnabled(Boolean(res.enabled));
    } catch {
      // Keep current value on transient error
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const setEnabledLocal = useCallback((val: boolean) => {
    setEnabled(val);
  }, []);

  const value = useMemo<ApiTesterContextValue>(
    () => ({ enabled, loading, refresh, setEnabledLocal }),
    [enabled, loading, refresh, setEnabledLocal],
  );

  return <ApiTesterContext.Provider value={value}>{children}</ApiTesterContext.Provider>;
}

export function useApiTesterFeature(): ApiTesterContextValue {
  return useContext(ApiTesterContext) ?? FALLBACK;
}
