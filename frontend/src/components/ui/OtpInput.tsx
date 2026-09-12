import { forwardRef, useImperativeHandle, useRef, type ClipboardEvent, type KeyboardEvent } from "react";
import { cn } from "@/lib/utils";

const LENGTH = 6;

export type OtpHandle = {
  focus: () => void;
};

type OtpInputProps = {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  id?: string;
};

// OtpInput is a six-box one-time-code field. Each box stacks a fill layer, a
// one-shot ring, the visible digit and a transparent-text input on top, so the
// digit can pop in while the real input keeps focus, selection and paste
// behaviour. Typing auto-advances, paste fills from the first box, Backspace
// clears in place or steps back, and arrows move between boxes.
const OtpInput = forwardRef<OtpHandle, OtpInputProps>(function OtpInput({ value, onChange, disabled, id }, ref) {
  const refs = useRef<(HTMLInputElement | null)[]>([]);

  useImperativeHandle(ref, () => ({ focus: () => refs.current[0]?.focus() }), []);

  const digits = value.padEnd(LENGTH, " ").slice(0, LENGTH).split("");
  const complete = value.replace(/\s/g, "").length === LENGTH;

  const focusAt = (index: number) => {
    refs.current[Math.max(0, Math.min(LENGTH - 1, index))]?.focus();
  };

  const write = (index: number, typed: string) => {
    const clean = typed.replace(/\D/g, "");
    if (!clean) return;
    const next = value.split("");
    for (let i = 0; i < clean.length && index + i < LENGTH; i++) {
      next[index + i] = clean[i];
    }
    onChange(next.join("").replace(/\s/g, "").slice(0, LENGTH));
    focusAt(index + clean.length);
  };

  const onKeyDown = (index: number) => (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Backspace") {
      e.preventDefault();
      const next = value.split("");
      if (next[index]) {
        next[index] = "";
        onChange(next.join(""));
        return;
      }
      next[index - 1] = "";
      onChange(next.join(""));
      focusAt(index - 1);
      return;
    }
    if (e.key === "ArrowLeft") {
      e.preventDefault();
      focusAt(index - 1);
    }
    if (e.key === "ArrowRight") {
      e.preventDefault();
      focusAt(index + 1);
    }
  };

  const onPaste = (e: ClipboardEvent<HTMLInputElement>) => {
    e.preventDefault();
    write(0, e.clipboardData.getData("text"));
  };

  return (
    <div className="flex justify-center gap-2" role="group" aria-label="Doğrulama kodu">
      {Array.from({ length: LENGTH }, (_, i) => {
        const digit = digits[i].trim();
        const filled = digit !== "";
        return (
          <div key={i} className="relative size-12 sm:size-13">
            <span
              aria-hidden="true"
              className={cn(
                "absolute inset-0 origin-bottom rounded-xl bg-primary/15",
                "transition-transform duration-300 ease-[cubic-bezier(0.34,1.4,0.64,1)]",
                filled ? "scale-y-100" : "scale-y-0",
              )}
            />
            {filled && (
              <span
                key={`ring-${digit}`}
                aria-hidden="true"
                className="otp-ring pointer-events-none absolute inset-0 rounded-xl border-2 border-primary/70"
              />
            )}
            {filled && (
              <span
                key={`digit-${digit}`}
                aria-hidden="true"
                className="otp-digit pointer-events-none absolute inset-0 flex items-center justify-center font-mono text-lg text-foreground"
              >
                {digit}
              </span>
            )}
            <input
              id={i === 0 ? id : undefined}
              ref={(el) => {
                refs.current[i] = el;
              }}
              type="text"
              inputMode="numeric"
              autoComplete={i === 0 ? "one-time-code" : "off"}
              maxLength={1}
              disabled={disabled}
              value={digit}
              aria-label={`${i + 1}. hane`}
              onChange={(e) => write(i, e.target.value)}
              onKeyDown={onKeyDown(i)}
              onPaste={onPaste}
              onFocus={(e) => e.target.select()}
              className={cn(
                "absolute inset-0 rounded-xl border bg-transparent caret-primary selection:bg-transparent",
                "text-center font-mono text-lg text-transparent",
                "outline-none transition-[border-color,box-shadow] duration-200",
                "focus-visible:border-ring focus-visible:ring-4 focus-visible:ring-ring/25",
                "disabled:opacity-100",
                complete ? "border-primary/60" : filled ? "border-primary/40" : "border-border/70",
              )}
            />
          </div>
        );
      })}
    </div>
  );
});

export default OtpInput;
