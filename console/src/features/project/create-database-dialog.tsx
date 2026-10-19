"use client";

import { useId, useState } from "react";
import { useCreateDatabase } from "@/api/mutations/databases";
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
  PRIMARY_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";

/**
 * Creates a database in the project.
 *
 * A database name is a display name, not an identifier: the API accepts any
 * 2–120 character text, so the field is left as typed rather than forced into a
 * slug. Tables are added afterwards, which is why the dialog says what the
 * database will hold instead of pretending to create one ready to query.
 */
export function CreateDatabaseDialog({
  projectId,
  open,
  onOpenChange,
}: {
  projectId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const create = useCreateDatabase(projectId);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const fieldId = useId();
  const helpId = `${fieldId}-help`;

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setName("");
      setError(null);
    }
    onOpenChange(next);
  };

  const trimmed = name.trim();
  const valid = trimmed.length >= 2 && trimmed.length <= 120;

  const submit = async () => {
    if (!valid) return;
    setError(null);
    try {
      await create.mutateAsync({ name: trimmed });
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not create the database.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Create database</DialogTitle>
          <DialogDescription>
            A database groups tables. Tables, columns, and rows are added to it
            afterwards.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={fieldId}>Database name</Label>
          <Input
            id={fieldId}
            value={name}
            autoFocus
            autoComplete="off"
            aria-describedby={helpId}
            aria-invalid={name.length > 0 && !valid}
            placeholder="app"
            className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
            onChange={(event) => setName(event.target.value)}
          />
          <p
            id={helpId}
            className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]"
          >
            {name.length > 0 && !valid
              ? "Use 2 to 120 characters."
              : "A display name, 2 to 120 characters."}
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
              disabled={!valid || create.isPending}
              className={PRIMARY_ACTION}
            >
              {create.isPending ? "Creating…" : "Create database"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
