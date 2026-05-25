import { describe, it, expect, vi, afterEach } from "vitest";
import type { Hotspot } from "@argus-dev/types/git";
import { warnHotspotsMissingFilePath } from "../../src/lib/hotspot-validation.js";

const baseHotspot: Hotspot = {
  file_path: "src/main.ts",
  commit_count_90d: 12,
  commit_count_30d: 4,
  churn_percentile: 88,
  primary_owner: "alice",
  is_hotspot: true,
  is_stable: false,
  bus_factor: 2,
  contributor_count: 3,
  lines_added_90d: 100,
  lines_deleted_90d: 20,
  avg_commit_size: 10,
  commit_categories: {},
};

describe("warnHotspotsMissingFilePath", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("warns with identifying fields when file_path is missing", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const invalid = { ...baseHotspot, file_path: "" };

    warnHotspotsMissingFilePath([invalid], "getHotspotsPage");

    expect(warn).toHaveBeenCalledOnce();
    expect(warn).toHaveBeenCalledWith(
      "[getHotspotsPage] hotspot missing file_path",
      expect.objectContaining({
        primary_owner: "alice",
        commit_count_90d: 12,
        hotspot: invalid,
      }),
    );
  });

  it("does not warn when every hotspot has file_path", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});

    warnHotspotsMissingFilePath([baseHotspot], "HotspotsMini");

    expect(warn).not.toHaveBeenCalled();
  });
});
