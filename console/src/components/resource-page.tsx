"use client";

import type { ReactNode } from "react";
import { Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";
import { useEffect, useMemo, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import type { ServerPagination } from "@/components/data-table";

export function ResourceTableCard<T extends object>({
  data,
  searchable,
  searchPlaceholder = "Search this page",
  serverPagination,
  children,
}: {
  data: T[];
  searchable?: (item: T, term: string) => boolean;
  searchPlaceholder?: string;
  serverPagination?: ServerPagination;
  children: (filtered: T[], pagination?: ServerPagination) => ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const querySearch = searchParams.get("search") ?? "";
  // The input is the source of truth while typing so each keystroke filters the
  // already-loaded page immediately instead of triggering a router navigation.
  // The URL is updated after a short idle, and external URL changes (back /
  // forward, links) are adopted during render.
  const [search, setSearch] = useState(querySearch);
  const [syncedSearch, setSyncedSearch] = useState(querySearch);
  if (querySearch !== syncedSearch) {
    setSyncedSearch(querySearch);
    setSearch(querySearch);
  }
  useEffect(() => {
    if (search === querySearch) return;
    const timer = window.setTimeout(() => {
      const next = new URLSearchParams(searchParams.toString());
      if (search) next.set("search", search);
      else next.delete("search");
      next.delete("cursor");
      router.replace(
        `${pathname}${next.toString() ? `?${next.toString()}` : ""}`,
        { scroll: false },
      );
    }, 300);
    return () => window.clearTimeout(timer);
  }, [pathname, querySearch, router, search, searchParams]);
  const filtered = useMemo(
    () =>
      searchable && search
        ? data.filter((item) => searchable(item, search.toLowerCase()))
        : data,
    [data, search, searchable],
  );
  return (
    <Card>
      {searchable ? (
        <div className="border-b border-graphite bg-white/[0.01] p-4">
          <div className="relative max-w-sm">
            <Search
              className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ash"
              aria-hidden="true"
            />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={searchPlaceholder}
              className="pl-9"
              aria-label={searchPlaceholder}
            />
          </div>
          <p className="mt-2 text-[11px] leading-5 text-fog">
            Searches the records on this page. The API does not expose a global
            resource search for this list.
          </p>
        </div>
      ) : null}
      {children(filtered, serverPagination)}
    </Card>
  );
}
