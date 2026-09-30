import type { ReactNode } from "react";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

/**
 * FormField is the shared labelled-control wrapper used by Console forms. It
 * keeps the label/control/hint structure and spacing in one place instead of
 * repeating an identical local `Field` component in each feature view.
 *
 * `className` overrides the wrapper spacing for the few forms that use a
 * different gap; it is merged with the default so Tailwind conflicts resolve
 * predictably.
 */
export function FormField({
  label,
  htmlFor,
  hint,
  className,
  children,
}: {
  label: string;
  htmlFor: string;
  hint?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cn("space-y-1.5", className)}>
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {hint ? <p className="text-[11px] leading-5 text-fog">{hint}</p> : null}
    </div>
  );
}
