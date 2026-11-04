"use client";

import { useId, useState } from "react";
import { toast } from "sonner";
import type { components } from "@/api/generated/schema";
import { useCreateDatabaseRow } from "@/api/mutations/databases";
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
import { Textarea } from "@/components/ui/textarea";
import {
  PRIMARY_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";
import { cn } from "@/lib/utils";

type Column = components["schemas"]["DatabaseColumn"];

const CONTROL_CLASS =
  "border-[var(--projects-border)] bg-[var(--projects-card-bg)] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15";

function defaultValue(column: Column): string {
  if (!column.required) return "";
  if (column.type === "boolean") return "false";
  return "";
}

function buildRowData(
  columns: Column[],
  values: Record<string, string>,
): Record<string, unknown> {
  const data: Record<string, unknown> = {};
  for (const column of columns) {
    const raw = values[column.key] ?? "";
    if (raw.trim() === "") continue;
    switch (column.type) {
      case "integer": {
        const value = Number(raw);
        if (!Number.isSafeInteger(value)) {
          throw new Error(`${column.key} must be a whole number.`);
        }
        data[column.key] = value;
        break;
      }
      case "double": {
        const value = Number(raw);
        if (!Number.isFinite(value)) {
          throw new Error(`${column.key} must be a number.`);
        }
        data[column.key] = value;
        break;
      }
      case "boolean":
        data[column.key] = raw === "true";
        break;
      case "json":
        try {
          data[column.key] = JSON.parse(raw);
        } catch {
          throw new Error(`${column.key} must be valid JSON.`);
        }
        break;
      default:
        data[column.key] = raw;
    }
  }
  return data;
}

/**
 * Adds one typed row.
 *
 * The inputs follow the declared column type: numbers are sent as numbers,
 * booleans as booleans, and json as parsed JSON, because the API validates
 * against the schema and rejects a string where a number was declared. An
 * untouched optional field is omitted so the server applies its column default.
 */
export function CreateRowDialog({
  projectId,
  databaseId,
  tableId,
  tableName,
  columns,
  open,
  onOpenChange,
}: {
  projectId: string;
  databaseId: string;
  tableId: string;
  tableName: string;
  columns: Column[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const create = useCreateDatabaseRow(projectId, databaseId, tableId);
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      columns.map((column) => [column.key, defaultValue(column)]),
    ),
  );
  const [error, setError] = useState<string | null>(null);
  const fieldId = useId();

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setValues(
        Object.fromEntries(
          columns.map((column) => [column.key, defaultValue(column)]),
        ),
      );
      setError(null);
    }
    onOpenChange(next);
  };

  const setValue = (key: string, value: string) =>
    setValues((current) => ({ ...current, [key]: value }));

  const submit = async () => {
    setError(null);
    let data: Record<string, unknown>;
    try {
      data = buildRowData(columns, values);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Invalid row data.");
      return;
    }
    try {
      await create.mutateAsync({ data });
      toast.success(`Added a row to ${tableName}`);
      handleOpenChange(false);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Could not add the row.",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Add row</DialogTitle>
          <DialogDescription>
            Values are validated against the columns declared on {tableName}.
            Leave an optional field empty to use its default.
          </DialogDescription>
        </DialogHeader>

        <form
          className="mt-4 flex max-h-[60vh] flex-col gap-3 overflow-y-auto pr-1"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          {columns.length === 0 ? (
            <p className="m-0 text-[13px] leading-5 text-[var(--projects-muted)]">
              This table has no declared columns yet. The row will be stored
              with an empty data object.
            </p>
          ) : null}

          {columns.map((column) => {
            const inputId = `${fieldId}-${column.key}`;
            return (
              <div key={column.key} className="flex flex-col gap-1.5">
                <Label htmlFor={inputId}>
                  {column.key}
                  {column.required ? (
                    <span
                      className="ml-1 text-[var(--projects-danger)]"
                      aria-hidden="true"
                    >
                      *
                    </span>
                  ) : null}
                </Label>

                {column.type === "boolean" ? (
                  <select
                    id={inputId}
                    value={values[column.key] ?? ""}
                    onChange={(event) =>
                      setValue(column.key, event.target.value)
                    }
                    className={cn(
                      "h-9 w-full rounded-lg border px-2.5 text-[13px]",
                      CONTROL_CLASS,
                    )}
                  >
                    <option value="">Not set</option>
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </select>
                ) : column.type === "json" ? (
                  <Textarea
                    id={inputId}
                    value={values[column.key] ?? ""}
                    spellCheck={false}
                    rows={3}
                    placeholder='{"key":"value"}'
                    className={cn("font-mono text-[13px]", CONTROL_CLASS)}
                    onChange={(event) =>
                      setValue(column.key, event.target.value)
                    }
                  />
                ) : (
                  <Input
                    id={inputId}
                    value={values[column.key] ?? ""}
                    type={
                      column.type === "integer" || column.type === "double"
                        ? "number"
                        : "text"
                    }
                    step={column.type === "integer" ? "1" : undefined}
                    autoComplete="off"
                    spellCheck={false}
                    placeholder={
                      column.type === "datetime"
                        ? "2026-01-31T10:00:00Z"
                        : undefined
                    }
                    className={cn(
                      column.type === "integer" || column.type === "double"
                        ? ""
                        : "font-mono text-[13px]",
                      CONTROL_CLASS,
                    )}
                    onChange={(event) =>
                      setValue(column.key, event.target.value)
                    }
                  />
                )}

                <p className="m-0 text-[11.5px] leading-4 text-[var(--projects-muted)]">
                  {column.type}
                  {column.varchar_size ? `(${column.varchar_size})` : ""}
                  {column.default !== undefined && column.default !== null
                    ? ` · default ${JSON.stringify(column.default)}`
                    : ""}
                </p>
              </div>
            );
          })}

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
              disabled={create.isPending}
            >
              {create.isPending ? "Adding…" : "Add row"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
