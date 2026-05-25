import type { Metadata } from "next";
import { Code2 } from "lucide-react";
import { SymbolTableWrapper as SymbolTable } from "@/components/symbols/symbol-table-wrapper";

export const metadata: Metadata = { title: "Symbols" };

export default async function SymbolsPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  return (
    <div className="p-4 sm:p-6 space-y-6 max-w-[1600px]">
      <div>
        <h1 className="text-xl font-semibold text-[var(--color-text-primary)] mb-1 flex items-center gap-2">
          <Code2 className="h-5 w-5 text-[var(--color-accent-primary)]" />
          Symbol Index
        </h1>
        <p className="text-sm text-[var(--color-text-secondary)]">
          Searchable index of all functions, classes, and exports parsed from this repository.
        </p>
        <p className="mt-2 text-xs text-[var(--color-text-tertiary)] max-w-2xl">
          Symbols are ranked by <strong className="text-[var(--color-text-secondary)]">importance</strong> — a combination of PageRank centrality, how many other files import this symbol, and whether it lives in a high-churn hotspot file. Use filters to narrow by kind, language, or visibility. Click a row to inspect callers, callees, git history, and health.
        </p>
      </div>
      <SymbolTable repoId={id} />
    </div>
  );
}
