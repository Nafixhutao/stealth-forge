"use client";

import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  DANGER_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";

/**
 * The confirmation for deleting a resource that cannot be restored.
 *
 * Every delete in the project surface goes through this: a database takes its
 * tables and rows with it, a bucket takes its files, and a function takes its
 * deployments. A single "Delete" button is too easy to hit by accident on a
 * list where the row's neighbours look the same, so the operator names the
 * resource first and the confirm control stays disabled until the name matches
 * exactly. That is the cheapest way to make an irreversible action deliberate.
 */
export function DeleteResourceDialog({
  open,
  onOpenChange,
  resourceLabel,
  name,
  consequence,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** What is being deleted, as a singular noun: "database", "bucket", "function". */
  resourceLabel: string;
  /** The resource's name, which the operator has to type back. */
  name: string;
  /** What goes with it, in one sentence. */
  consequence: string;
  onConfirm: () => Promise<void>;
}) {
  const [typed, setTyped] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const fieldId = useId();
  const helpId = `${fieldId}-help`;

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setTyped("");
      setError(null);
      setPending(false);
    }
    onOpenChange(next);
  };

  const matches = typed === name;

  const submit = async () => {
    if (!matches || pending) return;
    setError(null);
    setPending(true);
    try {
      await onConfirm();
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : `Could not delete the ${resourceLabel}.`,
      );
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Delete {resourceLabel}</DialogTitle>
          <DialogDescription>{consequence}</DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={fieldId}>
            Type{" "}
            <span className="font-mono text-[var(--projects-text)]">{name}</span>{" "}
            to confirm
          </Label>
          <Input
            id={fieldId}
            value={typed}
            autoFocus
            autoComplete="off"
            spellCheck={false}
            aria-describedby={helpId}
            aria-invalid={typed.length > 0 && !matches}
            placeholder={name}
            className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] font-mono text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--projects-danger)]/70 focus-visible:ring-[var(--projects-danger)]/15"
            onChange={(event) => setTyped(event.target.value)}
          />
          <p
            id={helpId}
            className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]"
          >
            {typed.length > 0 && !matches
              ? "The name does not match yet."
              : "This cannot be undone."}
          </p>

          {error ? (
            <p
              role="alert"
              className="m-0 text-[12px] leading-4 text-[var(--projects-danger)]"
            >
              {error}
            </p>
          ) : null}

          <DialogFooter className="mt-2">
            <Button
              type="button"
              variant="secondary"
              onClick={() => handleOpenChange(false)}
              className={SECONDARY_ACTION}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!matches || pending}
              className={DANGER_ACTION}
            >
              {pending ? "Deleting…" : `Delete ${resourceLabel}`}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
