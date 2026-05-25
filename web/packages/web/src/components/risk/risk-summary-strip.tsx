"use client";

import useSWR from "swr";
import { Flame, Users, Trash2, Lightbulb, ShieldAlert } from "lucide-react";
import { StatCard } from "@argus-dev/ui/shared/stat-card";
import { formatNumber } from "@argus-dev/ui/lib/format";
import { getGitSummary, getOwnership } from "@/lib/api/git";
import { getDeadCodeSummary } from "@/lib/api/dead-code";
import { getDecisionHealth } from "@/lib/api/decisions";
import { listSecurityFindings } from "@/lib/api/security";

const RISK_BASE = (repoId: string) => `/repos/${repoId}/risk`;

/**
 * Always-visible 5-card strip atop /repos/[id]/risk. Pulls cached SWR data
 * shared with the tab bodies, so switching tabs doesn't trigger refetches.
 */
export function RiskSummaryStrip({ repoId }: { repoId: string }) {
  const { data: gitSummary } = useSWR(`git-summary:${repoId}`, () => getGitSummary(repoId), {
    revalidateOnFocus: false,
  });
  const { data: ownership } = useSWR(`ownership:${repoId}:module`, () =>
    getOwnership(repoId, "module"),
  );
  const { data: deadCode } = useSWR(`dead-code-summary:${repoId}`, () =>
    getDeadCodeSummary(repoId),
  );
  const { data: decisionHealth } = useSWR(
    `decision-health:${repoId}`,
    () => getDecisionHealth(repoId),
    { revalidateOnFocus: false },
  );
  const { data: securityFindings } = useSWR(
    `security-findings:${repoId}`,
    // Pull a tight slice; we only need the count for the badge here.
    () => listSecurityFindings(repoId, { limit: 200 }).catch(() => [] as Awaited<ReturnType<typeof listSecurityFindings>>),
    { revalidateOnFocus: false },
  );

  const siloCount = ownership ? ownership.filter((o) => o.is_silo).length : null;
  const reclaimable = deadCode ? (deadCode.deletable_lines ?? 0) : null;
  const staleDecisions = decisionHealth ? (decisionHealth.stale_decisions?.length ?? 0) : null;
  const securityCritical = securityFindings
    ? securityFindings.filter((f) => f.severity === "high" || f.severity === "critical").length
    : null;

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      <StatCard
        label="Hotspots"
        value={gitSummary ? formatNumber(gitSummary.hotspot_count) : "–"}
        description="files with above-average churn"
        icon={<Flame className="h-4 w-4 text-orange-400" />}
        href={`${RISK_BASE(repoId)}?tab=hotspots`}
      />
      <StatCard
        label="Silos"
        value={siloCount !== null ? formatNumber(siloCount) : "–"}
        description="modules owned by one person"
        icon={<Users className="h-4 w-4 text-amber-400" />}
        href={`${RISK_BASE(repoId)}?tab=heatmap`}
      />
      <StatCard
        label="Reclaimable"
        value={reclaimable !== null ? formatNumber(reclaimable) : "–"}
        description="dead-code lines safe to delete"
        icon={<Trash2 className="h-4 w-4 text-rose-400" />}
        href={`${RISK_BASE(repoId)}?tab=dead-code`}
      />
      <StatCard
        label="Stale Decisions"
        value={staleDecisions !== null ? formatNumber(staleDecisions) : "–"}
        description="ADRs not reviewed in 6+ months"
        icon={<Lightbulb className="h-4 w-4 text-yellow-400" />}
        href={`/repos/${repoId}/decisions`}
      />
      <StatCard
        label="Security"
        value={securityCritical !== null ? formatNumber(securityCritical) : "–"}
        description={securityCritical !== null ? "critical / high findings" : "scan not run"}
        icon={<ShieldAlert className="h-4 w-4 text-red-400" />}
        href={`/repos/${repoId}/security`}
      />
    </div>
  );
}
