import type { ReactNode } from "react";
import { Moon, Sun } from "lucide-react";
import Logo from "@/components/ui/Logo";
import { useTheme } from "@/contexts/ThemeContext";
import { COMPANY } from "@/lib/brand";

type AuthLayoutProps = {
  title: string;
  description: string;
  children: ReactNode;
};

export default function AuthLayout({ title, description, children }: AuthLayoutProps) {
  const { isDark, toggle } = useTheme();

  return (
    <div className="auth-backdrop relative flex min-h-svh flex-col items-center justify-center gap-8 p-6 md:p-10">
      <button
        type="button"
        onClick={toggle}
        aria-label={isDark ? "Aydınlık temaya geç" : "Koyu temaya geç"}
        className="absolute top-5 right-5 flex size-10 items-center justify-center rounded-xl text-muted-foreground hover:bg-accent"
      >
        {isDark ? <Sun className="size-4" /> : <Moon className="size-4" />}
      </button>

      <div className="animate-in fade-in slide-in-from-bottom-3 w-full max-w-[26rem] duration-500 ease-out">
        <div className="mb-7 flex flex-col items-center gap-3 text-center">
          <Logo className="size-16" wordmark wordmarkClassName="text-2xl" />
          <div className="space-y-1.5">
            <h1 className="text-[1.375rem] leading-tight font-semibold tracking-tight">{title}</h1>
            <p className="mx-auto max-w-[22rem] text-sm leading-relaxed text-muted-foreground">{description}</p>
          </div>
        </div>

        <div className="rounded-3xl bg-card/80 p-7 shadow-xl shadow-black/[0.04] ring-1 ring-border/60 backdrop-blur-sm dark:shadow-black/20">
          {children}
        </div>
      </div>

      <div className="animate-in fade-in flex max-w-[26rem] flex-col items-center gap-2 delay-200 duration-700 fill-mode-both">
        <span className="h-px w-16 bg-gradient-to-r from-transparent via-border to-transparent" />
        <p className="text-center text-[0.625rem] leading-relaxed font-medium tracking-[0.14em] text-muted-foreground/60 uppercase">
          {COMPANY}
        </p>
        <p className="text-center text-[0.625rem] tracking-wide text-muted-foreground/40">
          © {new Date().getFullYear()} · Tüm hakları saklıdır
        </p>
      </div>
    </div>
  );
}
