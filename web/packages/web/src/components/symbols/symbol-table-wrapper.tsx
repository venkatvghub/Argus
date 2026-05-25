"use client";

import { useMemo, useState } from "react";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";
import { SymbolTable, type SymbolFilters } from "@argus-dev/ui/symbols/symbol-table";
import { HotSymbolsBoard, type HotSymbol } from "@argus-dev/ui/symbols/hot-symbols-board";
import { SymbolDrawerWrapper } from "./symbol-drawer-wrapper";
import { listSymbolsPage, type SymbolSortKey } from "@/lib/api/symbols";
import { getRepoStats } from "@/lib/api/repos";
import { useDebounce } from "@/lib/hooks/use-debounce";
import type { Paginated, SymbolResponse } from "@/lib/api/types";

const LIMIT = 50;

interface Props {
  repoId: string;
}

export function SymbolTableWrapper({ repoId }: Props) {
  const [filters, setFilters] = useState<SymbolFilters>({
    q: "",
    kind: "all",
    language: "all",
    visibility: "all",
    inHotFiles: false,
    inEntryPoints: false,
    sort: "importance",
  });
  const debouncedQ = useDebounce(filters.q, 300);
  const [selected, setSelected] = useState<SymbolResponse | null>(null);

  // The fetch key changes whenever any filter does — SWR will fall back to
  // page 0 cleanly without us having to reset infinite state by hand.
  const keyPayload = useMemo(
    () => ({ ...filters, q: debouncedQ }),
    [filters, debouncedQ],
  );

  const { data, size, setSize, isLoading, isValidating } = useSWRInfinite<
    Paginated<SymbolResponse>
  >(
    (pageIndex, previousPageData) => {
      if (previousPageData && !previousPageData.has_more) return null;
      return `symbols:${repoId}:${JSON.stringify(keyPayload)}:${pageIndex}`;
    },
    (key) => {
      const pageIndex = parseInt(key.split(":").pop()!, 10);
      return listSymbolsPage({
        repo_id: repoId,
        q: keyPayload.q || undefined,
        kind: keyPayload.kind !== "all" ? keyPayload.kind : undefined,
        language: keyPayload.language !== "all" ? keyPayload.language : undefined,
        visibility:
          keyPayload.visibility !== "all" ? keyPayload.visibility : undefined,
        in_hot_files: keyPayload.inHotFiles || undefined,
        in_entry_points: keyPayload.inEntryPoints || undefined,
        sort: keyPayload.sort,
        limit: LIMIT,
        offset: pageIndex * LIMIT,
      });
    },
    { revalidateOnFocus: false, revalidateFirstPage: false },
  );

  const items = useMemo(
    () => (data ? data.flatMap((p) => p.items) : []),
    [data],
  );
  const total = data && data.length > 0 ? data[0].total : 0;
  const hasMore = data ? data[data.length - 1].has_more : false;

  // Hot symbols board — uses the same server-ranked stream so it matches
  // the table's order. We just take the top of the first page.
  const hotSymbols: HotSymbol[] = useMemo(() => {
    const firstPage = data && data.length > 0 ? data[0].items : [];
    return firstPage
      .filter((s) => (s.importance_score ?? 0) > 0)
      .slice(0, 8)
      .map((s) => ({
        symbol: s,
        score: s.importance_score ?? 0,
        pagerank: s.file_pagerank ?? 0,
      }));
  }, [data]);

  // Fetch repo stats once to get the full language list for the dropdown.
  const { data: statsData } = useSWR(
    `repo-stats:${repoId}`,
    () => getRepoStats(repoId),
    { revalidateOnFocus: false },
  );
  const availableLanguages = useMemo(() => {
    if (statsData?.languages && Object.keys(statsData.languages).length > 0) {
      return Object.keys(statsData.languages).sort();
    }
    // Fallback: collect from loaded symbol pages.
    const seen = new Set<string>();
    for (const page of data ?? []) {
      for (const sym of page.items) {
        if (sym.language) seen.add(sym.language);
      }
    }
    return Array.from(seen).sort();
  }, [statsData, data]);

  return (
    <div className="space-y-6">
      {hotSymbols.length > 0 && (
        <HotSymbolsBoard items={hotSymbols} onSelect={setSelected} />
      )}
      <SymbolTable
        items={items}
        repoId={repoId}
        isLoading={isLoading}
        isValidating={isValidating}
        hasMore={hasMore}
        total={total}
        filters={filters}
        onFiltersChange={setFilters}
        onLoadMore={() => setSize(size + 1)}
        onSelect={setSelected}
        availableLanguages={availableLanguages}
        drawer={
          <SymbolDrawerWrapper
            symbol={selected}
            repoId={repoId}
            onClose={() => setSelected(null)}
          />
        }
      />
    </div>
  );
}
