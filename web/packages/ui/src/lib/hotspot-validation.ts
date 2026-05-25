import type { Hotspot } from "@argus-dev/types/git";

/** Emit a structured warning for hotspots missing file_path so bad API rows can be traced. */
export function warnHotspotsMissingFilePath(
  hotspots: ReadonlyArray<Hotspot>,
  context: string,
): void {
  for (const h of hotspots) {
    if (h.file_path) continue;
    console.warn(`[${context}] hotspot missing file_path`, {
      primary_owner: h.primary_owner,
      commit_count_90d: h.commit_count_90d,
      commit_count_30d: h.commit_count_30d,
      churn_percentile: h.churn_percentile,
      is_hotspot: h.is_hotspot,
      hotspot: h,
    });
  }
}
