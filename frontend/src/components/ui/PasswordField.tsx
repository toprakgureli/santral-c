import { forwardRef, useState, type InputHTMLAttributes } from "react";
import { Check, Eye, EyeOff, Wand2 } from "lucide-react";
import { Input } from "@/components/ui";
import { PASSWORD_MAX, checkPassword, generatePassword } from "@/lib/password";
import { cn } from "@/lib/utils";

type PasswordFieldProps = Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "value" | "onChange"> & {
  value: string;
  onChange: (value: string) => void;
  // generator shows the wand button; a generated password is revealed so it
  // can be copied or handed over.
  generator?: boolean;
};

// PasswordField is a password input with show/hide and an optional generator.
const PasswordField = forwardRef<HTMLInputElement, PasswordFieldProps>(function PasswordField(
  { value, onChange, generator, className, ...props },
  ref,
) {
  const [show, setShow] = useState(false);
  const padding = generator ? "pr-[4.5rem]" : "pr-10";
  return (
    <div className="relative">
      <Input
        ref={ref}
        type={show ? "text" : "password"}
        maxLength={PASSWORD_MAX}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={cn(padding, className)}
        {...props}
      />
      <div className="absolute top-1/2 right-1.5 flex -translate-y-1/2 items-center">
        {generator && (
          <button
            type="button"
            tabIndex={-1}
            title="Kurallara uygun şifre oluştur"
            aria-label="Şifre oluştur"
            onClick={() => {
              onChange(generatePassword());
              setShow(true);
            }}
            className="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:text-foreground"
          >
            <Wand2 className="size-4" />
          </button>
        )}
        <button
          type="button"
          tabIndex={-1}
          onClick={() => setShow((v) => !v)}
          aria-label={show ? "Şifreyi gizle" : "Şifreyi göster"}
          className="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:text-foreground"
        >
          {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
        </button>
      </div>
    </div>
  );
});

export default PasswordField;

// PasswordRules is the policy checklist; each rule ticks green once the value
// satisfies it.
export function PasswordRules({ value, className }: { value: string; className?: string }) {
  return (
    <ul className={cn("grid grid-cols-2 gap-x-3 gap-y-1 sm:grid-cols-3", className)} aria-label="Şifre kuralları">
      {checkPassword(value).map((rule) => (
        <li key={rule.key} className={cn("flex items-center gap-1.5 text-xs transition-colors", rule.ok ? "text-success" : "text-muted-foreground")}>
          <span
            className={cn(
              "flex size-4 shrink-0 items-center justify-center rounded border transition-colors",
              rule.ok ? "border-success bg-success text-white" : "border-border/80 bg-transparent",
            )}
          >
            {rule.ok && <Check className="size-3" strokeWidth={3} />}
          </span>
          {rule.label}
        </li>
      ))}
    </ul>
  );
}
