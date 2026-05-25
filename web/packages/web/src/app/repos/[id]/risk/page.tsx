"use client";

import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useCallback } from "react";
import { ShieldAlert } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@argus-dev/ui/ui/tabs";
import { HeatmapTab } from "@/components/risk/heatmap-tab";
import { HotspotsTab } from "@/components/risk/hotspots-tab";
import { DeadCodeTab } from "@/components/risk/dead-code-tab";
import { ImpactTab } from "@/components/risk/impact-tab";
import { ModulesTab } from "@/components/risk/modules-tab";
import { RiskSummaryStrip } from "@/components/risk/risk-summary-strip";

const TABS = ["heatmap", "hotspots", "modules", "dead-code", "impact"] as const;
type TabId = (typeof TABS)[number];

export default function RiskPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const repoId = params.id;

  const rawTab = searchParams.get("tab") as TabId | null;
  const activeTab: TabId = rawTab && TABS.includes(rawTab) ? rawTab : "heatmap";

  const setTab = useCallback(
    (next: string) => {
      const sp = new URLSearchParams(searchParams.toString());
      if (next === "heatmap") sp.delete("tab");
      else sp.set("tab", next);
      const qs = sp.toString();
      router.replace(qs ? `?${qs}` : "?", { scroll: false });
    },
    [router, searchParams],
  );

  return (
    <div className="p-4 sm:p-6 space-y-6 max-w-[1600px]">
      <div>
        <h1 className="text-xl font-semibold text-[var(--color-text-primary)] mb-1 flex items-center gap-2">
          <ShieldAlert className="h-5 w-5 text-orange-500" />
          Risk
        </h1>
        <p className="text-sm text-[var(--color-text-secondary)]">
          Where the risk is concentrated — ownership silos, churn hotspots, dead code, and PR
          blast radius — in one place.
        </p>
      </div>

      <RiskSummaryStrip repoId={repoId} />

      <details className="rounded-lg border border-[var(--color-border-default)] bg-[var(--color-bg-elevated)] px-4 py-3 text-xs text-[var(--color-text-secondary)]">
        <summary className="cursor-pointer font-medium text-[var(--color-text-primary)] select-none">
          What do these metrics mean?
        </summary>
        <dl className="mt-3 grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2 lg:grid-cols-3">
          <div><dt className="font-semibold text-[var(--color-text-primary)]">Hotspot</dt><dd>A file changed so often it statistically contains more bugs. Ranked by churn percentile — how often it was edited vs. all other files in this repo.</dd></div>
          <div><dt className="font-semibold text-[var(--color-text-primary)]">Bus factor</dt><dd>Minimum number of contributors whose loss would leave the code unmaintainable. Factor&nbsp;≤&nbsp;1 means one person&rsquo;s absence puts that module at serious risk.</dd></div>
          <div><dt className="font-semibold text-[var(--color-text-primary)]">Silo</dt><dd>A module where &gt;80% of commits come from a single developer. Ownership concentration — not a bug, but a knowledge-risk flag.</dd></div>
          <div><dt className="font-semibold text-[var(--color-text-primary)]">Reclaimable lines</dt><dd>Dead-code lines that have no callers or importers and have been confirmed safe to delete. Fewer lines = lower maintenance burden.</dd></div>
          <div><dt className="font-semibold text-[var(--color-text-primary)]">Stale decisions</dt><dd>Architecture Decision Records (ADRs) that were last reviewed more than 6 months ago and may no longer reflect the current design.</dd></div>
          <div><dt className="font-semibold text-[var(--color-text-primary)]">Hotspot trend</dt><dd>Whether a file&rsquo;s edit rate in the last 30 days is accelerating versus its 90-day baseline. Heating arrow means the risk is growing.</dd></div>
          <div><dt className="font-semibold text-[var(--color-text-primary)]">Ownership heatmap</dt><dd>Treemap where each tile is a module. Tile size = code volume. Color = primary owner. Border color = bus factor (red&nbsp;≤1, amber&nbsp;2, green&nbsp;≥3).</dd></div>
          <div><dt className="font-semibold text-[var(--color-text-primary)]">Health score</dt><dd>Composite 0–100 score across churn, ownership concentration, dead code, and doc coverage. Lower is worse.</dd></div>
        </dl>
      </details>

      <Tabs value={activeTab} onValueChange={setTab} className="space-y-4">
        <TabsList className="h-auto flex-wrap">
          <TabsTrigger value="heatmap">Heatmap</TabsTrigger>
          <TabsTrigger value="hotspots">Hotspots</TabsTrigger>
          <TabsTrigger value="modules">Modules</TabsTrigger>
          <TabsTrigger value="dead-code">Dead code</TabsTrigger>
          <TabsTrigger value="impact">Impact analyzer</TabsTrigger>
        </TabsList>

        <TabsContent value="heatmap" className="space-y-6">
          <HeatmapTab repoId={repoId} />
        </TabsContent>
        <TabsContent value="hotspots" className="space-y-6">
          <HotspotsTab repoId={repoId} />
        </TabsContent>
        <TabsContent value="modules" className="space-y-6">
          <ModulesTab repoId={repoId} />
        </TabsContent>
        <TabsContent value="dead-code" className="space-y-6">
          <DeadCodeTab repoId={repoId} />
        </TabsContent>
        <TabsContent value="impact" className="space-y-6">
          <ImpactTab repoId={repoId} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
