"use client";

import { useId, useState } from "react";
import { useCreateDatabaseTable } from "@/api/mutations/databases";
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
 * Creates a table in the open database.
 *
 * A table is only a name and its access policy. Columns are added afterwards
 * through the API, and the API defaults every permission array to empty, so the
 * dialog says that rather than implying the table is immediately readable by a
 * customer application.
 */
export function CreateTableDialog({
  projectId,
  databaseId,
  open,
  onOpenChange,
}: {
  projectId: string;
  databaseId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const create = useCreateDatabaseTable(projectId, databaseId);
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
      await create.mutateAsync({ name: trimmed, row_security: true });
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Could not create the table.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Create table</DialogTitle>
          <DialogDescription>
            Tables hold typed rows. Application access starts closed until the
            table&apos;s permission arrays are set through the API.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={fieldId}>Table name</Label>
          <Input
            id={fieldId}
            value={name}
            autoFocus
            autoComplete="off"
            aria-describedby={helpId}
            aria-invalid={name.length > 0 && !valid}
            placeholder="orders"
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
              {create.isPending ? "Creating…" : "Create table"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
