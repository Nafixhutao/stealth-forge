"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { ChevronRight } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { formatDate } from "@/lib/format";

export function downloadLogs(items: AdminLogItem[]) {
  const payload = items.map((item) => ({
    timestamp: item.timestamp,
    service: item.service,
    level: item.level ?? "unknown",
    message: item.message,
    trace_id: item.trace_id ?? null,
    span_id: item.span_id ?? null,
    attributes: item.attributes ?? {},
    resource_attributes: item.resource_attributes ?? {},
  }));
  const blob = new Blob([JSON.stringify(payload, null, 2)], {
    type: "application/json",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `stealth-logs-${new Date().toISOString().replaceAll(":", "-")}.json`;
  link.click();
  URL.revokeObjectURL(url);
}

export type AdminLogItem = {
  timestamp: string;
  trace_id?: string;
  span_id?: string;
  level?: string;
  service: string;
  message: string;
  attributes?: Record<string, string>;
  resource_attributes?: Record<string, string>;
};

export function VirtualizedLogTable({ items }: { items: AdminLogItem[] }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [selected, setSelected] = useState<AdminLogItem | null>(null);
  // TanStack Virtual intentionally exposes an imperative virtualizer instance;
  // React Compiler must not memoize that object as if it were render data.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 72,
    overscan: 8,
    getItemKey: (index) =>
      `${items[index].timestamp}-${items[index].trace_id ?? ""}-${index}`,
  });

  return (
    <>
      <div ref={scrollRef} className="max-h-[620px] overflow-auto">
        <div
          role="table"
          aria-label="Structured logs"
          aria-rowcount={items.length + 1}
          className="min-w-[760px] text-left text-sm"
        >
          <div
            role="row"
            aria-rowindex={1}
            className="sticky top-0 z-10 grid grid-cols-[150px_130px_90px_minmax(300px,1fr)_170px_116px] border-b border-graphite bg-carbon text-xs uppercase tracking-[0.1em] text-fog"
          >
            <div role="columnheader" className="px-4 py-3 font-medium">
              Time
            </div>
            <div role="columnheader" className="px-4 py-3 font-medium">
              Service
            </div>
            <div role="columnheader" className="px-4 py-3 font-medium">
              Level
            </div>
            <div role="columnheader" className="px-4 py-3 font-medium">
              Message
            </div>
            <div role="columnheader" className="px-4 py-3 font-medium">
              Trace
            </div>
            <div role="columnheader" className="px-4 py-3 font-medium">
              Detail
            </div>
          </div>
          <div
            role="rowgroup"
            style={{
              height: `${virtualizer.getTotalSize()}px`,
              position: "relative",
            }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const item = items[virtualRow.index];
              return (
                <div
                  key={virtualRow.key}
                  role="row"
                  aria-rowindex={virtualRow.index + 2}
                  className="absolute left-0 grid h-[72px] w-full grid-cols-[150px_130px_90px_minmax(300px,1fr)_170px_116px] border-b border-graphite text-left align-top transition-colors duration-150 hover:bg-white/[0.025]"
                  style={{
                    top: 0,
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <span
                    role="cell"
                    className="whitespace-nowrap px-4 py-3 font-mono text-xs text-fog"
                  >
                    {formatDate(item.timestamp)}
                  </span>
                  <span role="cell" className="truncate px-4 py-3 text-mist">
                    {item.service}
                  </span>
                  <span role="cell" className="px-4 py-3">
                    <Badge variant="neutral">{item.level ?? "unknown"}</Badge>
                  </span>
                  <span
                    role="cell"
                    className="truncate px-4 py-3 text-mist"
                    title={item.message}
                  >
                    {item.message}
                  </span>
                  <span
                    role="cell"
                    className="truncate px-4 py-3 font-mono text-xs text-fog"
                  >
                    {item.trace_id ?? "none"}
                  </span>
                  <span role="cell" className="px-2 py-3">
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setSelected(item)}
                      aria-label={`Open log from ${item.service}`}
                    >
                      Open{" "}
                      <ChevronRight className="size-3.5" aria-hidden="true" />
                    </Button>
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      </div>
      <Dialog
        open={selected !== null}
        onOpenChange={(open) => !open && setSelected(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Log detail</DialogTitle>
            <DialogDescription>
              Structured fields are redacted at the telemetry query boundary.
            </DialogDescription>
          </DialogHeader>
          {selected ? <LogDetail item={selected} /> : null}
        </DialogContent>
      </Dialog>
    </>
  );
}

function LogDetail({ item }: { item: AdminLogItem }) {
  return (
    <div className="space-y-5 text-sm">
      <div className="grid gap-3 sm:grid-cols-2">
        <LogField label="Time" value={formatDate(item.timestamp)} mono />
        <LogField label="Service" value={item.service} />
        <LogField label="Level" value={item.level ?? "unknown"} />
        <LogField
          label="Trace ID"
          value={item.trace_id ?? "none"}
          mono
          href={
            item.trace_id
              ? `/admin/telemetry/traces?trace_id=${encodeURIComponent(item.trace_id)}`
              : undefined
          }
        />
        <LogField label="Span ID" value={item.span_id ?? "none"} mono />
      </div>
      <div>
        <p className="mb-2 text-xs uppercase tracking-[0.1em] text-fog">
          Message
        </p>
        <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border border-graphite bg-void p-3 font-mono text-xs leading-5 text-mist">
          {item.message}
        </pre>
      </div>
      <AttributeGroup label="Attributes" values={item.attributes} />
      <AttributeGroup
        label="Resource attributes"
        values={item.resource_attributes}
      />
    </div>
  );
}

function LogField({
  label,
  value,
  mono = false,
  href,
}: {
  label: string;
  value: string;
  mono?: boolean;
  href?: string;
}) {
  return (
    <div className="min-w-0">
      <p className="text-xs uppercase tracking-[0.1em] text-fog">{label}</p>
      {href ? (
        <Link
          href={href}
          className={`mt-1 block truncate text-mist underline decoration-graphite underline-offset-4 hover:text-paper ${mono ? "font-mono text-xs" : ""}`}
        >
          {value}
        </Link>
      ) : (
        <p
          className={`mt-1 truncate text-mist ${mono ? "font-mono text-xs" : ""}`}
        >
          {value}
        </p>
      )}
    </div>
  );
}

function AttributeGroup({
  label,
  values,
}: {
  label: string;
  values?: Record<string, string>;
}) {
  const entries = Object.entries(values ?? {});
  if (!entries.length) return null;
  return (
    <div>
      <p className="mb-2 text-xs uppercase tracking-[0.1em] text-fog">
        {label}
      </p>
      <div className="divide-y divide-graphite rounded-md border border-graphite">
        {entries.map(([key, value]) => (
          <div
            key={key}
            className="grid gap-2 px-3 py-2 sm:grid-cols-[0.8fr_1.2fr]"
          >
            <span className="truncate font-mono text-xs text-fog">{key}</span>
            <span className="break-words font-mono text-xs text-mist">
              {value}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
