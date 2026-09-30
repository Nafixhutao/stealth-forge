import type { ReactNode } from "react";
import { Label } from "@/components/ui/label";

/**
 * FormField is the shared labelled-control wrapper used by Console forms. It
 * keeps the label/control/hint structure and spacing in one place instead of
 * repeating an identical local `Field` component in each feature view.
 */
export function FormField({
  label,
  htmlFor,
  hint,
  children,
}: {
  label: string;
  htmlFor: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {hint ? <p className="text-[11px] leading-5 text-fog">{hint}</p> : null}
    </div>
  );
}
