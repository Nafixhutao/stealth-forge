"use client";

import { useId, useState } from "react";
import { useCreateManagedDatabase } from "@/api/mutations/managed-databases";
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
 * Creates the project's free MySQL database.
 *
 * The name is a display name; the API derives the MySQL database and user names
 * so two projects can never collide. The free tier is one database per project,
 * and the dialog says so before the operator types anything.
 */
export function CreateManagedDatabaseDialog({
  projectId,
  open,
  onOpenChange,
}: {
  projectId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const create = useCreateManagedDatabase(projectId);
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
          <DialogTitle>Create MySQL database</DialogTitle>
          <DialogDescription>
            The free tier includes one MySQL database per project. Choose a
            display name; the database and user names are generated.
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
            placeholder="toko"
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
              className={SECONDARY_ACTION}
              onClick={() => handleOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              className={PRIMARY_ACTION}
              disabled={!valid || create.isPending}
            >
              {create.isPending ? "Creating…" : "Create database"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
