import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

// Every theme is a full token set in index.css, selected by data-theme on
// <html>. Dark-kind themes also carry the .dark class so dark: utilities
// keep working. `swatch` feeds the picker: background, card, primary.
export type Theme = "light" | "dark" | "midnight" | "forest" | "sand" | "crimson" | "matte";

export interface ThemeInfo {
  id: Theme;
  label: string;
  hint: string;
  kind: "light" | "dark";
  swatch: [string, string, string];
}

export const THEMES: ThemeInfo[] = [
  { id: "light", label: "Açık", hint: "Beyaz kartlar, nötr gri", kind: "light", swatch: ["#f8f9fb", "#ffffff", "#2a2e3a"] },
  { id: "dark", label: "Koyu", hint: "Grafit, varsayılan", kind: "dark", swatch: ["#1e2128", "#292d35", "#e6e7ea"] },
  { id: "midnight", label: "Gece Mavisi", hint: "Lacivert zemin, mavi vurgu", kind: "dark", swatch: ["#151a2b", "#1d2337", "#5b8cff"] },
  { id: "forest", label: "Orman", hint: "Yeşile çalan koyu ton", kind: "dark", swatch: ["#171f1b", "#1f2924", "#4fd1a1"] },
  { id: "sand", label: "Kum", hint: "Sıcak krem, kahve vurgu", kind: "light", swatch: ["#f6f1e8", "#fdfaf4", "#6b4f2a"] },
  { id: "crimson", label: "Bordo", hint: "Koyu kızıl zemin, kırmızı vurgu", kind: "dark", swatch: ["#1c1114", "#26171b", "#e0475a"] },
  { id: "matte", label: "Mat Siyah", hint: "Saf siyah, en koyu", kind: "dark", swatch: ["#000000", "#111111", "#f2f2f2"] },
];

const IDS = new Set<string>(THEMES.map((t) => t.id));

type ThemeValue = {
  theme: Theme;
  info: ThemeInfo;
  themes: ThemeInfo[];
  setTheme: (theme: Theme) => void;
  toggle: () => void;
  isDark: boolean;
};

const STORAGE_KEY = "santral.theme";
const ThemeContext = createContext<ThemeValue | null>(null);

function readTheme(): Theme {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    return stored && IDS.has(stored) ? (stored as Theme) : "dark";
  } catch {
    return "dark";
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readTheme);
  const switchTimer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearTimeout(switchTimer.current), []);

  const info = useMemo(() => THEMES.find((t) => t.id === theme) ?? THEMES[1], [theme]);

  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = theme;
    root.classList.toggle("dark", info.kind === "dark");
    try {
      window.localStorage.setItem(STORAGE_KEY, theme);
    } catch {
      return;
    }
  }, [theme, info.kind]);

  // Colours cross-fade for a moment on every switch instead of snapping.
  const setTheme = useCallback((next: Theme) => {
    const root = document.documentElement;
    root.classList.add("theme-switching");
    window.clearTimeout(switchTimer.current);
    switchTimer.current = window.setTimeout(() => {
      root.classList.remove("theme-switching");
    }, 320);
    setThemeState(next);
  }, []);

  // toggle flips between the plain light and dark themes (keyboard-friendly
  // shortcut kept for the auth pages).
  const toggle = useCallback(() => {
    setTheme(info.kind === "dark" ? "light" : "dark");
  }, [info.kind, setTheme]);

  const value = useMemo<ThemeValue>(
    () => ({ theme, info, themes: THEMES, setTheme, toggle, isDark: info.kind === "dark" }),
    [theme, info, setTheme, toggle],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used within ThemeProvider");
  return ctx;
}
