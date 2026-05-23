package analysis

const (
	tiktokenEncoding = "cl100k_base"

	tokenBloatThresholdDefault = 50.0

	phantomCouplingChurnThreshold     = 5
	phantomCouplingOwnershipThreshold = 0.5
	ownershipPercentMultiplier        = 100

	dartAsyncGapMaxRunes = 200
	sqlConcatWindowRunes = 100
	sqlCredentialMinLen  = 3

	pageRankDamping     = 0.85
	pageRankTolerance   = 1e-6
	leidenResolution    = 1.0
	leidenIterations    = 10
	floatCompareEpsilon = 1e-9

	// Phase 5.1 — Structural Complexity markers
	brainMethodNLOCThreshold  = 50
	brainMethodCyclomaticMin  = 15
	brainMethodNestingMin     = 4
	brainMethodPageRankTopPct = 0.10 // top 10%
	nestedComplexityDepthMin  = 4
	bumpyRoadBranchMin        = 3

	// Phase 5.2 — Size & API Complexity markers
	complexMethodCyclomaticMin  = 9
	largeMethodNLOCGo           = 60
	largeMethodNLOCJavaPython   = 80
	largeMethodNLOCDefault      = 60
	primitiveObsessionParamMin  = 6

	// Phase 5.3 — DRY Violation
	dryHashWindow              = 6
	dryHashStride              = 3
	dryHashMinSimilarity       = 0.80
	dryActiveDaysThreshold     = 90
	dryActiveDeductionMultiplier = 1.5
)
