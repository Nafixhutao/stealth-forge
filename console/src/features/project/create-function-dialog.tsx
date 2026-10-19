"use client";

import { useId, useState } from "react";
import { FunctionRuntime } from "@/api/generated/schema";
import { useCreateFunction } from "@/api/mutations/functions";
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
import { cn, isSlug, toSlug } from "@/lib/utils";

/** The runtimes the API accepts, with the entrypoint each one conventionally uses. */
const RUNTIMES: { value: FunctionRuntime; label: string; entrypoint: string }[] = [
  { value: FunctionRuntime.node_22, label: "Node.js 22", entrypoint: "src/main.js" },
  { value: FunctionRuntime.python_3_13, label: "Python 3.13", entrypoint: "main.py" },
  { value: FunctionRuntime.go_1_24, label: "Go 1.24", entrypoint: "main.go" },
];

/**
 * Creates a function in the project.
 *
 * This writes metadata only: the function serves nothing until a deployment is
 * uploaded, and the console has no function upload yet, so the dialog says that
 * instead of implying the function is ready to run. The entrypoint follows the
 * selected runtime while it is untouched, and stops following it as soon as the
 * operator edits it.
 */
export function CreateFunctionDialog({
  projectId,
  open,
  onOpenChange,
}: {
  projectId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const create = useCreateFunction(projectId);
  const [name, setName] = useState("");
  const [runtime, setRuntime] = useState<FunctionRuntime>(RUNTIMES[0].value);
  const [entrypoint, setEntrypoint] = useState(RUNTIMES[0].entrypoint);
  const [entrypointEdited, setEntrypointEdited] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fieldId = useId();
  const helpId = `${fieldId}-help`;
  const runtimeId = `${fieldId}-runtime`;
  const entrypointId = `${fieldId}-entrypoint`;

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setName("");
      setRuntime(RUNTIMES[0].value);
      setEntrypoint(RUNTIMES[0].entrypoint);
      setEntrypointEdited(false);
      setError(null);
    }
    onOpenChange(next);
  };

  const changeRuntime = (next: FunctionRuntime) => {
    setRuntime(next);
    const preset = RUNTIMES.find((item) => item.value === next);
    // Only follow the runtime while the operator has not named their own file.
    if (preset && !entrypointEdited) setEntrypoint(preset.entrypoint);
  };

  const normalized = toSlug(name);
  const trimmedEntrypoint = entrypoint.trim();
  const valid = isSlug(normalized) && trimmedEntrypoint.length > 0;

  const submit = async () => {
    if (!valid) return;
    setError(null);
    try {
      await create.mutateAsync({
        name: normalized,
        runtime,
        entrypoint: trimmedEntrypoint,
        // The API defaults these, but the request states them so what was
        // created matches what the dialog promised: enabled, logging on.
        commands: "",
        timeout_seconds: 15,
        enabled: true,
        logging: true,
      });
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not create the function.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Create function</DialogTitle>
          <DialogDescription>
            A function runs on demand. This creates its metadata; it starts
            answering once a deployment is uploaded.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor={fieldId}>Function name</Label>
          <Input
            id={fieldId}
            value={name}
            autoFocus
            autoComplete="off"
            spellCheck={false}
            aria-describedby={helpId}
            aria-invalid={name.length > 0 && !valid}
            placeholder="send-receipt"
            className="border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15"
            onChange={(event) => setName(event.target.value)}
          />
          <p
            id={helpId}
            className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]"
          >
            {name.length > 0 && !valid
              ? "Use 2 to 63 characters: lower-case letters, numbers, and hyphens."
              : normalized
                ? `Will be created as ${normalized}`
                : "Lower-case letters, numbers, and hyphens."}
          </p>

          <Label htmlFor={runtimeId} className="mt-1">
            Runtime
          </Label>
          <select
            id={runtimeId}
            value={runtime}
            onChange={(event) =>
              changeRuntime(event.target.value as FunctionRuntime)
            }
            className="h-9 w-full rounded-lg border border-[var(--projects-border)] bg-[var(--projects-card-bg)] px-2.5 text-[13px] text-[var(--projects-text)] focus-visible:border-[var(--user-accent)]/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/15"
          >
            {RUNTIMES.map((item) => (
              <option key={item.value} value={item.value}>
                {item.label}
              </option>
            ))}
          </select>

          <Label htmlFor={entrypointId} className="mt-1">
            Entrypoint
          </Label>
          <Input
            id={entrypointId}
            value={entrypoint}
            autoComplete="off"
            spellCheck={false}
            placeholder={RUNTIMES.find((item) => item.value === runtime)?.entrypoint}
            className={cn(
              "border-[var(--projects-border)] bg-[var(--projects-card-bg)] font-mono text-[13px] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)]",
              "focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15",
            )}
            onChange={(event) => {
              setEntrypointEdited(true);
              setEntrypoint(event.target.value);
            }}
          />
          <p className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]">
            A path inside the deployment, such as src/main.js.
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
              {create.isPending ? "Creating…" : "Create function"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
