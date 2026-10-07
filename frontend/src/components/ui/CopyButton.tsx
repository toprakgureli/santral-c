import { useEffect, useState } from "react";
import { Check, Copy } from "lucide-react";
import { Button } from "@/components/ui";
import { cn } from "@/lib/utils";

// copyText puts text on the clipboard and reports whether it worked.
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Older browsers or a non-secure context: fall back to a hidden textarea.
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    ta.remove();
    return ok;
  }
}

// CopyButton copies text and briefly confirms it.
export default function CopyButton({ text, label, variant = "secondary", className }: { text: string; label: string; variant?: "primary" | "secondary"; className?: string }) {
  const [done, setDone] = useState(false);
  useEffect(() => {
    if (!done) return;
    const t = window.setTimeout(() => setDone(false), 2000);
    return () => window.clearTimeout(t);
  }, [done]);
  return (
    <Button
      variant={variant}
      className={cn(className, done && "bg-success text-white hover:bg-success")}
      onClick={async () => {
        if (await copyText(text)) setDone(true);
      }}
    >
      {done ? <Check /> : <Copy />}
      {done ? "Kopyalandı" : label}
    </Button>
  );
}
