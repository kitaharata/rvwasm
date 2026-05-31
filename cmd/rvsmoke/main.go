package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kitaharata/rvwasm/internal/analyze"
)

type artifactFlags map[string]string

type repeatedStrings []string

var globalDryRun bool

func (r *repeatedStrings) String() string { return strings.Join(*r, ",") }
func (r *repeatedStrings) Set(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("empty value")
	}
	*r = append(*r, s)
	return nil
}

func (a *artifactFlags) String() string {
	if a == nil || len(*a) == 0 {
		return ""
	}
	keys := make([]string, 0, len(*a))
	for k := range *a {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+(*a)[k])
	}
	return strings.Join(parts, ",")
}

func (a *artifactFlags) Set(s string) error {
	if *a == nil {
		*a = artifactFlags{}
	}
	k, v, ok := strings.Cut(s, "=")
	if !ok || strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
		return fmt.Errorf("artifact must be role=/path/to/file")
	}
	(*a)[strings.TrimSpace(k)] = strings.TrimSpace(v)
	return nil
}

type cliOutput struct {
	Integrity               analyze.BundleIntegrityReport           `json:"integrity"`
	Signature               analyze.LogSignatureSet                 `json:"signature"`
	CI                      analyze.CISummary                       `json:"ci"`
	Gate                    analyze.CIGateReport                    `json:"gate"`
	Checklist               analyze.CIActionChecklist               `json:"checklist"`
	Runner                  analyze.HeadlessSmokeRunnerSpec         `json:"runner"`
	PolicyTree              analyze.PolicyViolationTree             `json:"policy_violation_tree"`
	History                 analyze.BundleHistoryAggregate          `json:"history"`
	ReproPackage            analyze.MinimalReproPackageSpec         `json:"repro_package"`
	ReproZip                analyze.ReproZipInspection              `json:"repro_zip,omitempty"`
	TrendChart              analyze.BundleTrendChartData            `json:"trend_chart,omitempty"`
	Matrix                  analyze.MatrixResultAggregate           `json:"matrix_aggregate,omitempty"`
	MatrixFlakes            analyze.MatrixFlakeReport               `json:"matrix_flakes,omitempty"`
	ReproChecksums          analyze.ReproZipChecksumManifest        `json:"repro_zip_checksums,omitempty"`
	ReproVerify             analyze.ReproZipChecksumVerification    `json:"repro_zip_checksum_verification,omitempty"`
	ArtifactIndex           analyze.CIArtifactIndex                 `json:"ci_artifact_index,omitempty"`
	Release                 analyze.ReleaseBundleManifest           `json:"release_bundle_manifest,omitempty"`
	SBOM                    analyze.DependencyInventory             `json:"dependency_inventory,omitempty"`
	Attestation             analyze.ProvenanceAttestation           `json:"provenance_attestation,omitempty"`
	ReleaseZip              analyze.ReleaseHandoffPackageInspection `json:"release_handoff_zip,omitempty"`
	AttestationVerification analyze.AttestationVerification         `json:"attestation_verification,omitempty"`
	SBOMDiff                analyze.DependencyInventoryDiff         `json:"dependency_inventory_diff,omitempty"`
	ReleaseZipCompare       analyze.ReleaseHandoffPackageComparison `json:"release_handoff_zip_compare,omitempty"`
	Retention               analyze.RetentionManifest               `json:"retention_manifest,omitempty"`
	RetentionAudit          analyze.RetentionAudit                  `json:"retention_audit,omitempty"`
	ReleaseScore            analyze.ReleaseVerificationScore        `json:"release_verification_score,omitempty"`
	ReleaseVerifyGate       analyze.ReleaseVerificationGateReport   `json:"release_verification_gate,omitempty"`
	ReleaseAudit            analyze.ReleaseAuditReport              `json:"release_audit,omitempty"`
	ReleaseAuditDiff        analyze.ReleaseAuditDiffReport          `json:"release_audit_diff,omitempty"`
	ReleaseWaivers          analyze.ReleaseWaiverReport             `json:"release_waivers,omitempty"`
	ReleaseTodos            analyze.ReleaseAuditTodoReport          `json:"release_audit_todos,omitempty"`
	WaiverCalendar          analyze.WaiverExpiryCalendar            `json:"waiver_expiry_calendar,omitempty"`
	ReleaseChangelog        analyze.ReleaseAuditChangelog           `json:"release_audit_changelog,omitempty"`
	FinalDecision           analyze.FinalReleaseDecision            `json:"final_release_decision,omitempty"`
	EvidenceBundle          analyze.ReleaseEvidenceBundleInspection `json:"release_evidence_bundle,omitempty"`
	BundleDiff              []analyze.DiagnosticBundleDiff          `json:"bundle_diff,omitempty"`
	Trend                   analyze.BundleTrendReport               `json:"bundle_trend,omitempty"`
	ArtifactChecks          []analyze.ArtifactIntegrityRow          `json:"artifact_checks,omitempty"`
	showReleaseAudit        bool
}

func releaseAuditOutputRequested(paths ...string) bool {
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			return true
		}
	}
	return false
}

func main() {
	bundlePath := flag.String("bundle", "", "diagnostic bundle JSON, compressed bundle JSON, or raw gzip+base64 payload")
	manifestPath := flag.String("manifest", "", "artifact manifest JSON when no diagnostic bundle is available")
	tracePath := flag.String("trace", "", "optional trace text for log signature")
	consolePath := flag.String("console", "", "optional console text for log signature")
	out := flag.String("out", "text", "text, json, md, html, junit, or sarif")
	baselinePath := flag.String("baseline", "", "optional baseline diagnostic bundle for regression diff")
	policyPath := flag.String("policy", "", "optional CI gate policy JSON")
	policyTemplate := flag.String("policy-template", "default", "CI gate policy template when -policy is not provided")
	printPolicy := flag.String("print-policy", "", "print a named CI gate policy template as JSON and exit")
	listPolicies := flag.Bool("list-policies", false, "list built-in CI gate policy templates and exit")
	printGithubActions := flag.String("print-github-actions", "", "print a GitHub Actions workflow for the named policy template and exit")
	printGithubActionsMatrix := flag.String("print-github-actions-matrix", "", "print a matrix GitHub Actions workflow for the named policy template and exit")
	githubActionsPath := flag.String("github-actions", "", "optional path to write GitHub Actions workflow YAML")
	policyTreePath := flag.String("policy-tree", "", "optional path to write policy violation tree Markdown")
	historyPath := flag.String("history", "", "optional path to write bundle history aggregate text")
	reproZipPath := flag.String("repro-zip", "", "optional path to write a minimal reproduction package zip")
	inspectReproZipPath := flag.String("inspect-repro-zip", "", "optional path to inspect a minimal reproduction package zip")
	trendCSVPath := flag.String("trend-csv", "", "optional path to write bundle trend CSV")
	trendChartPath := flag.String("trend-chart-json", "", "optional path to write bundle trend chart JSON")
	trendHTMLPath := flag.String("trend-html", "", "optional path to write standalone bundle trend HTML")
	githubActionsMatrixPath := flag.String("github-actions-matrix", "", "optional path to write matrix GitHub Actions workflow YAML")
	matrixSummaryPath := flag.String("matrix-summary", "", "optional path to write aggregated rvsmoke matrix result text")
	matrixSummaryJSONPath := flag.String("matrix-summary-json", "", "optional path to write aggregated rvsmoke matrix result JSON")
	matrixSummaryHTMLPath := flag.String("matrix-summary-html", "", "optional path to write aggregated rvsmoke matrix result HTML")
	reproChecksumsPath := flag.String("repro-checksums", "", "optional path to write minimal repro ZIP checksum manifest JSON")
	verifyReproChecksumsPath := flag.String("verify-repro-checksums", "", "optional baseline repro checksum manifest JSON to compare with the inspected repro ZIP")
	checksumVerifyPath := flag.String("checksum-verify", "", "optional path to write repro checksum verification text")
	checksumVerifyJSONPath := flag.String("checksum-verify-json", "", "optional path to write repro checksum verification JSON")
	matrixFlakesPath := flag.String("matrix-flakes", "", "optional path to write matrix flake report text")
	matrixFlakesJSONPath := flag.String("matrix-flakes-json", "", "optional path to write matrix flake report JSON")
	matrixFlakesHTMLPath := flag.String("matrix-flakes-html", "", "optional path to write matrix flake report HTML")
	artifactIndexPath := flag.String("artifact-index", "", "optional path to write CI artifact index JSON")
	releaseManifestPath := flag.String("release-manifest", "", "optional path to write release bundle manifest JSON")
	releaseHTMLPath := flag.String("release-html", "", "optional path to write navigable release bundle manifest HTML")
	goModPath := flag.String("go-mod", "go.mod", "optional go.mod path for dependency inventory/SBOM-lite output")
	sbomPath := flag.String("sbom", "", "optional path to write dependency inventory JSON")
	sbomTextPath := flag.String("sbom-text", "", "optional path to write dependency inventory text")
	attestationPath := flag.String("attestation", "", "optional path to write provenance attestation JSON")
	attestationTextPath := flag.String("attestation-text", "", "optional path to write provenance attestation text")
	releaseZipPath := flag.String("release-zip", "", "optional path to write release handoff ZIP with manifest/index/SBOM/attestation")
	inspectReleaseZipPath := flag.String("inspect-release-zip", "", "optional path to inspect release handoff ZIP")
	releaseZipInspectHTMLPath := flag.String("release-zip-inspect-html", "", "optional path to write release handoff ZIP inspection HTML")
	verifyAttestationPath := flag.String("verify-attestation", "", "optional path to write provenance attestation verification JSON")
	verifyAttestationTextPath := flag.String("verify-attestation-text", "", "optional path to write provenance attestation verification text")
	sbomBaselinePath := flag.String("sbom-baseline", "", "optional baseline dependency inventory JSON for SBOM-lite diff")
	sbomDiffPath := flag.String("sbom-diff", "", "optional path to write dependency inventory diff text")
	sbomDiffJSONPath := flag.String("sbom-diff-json", "", "optional path to write dependency inventory diff JSON")
	compareReleaseZipInspectionPath := flag.String("compare-release-zip-inspection", "", "optional baseline release handoff ZIP inspection JSON for comparison")
	releaseZipComparePath := flag.String("release-zip-compare", "", "optional path to write release handoff ZIP comparison text")
	releaseZipCompareJSONPath := flag.String("release-zip-compare-json", "", "optional path to write release handoff ZIP comparison JSON")
	retentionManifestPath := flag.String("retention-manifest", "", "optional path to write CI artifact retention manifest JSON")
	retentionTextPath := flag.String("retention-text", "", "optional path to write CI artifact retention manifest text")
	releaseVerificationHTMLPath := flag.String("release-verification-html", "", "optional path to write release verification HTML with attestation/SBOM/ZIP/retention sections")
	releaseVerifyPolicyPath := flag.String("release-verify-policy", "", "optional release verification gate policy JSON")
	releaseVerifyTemplate := flag.String("release-verify-template", "default", "release verification gate policy template when -release-verify-policy is not provided")
	printReleaseVerifyPolicy := flag.String("print-release-verify-policy", "", "print a named release verification gate policy template as JSON and exit")
	listReleaseVerifyPolicies := flag.Bool("list-release-verify-policies", false, "list built-in release verification policy templates and exit")
	retentionAuditPath := flag.String("retention-audit", "", "optional path to write retention expiry audit text")
	retentionAuditJSONPath := flag.String("retention-audit-json", "", "optional path to write retention expiry audit JSON")
	releaseScorePath := flag.String("release-score", "", "optional path to write release verification score text")
	releaseScoreJSONPath := flag.String("release-score-json", "", "optional path to write release verification score JSON")
	releaseGatePath := flag.String("release-gate", "", "optional path to write release verification gate text")
	releaseGateJSONPath := flag.String("release-gate-json", "", "optional path to write release verification gate JSON")
	releaseAuditPath := flag.String("release-audit", "", "optional path to write combined release audit text")
	releaseAuditJSONPath := flag.String("release-audit-json", "", "optional path to write combined release audit JSON")
	releaseAuditHTMLPath := flag.String("release-audit-html", "", "optional path to write combined release audit HTML")
	releaseAuditBaselinePath := flag.String("release-audit-baseline", "", "optional baseline release audit JSON for audit diff")
	releaseAuditDiffPath := flag.String("release-audit-diff", "", "optional path to write release audit diff text")
	releaseAuditDiffJSONPath := flag.String("release-audit-diff-json", "", "optional path to write release audit diff JSON")
	releaseWaiversPath := flag.String("release-waivers", "", "optional release waiver rules JSON")
	printReleaseWaiverTemplate := flag.Bool("print-release-waiver-template", false, "print a release waiver template JSON and exit")
	releaseWaiverReportPath := flag.String("release-waiver-report", "", "optional path to write release waiver report text")
	releaseWaiverReportJSONPath := flag.String("release-waiver-report-json", "", "optional path to write release waiver report JSON")
	releaseTodoPath := flag.String("release-todo", "", "optional path to write release audit TODO markdown")
	releaseTodoJSONPath := flag.String("release-todo-json", "", "optional path to write release audit TODO JSON")
	releaseAuditNavHTMLPath := flag.String("release-audit-nav-html", "", "optional path to write extended navigable release audit HTML")
	waiverCalendarPath := flag.String("waiver-calendar", "", "optional path to write waiver expiry calendar text")
	waiverCalendarJSONPath := flag.String("waiver-calendar-json", "", "optional path to write waiver expiry calendar JSON")
	waiverCalendarHTMLPath := flag.String("waiver-calendar-html", "", "optional path to write waiver expiry calendar HTML")
	releaseChangelogPath := flag.String("release-changelog", "", "optional path to write release audit changelog Markdown")
	releaseChangelogJSONPath := flag.String("release-changelog-json", "", "optional path to write release audit changelog JSON")
	finalDecisionPath := flag.String("final-decision", "", "optional path to write final release decision text")
	finalDecisionJSONPath := flag.String("final-decision-json", "", "optional path to write final release decision JSON")
	releaseEvidenceZipPath := flag.String("release-evidence-zip", "", "optional path to write release evidence bundle ZIP")
	inspectReleaseEvidenceZipPath := flag.String("inspect-release-evidence-zip", "", "optional path to inspect release evidence bundle ZIP")
	releaseEvidenceInspectJSONPath := flag.String("release-evidence-inspect-json", "", "optional path to write release evidence bundle inspection JSON")
	dryRun := flag.Bool("dry-run", false, "compute reports without writing optional output files")
	exitCodeMode := flag.String("exit-code-mode", "on-fail", "exit behavior: on-fail or never")
	junitPath := flag.String("junit", "", "optional path to write JUnit XML")
	htmlPath := flag.String("html", "", "optional path to write self-contained HTML report")
	sarifPath := flag.String("sarif", "", "optional path to write SARIF report")
	steps := flag.Int("steps", 200000, "intended smoke steps")
	presetsText := flag.String("presets", "uart-blk,hvc-blk,uart-initrd,hvc-initrd,simplefb", "comma-separated smoke preset names")
	var artifacts artifactFlags
	var compareBundles repeatedStrings
	var matrixResults repeatedStrings
	flag.Var(&artifacts, "artifact", "verify loaded artifact bytes, role=/path/to/file; repeatable")
	flag.Var(&compareBundles, "compare", "additional bundle for trend compare, path or name=path; repeatable")
	flag.Var(&matrixResults, "matrix-result", "rvsmoke -out json result for matrix aggregation, path or name=path; repeatable")
	flag.Parse()
	globalDryRun = *dryRun
	releaseAuditRequested := releaseAuditOutputRequested(
		*releaseVerificationHTMLPath,
		*releaseScorePath, *releaseScoreJSONPath,
		*releaseGatePath, *releaseGateJSONPath,
		*releaseAuditPath, *releaseAuditJSONPath, *releaseAuditHTMLPath,
		*releaseAuditBaselinePath, *releaseAuditDiffPath, *releaseAuditDiffJSONPath,
		*releaseWaiversPath, *releaseWaiverReportPath, *releaseWaiverReportJSONPath,
		*releaseTodoPath, *releaseTodoJSONPath, *releaseAuditNavHTMLPath,
		*waiverCalendarPath, *waiverCalendarJSONPath, *waiverCalendarHTMLPath,
		*releaseChangelogPath, *releaseChangelogJSONPath,
		*finalDecisionPath, *finalDecisionJSONPath,
		*releaseEvidenceZipPath, *inspectReleaseEvidenceZipPath, *releaseEvidenceInspectJSONPath,
	)
	if *listPolicies {
		fmt.Print(analyze.CIGatePolicyTemplateListString())
		return
	}
	if strings.TrimSpace(*printPolicy) != "" {
		fmt.Println(analyze.CIGatePolicyTemplateJSON(*printPolicy))
		return
	}
	if *listReleaseVerifyPolicies {
		fmt.Print(analyze.ReleaseVerificationGatePolicyTemplateListString())
		return
	}
	if strings.TrimSpace(*printReleaseVerifyPolicy) != "" {
		fmt.Println(analyze.ReleaseVerificationGatePolicyTemplateJSON(*printReleaseVerifyPolicy))
		return
	}
	if strings.TrimSpace(*printGithubActions) != "" {
		fmt.Print(analyze.GitHubActionsWorkflowYAML(*printGithubActions))
		return
	}
	if strings.TrimSpace(*printGithubActionsMatrix) != "" {
		fmt.Print(analyze.GitHubActionsMatrixWorkflowYAML(*printGithubActionsMatrix, splitCSV(*presetsText)))
		return
	}
	if *printReleaseWaiverTemplate {
		fmt.Println(analyze.ReleaseWaiverTemplateJSON())
		return
	}
	if strings.TrimSpace(*inspectReleaseZipPath) != "" && strings.TrimSpace(*bundlePath) == "" && strings.TrimSpace(*manifestPath) == "" {
		insp := inspectReleaseZipOrExit(*inspectReleaseZipPath)
		writeOptional(*releaseZipInspectHTMLPath, analyze.ReleaseHandoffPackageHTML(insp))
		if strings.EqualFold(*out, "json") {
			fmt.Println(analyze.ReleaseHandoffPackageInspectionJSON(insp))
		} else if strings.EqualFold(*out, "html") {
			fmt.Print(analyze.ReleaseHandoffPackageHTML(insp))
		} else {
			fmt.Print(analyze.ReleaseHandoffPackageInspectionString(insp))
		}
		if insp.Status == "fail" {
			os.Exit(1)
		}
		return
	}

	if strings.TrimSpace(*inspectReleaseEvidenceZipPath) != "" && strings.TrimSpace(*bundlePath) == "" && strings.TrimSpace(*manifestPath) == "" {
		insp := inspectReleaseEvidenceZipOrExit(*inspectReleaseEvidenceZipPath)
		writeOptional(*releaseEvidenceInspectJSONPath, analyze.ReleaseEvidenceBundleInspectionJSON(insp))
		if strings.EqualFold(*out, "json") {
			fmt.Println(analyze.ReleaseEvidenceBundleInspectionJSON(insp))
		} else {
			fmt.Print(analyze.ReleaseEvidenceBundleInspectionString(insp))
		}
		if insp.Status == "fail" {
			os.Exit(1)
		}
		return
	}

	if strings.TrimSpace(*inspectReproZipPath) != "" && strings.TrimSpace(*bundlePath) == "" && strings.TrimSpace(*manifestPath) == "" {
		insp := inspectReproZipOrExit(*inspectReproZipPath)
		checksums := analyze.BuildReproZipChecksumManifest(insp)
		writeOptional(*reproChecksumsPath, analyze.ReproZipChecksumManifestJSON(checksums))
		if strings.EqualFold(*out, "json") {
			payload := map[string]any{"inspection": insp, "checksums": checksums}
			b, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Print(analyze.InspectMinimalReproZipString(insp))
			fmt.Print("\n")
			fmt.Print(analyze.ReproZipChecksumManifestString(checksums))
		}
		if insp.Status == "fail" {
			os.Exit(1)
		}
		return
	}

	bundle, raw, err := loadBundleOrManifest(*bundlePath, *manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: %v\n", err)
		os.Exit(2)
	}
	trace := mustReadOptional(*tracePath)
	console := mustReadOptional(*consolePath)
	integrity := analyze.BuildBundleIntegrityReport(bundle, raw)
	sig := analyze.BuildLogSignatureSet(trace, console, bundle.Manifest)
	ci := analyze.BuildCISummary(bundle, sig, integrity)
	checks := verifyArtifacts(artifacts, bundle.Manifest)
	for _, c := range checks {
		if !c.LooksValid {
			ci.Status = "fail"
			ci.ExitCode = 1
			ci.Messages = append(ci.Messages, "artifact hash mismatch: "+c.Role)
		}
	}
	baselineText := mustReadOptional(*baselinePath)
	bundleDiff := analyze.CompareDiagnosticBundles(bundle, baselineText)
	policy := loadPolicy(*policyPath, *policyTemplate)
	gate := analyze.BuildCIGateReport(bundle, integrity, sig, bundleDiff, policy)
	checklist := analyze.BuildCIActionChecklist(gate, integrity, bundleDiff)
	if gate.ExitCode != 0 {
		ci.Status = "fail"
		ci.ExitCode = gate.ExitCode
		ci.Messages = append(ci.Messages, gate.Summary...)
	}
	spec := analyze.BuildHeadlessSmokeRunnerSpec(bundle, splitCSV(*presetsText), *steps)
	trend := buildTrend(compareBundles, bundle, raw)
	policyTree := analyze.BuildPolicyViolationTree(gate, integrity, bundleDiff, trend)
	history := analyze.BuildBundleHistoryAggregate(trend)
	reproPackage := analyze.BuildMinimalReproPackageSpec(bundle, spec)
	reproInspection := analyze.ReproZipInspection{}
	if strings.TrimSpace(*inspectReproZipPath) != "" {
		reproInspection = inspectReproZipOrExit(*inspectReproZipPath)
		if reproInspection.Status == "fail" {
			ci.Status = "fail"
			ci.ExitCode = 1
			ci.Messages = append(ci.Messages, "minimal repro zip inspection failed")
		}
	}
	trendChart := analyze.BuildBundleTrendChartData(trend)
	matrixAggregate := buildMatrixAggregate(matrixResults)
	matrixFlakes := analyze.BuildMatrixFlakeReport(matrixAggregate)
	writeOptional(*trendCSVPath, analyze.BundleTrendCSV(trend))
	writeOptional(*trendChartPath, analyze.BundleTrendChartDataJSON(trendChart))
	if len(trend.Rows) != 0 {
		writeOptional(*trendHTMLPath, analyze.BundleTrendReportStandaloneHTML(trend))
	}
	writeOptional(*githubActionsMatrixPath, analyze.GitHubActionsMatrixWorkflowYAML(*policyTemplate, splitCSV(*presetsText)))
	if len(matrixAggregate.Rows) != 0 {
		writeOptional(*matrixSummaryPath, analyze.MatrixResultAggregateString(matrixAggregate))
		writeOptional(*matrixSummaryJSONPath, analyze.MatrixResultAggregateJSON(matrixAggregate))
		writeOptional(*matrixSummaryHTMLPath, analyze.MatrixResultAggregateHTML(matrixAggregate))
		writeOptional(*matrixFlakesPath, analyze.MatrixFlakeReportString(matrixFlakes))
		writeOptional(*matrixFlakesJSONPath, analyze.MatrixFlakeReportJSON(matrixFlakes))
		writeOptional(*matrixFlakesHTMLPath, analyze.MatrixFlakeReportHTML(matrixFlakes))
	}
	reproChecksums := analyze.ReproZipChecksumManifest{}
	reproVerify := analyze.ReproZipChecksumVerification{}
	if reproInspection.Status != "" {
		reproChecksums = analyze.BuildReproZipChecksumManifest(reproInspection)
		writeOptional(*reproChecksumsPath, analyze.ReproZipChecksumManifestJSON(reproChecksums))
		if strings.TrimSpace(*verifyReproChecksumsPath) != "" {
			reproVerify = analyze.VerifyReproZipChecksumManifest(reproChecksums, mustReadOptional(*verifyReproChecksumsPath))
			writeOptional(*checksumVerifyPath, analyze.ReproZipChecksumVerificationString(reproVerify))
			writeOptional(*checksumVerifyJSONPath, analyze.ReproZipChecksumVerificationJSON(reproVerify))
			if reproVerify.Status == "fail" {
				ci.Status = "fail"
				ci.ExitCode = 1
				ci.Messages = append(ci.Messages, "repro checksum verification failed")
			}
		}
	}
	junitXML := analyze.CIJUnitXML(gate, integrity, checks, bundleDiff)
	htmlReport := analyze.CIHTMLReport(ci, gate, integrity, sig, bundleDiff, checks)
	if len(trend.Rows) != 0 {
		htmlReport += "\n<hr>\n" + analyze.BundleTrendReportHTML(trend)
	}
	sarifReport := analyze.CISARIFReport(integrity, gate)
	writeOptional(*junitPath, junitXML)
	writeOptional(*htmlPath, htmlReport)
	writeOptional(*sarifPath, sarifReport)
	writeOptional(*githubActionsPath, analyze.GitHubActionsWorkflowYAML(*policyTemplate))
	writeOptional(*policyTreePath, analyze.PolicyViolationTreeMarkdown(policyTree))
	writeOptional(*historyPath, analyze.BundleHistoryAggregateString(history))
	if strings.TrimSpace(*reproZipPath) != "" {
		files := analyze.MinimalReproPackageFiles(bundle, raw, ci, gate, spec, policy, policyTree, history)
		writeZip(*reproZipPath, files)
	}
	artifactIndex := buildCIArtifactIndexFromPaths(map[string]string{
		"junit": *junitPath, "html": *htmlPath, "sarif": *sarifPath, "github-actions": *githubActionsPath, "github-actions-matrix": *githubActionsMatrixPath,
		"policy-tree": *policyTreePath, "history": *historyPath, "repro-zip": *reproZipPath, "trend-csv": *trendCSVPath, "trend-chart": *trendChartPath, "trend-html": *trendHTMLPath,
		"matrix-summary": *matrixSummaryPath, "matrix-summary-json": *matrixSummaryJSONPath, "matrix-summary-html": *matrixSummaryHTMLPath, "matrix-flakes": *matrixFlakesPath, "matrix-flakes-json": *matrixFlakesJSONPath, "matrix-flakes-html": *matrixFlakesHTMLPath,
		"repro-checksums": *reproChecksumsPath, "checksum-verify": *checksumVerifyPath, "checksum-verify-json": *checksumVerifyJSONPath,
		"sbom": *sbomPath, "sbom-text": *sbomTextPath, "sbom-diff": *sbomDiffPath, "attestation": *attestationPath, "attestation-text": *attestationTextPath, "attestation-verification": *verifyAttestationPath, "release-zip": *releaseZipPath, "release-zip-inspect-html": *releaseZipInspectHTMLPath, "release-zip-compare": *releaseZipComparePath, "retention-manifest": *retentionManifestPath, "retention-audit": *retentionAuditPath, "release-score": *releaseScorePath, "release-gate": *releaseGatePath, "release-audit": *releaseAuditPath, "release-verification-html": *releaseVerificationHTMLPath, "release-audit-html": *releaseAuditHTMLPath, "release-audit-nav-html": *releaseAuditNavHTMLPath, "release-audit-diff": *releaseAuditDiffPath, "release-audit-diff-json": *releaseAuditDiffJSONPath, "release-waiver-report": *releaseWaiverReportPath, "release-waiver-report-json": *releaseWaiverReportJSONPath, "release-todo": *releaseTodoPath, "release-todo-json": *releaseTodoJSONPath, "waiver-calendar": *waiverCalendarPath, "waiver-calendar-json": *waiverCalendarJSONPath, "waiver-calendar-html": *waiverCalendarHTMLPath, "release-changelog": *releaseChangelogPath, "release-changelog-json": *releaseChangelogJSONPath, "final-decision": *finalDecisionPath, "final-decision-json": *finalDecisionJSONPath, "release-evidence-zip": *releaseEvidenceZipPath, "release-evidence-inspect-json": *releaseEvidenceInspectJSONPath,
	})
	writeOptional(*artifactIndexPath, analyze.CIArtifactIndexJSON(artifactIndex))
	releaseManifest := analyze.BuildReleaseBundleManifest(bundle, raw, sig, ci, gate, artifactIndex, matrixAggregate, matrixFlakes, reproChecksums, reproVerify)
	writeOptional(*releaseManifestPath, analyze.ReleaseBundleManifestJSON(releaseManifest))
	writeOptional(*releaseHTMLPath, analyze.ReleaseBundleManifestHTML(releaseManifest, matrixAggregate, matrixFlakes))
	goModText := mustReadOptionalSoft(*goModPath)
	sbom := analyze.BuildDependencyInventory(goModText, artifactIndex)
	writeOptional(*sbomPath, analyze.DependencyInventoryJSON(sbom))
	writeOptional(*sbomTextPath, analyze.DependencyInventoryString(sbom))
	attestation := analyze.BuildProvenanceAttestation(releaseManifest, sbom, artifactIndex, "")
	writeOptional(*attestationPath, analyze.ProvenanceAttestationJSON(attestation))
	writeOptional(*attestationTextPath, analyze.ProvenanceAttestationString(attestation))
	attestationVerification := analyze.VerifyProvenanceAttestation(attestation, releaseManifest, sbom, artifactIndex)
	writeOptional(*verifyAttestationPath, analyze.AttestationVerificationJSON(attestationVerification))
	writeOptional(*verifyAttestationTextPath, analyze.AttestationVerificationString(attestationVerification))
	sbomDiff := analyze.CompareDependencyInventory(sbom, mustReadOptional(*sbomBaselinePath))
	writeOptional(*sbomDiffPath, analyze.DependencyInventoryDiffString(sbomDiff))
	writeOptional(*sbomDiffJSONPath, analyze.DependencyInventoryDiffJSON(sbomDiff))
	if strings.TrimSpace(*releaseZipPath) != "" {
		writeZip(*releaseZipPath, analyze.ReleaseHandoffPackageFiles(releaseManifest, artifactIndex, sbom, attestation))
	}
	releaseZipInspection := analyze.ReleaseHandoffPackageInspection{}
	if strings.TrimSpace(*inspectReleaseZipPath) != "" {
		releaseZipInspection = inspectReleaseZipOrExit(*inspectReleaseZipPath)
		writeOptional(*releaseZipInspectHTMLPath, analyze.ReleaseHandoffPackageHTML(releaseZipInspection))
		if releaseZipInspection.Status == "fail" {
			ci.Status = "fail"
			ci.ExitCode = 1
			ci.Messages = append(ci.Messages, "release handoff zip inspection failed")
		}
	} else if strings.TrimSpace(*releaseZipPath) != "" {
		releaseZipInspection = inspectReleaseZipOrExit(*releaseZipPath)
	}
	releaseZipCompare := analyze.ReleaseHandoffPackageComparison{}
	if strings.TrimSpace(*compareReleaseZipInspectionPath) != "" && releaseZipInspection.Status != "" {
		releaseZipCompare = analyze.CompareReleaseHandoffPackageInspection(releaseZipInspection, mustReadOptional(*compareReleaseZipInspectionPath))
		writeOptional(*releaseZipComparePath, analyze.ReleaseHandoffPackageComparisonString(releaseZipCompare))
		writeOptional(*releaseZipCompareJSONPath, analyze.ReleaseHandoffPackageComparisonJSON(releaseZipCompare))
		if releaseZipCompare.Status == "fail" {
			ci.Status = "fail"
			ci.ExitCode = 1
			ci.Messages = append(ci.Messages, "release handoff zip comparison failed")
		}
	}
	retention := analyze.BuildRetentionManifest(artifactIndex, releaseManifest, "")
	writeOptional(*retentionManifestPath, analyze.RetentionManifestJSON(retention))
	writeOptional(*retentionTextPath, analyze.RetentionManifestString(retention))
	releaseVerifyPolicy := loadReleaseVerifyPolicy(*releaseVerifyPolicyPath, *releaseVerifyTemplate)
	retentionAudit := analyze.BuildRetentionAudit(retention, "", releaseVerifyPolicy.MinArtifactRetentionDays, releaseVerifyPolicy.ExpiringSoonDays)
	releaseScore := analyze.BuildReleaseVerificationScore(releaseManifest, attestationVerification, sbomDiff, releaseZipCompare, retentionAudit, matrixFlakes)
	releaseVerifyGate := analyze.BuildReleaseVerificationGateReport(releaseManifest, attestationVerification, sbomDiff, releaseZipCompare, retentionAudit, releaseScore, releaseVerifyPolicy)
	releaseAudit := analyze.BuildReleaseAuditReport(releaseManifest, attestationVerification, sbomDiff, releaseZipCompare, retention, matrixFlakes, releaseVerifyPolicy, "")
	releaseAuditDiff := analyze.BuildReleaseAuditDiff(releaseAudit, mustReadOptional(*releaseAuditBaselinePath))
	releaseWaivers := analyze.BuildReleaseWaiverReport(releaseAudit, mustReadOptional(*releaseWaiversPath), "")
	releaseTodos := analyze.BuildReleaseAuditTodoReport(releaseAudit, releaseWaivers)
	writeOptional(*retentionAuditPath, analyze.RetentionAuditString(retentionAudit))
	writeOptional(*retentionAuditJSONPath, analyze.RetentionAuditJSON(retentionAudit))
	writeOptional(*releaseScorePath, analyze.ReleaseVerificationScoreString(releaseScore))
	writeOptional(*releaseScoreJSONPath, analyze.ReleaseVerificationScoreJSON(releaseScore))
	writeOptional(*releaseGatePath, analyze.ReleaseVerificationGateReportString(releaseVerifyGate))
	writeOptional(*releaseGateJSONPath, analyze.ReleaseVerificationGateReportJSON(releaseVerifyGate))
	writeOptional(*releaseAuditPath, analyze.ReleaseAuditReportString(releaseAudit))
	writeOptional(*releaseAuditJSONPath, analyze.ReleaseAuditReportJSON(releaseAudit))
	writeOptional(*releaseAuditHTMLPath, analyze.ReleaseAuditReportHTML(releaseAudit))
	writeOptional(*releaseAuditDiffPath, analyze.ReleaseAuditDiffString(releaseAuditDiff))
	writeOptional(*releaseAuditDiffJSONPath, analyze.ReleaseAuditDiffJSON(releaseAuditDiff))
	writeOptional(*releaseWaiverReportPath, analyze.ReleaseWaiverReportString(releaseWaivers))
	writeOptional(*releaseWaiverReportJSONPath, analyze.ReleaseWaiverReportJSON(releaseWaivers))
	writeOptional(*releaseTodoPath, analyze.ReleaseAuditTodoReportMarkdown(releaseTodos))
	writeOptional(*releaseTodoJSONPath, analyze.ReleaseAuditTodoReportJSON(releaseTodos))
	waiverCalendar := analyze.BuildWaiverExpiryCalendar(mustReadOptional(*releaseWaiversPath), releaseWaivers, "", releaseVerifyPolicy.ExpiringSoonDays)
	releaseChangelog := analyze.BuildReleaseAuditChangelog(releaseAuditDiff, releaseWaivers, releaseTodos, waiverCalendar)
	finalDecision := analyze.BuildFinalReleaseDecision(releaseAudit, releaseWaivers, releaseTodos, waiverCalendar, releaseVerifyPolicy)
	writeOptional(*waiverCalendarPath, analyze.WaiverExpiryCalendarString(waiverCalendar))
	writeOptional(*waiverCalendarJSONPath, analyze.WaiverExpiryCalendarJSON(waiverCalendar))
	writeOptional(*waiverCalendarHTMLPath, analyze.WaiverExpiryCalendarHTML(waiverCalendar))
	writeOptional(*releaseChangelogPath, analyze.ReleaseAuditChangelogMarkdown(releaseChangelog))
	writeOptional(*releaseChangelogJSONPath, analyze.ReleaseAuditChangelogJSON(releaseChangelog))
	writeOptional(*finalDecisionPath, analyze.FinalReleaseDecisionString(finalDecision))
	writeOptional(*finalDecisionJSONPath, analyze.FinalReleaseDecisionJSON(finalDecision))
	releaseEvidenceInspection := analyze.ReleaseEvidenceBundleInspection{}
	if strings.TrimSpace(*releaseEvidenceZipPath) != "" {
		writeZip(*releaseEvidenceZipPath, analyze.ReleaseEvidenceBundleFiles(releaseAudit, releaseAuditDiff, releaseWaivers, releaseTodos, waiverCalendar, releaseChangelog, finalDecision))
		if !globalDryRun {
			releaseEvidenceInspection = inspectReleaseEvidenceZipOrExit(*releaseEvidenceZipPath)
		}
	}
	if strings.TrimSpace(*inspectReleaseEvidenceZipPath) != "" {
		releaseEvidenceInspection = inspectReleaseEvidenceZipOrExit(*inspectReleaseEvidenceZipPath)
		if releaseEvidenceInspection.Status == "fail" {
			ci.Status = "fail"
			ci.ExitCode = 1
			ci.Messages = append(ci.Messages, "release evidence bundle inspection failed")
		}
	}
	writeOptional(*releaseEvidenceInspectJSONPath, analyze.ReleaseEvidenceBundleInspectionJSON(releaseEvidenceInspection))
	writeOptional(*releaseAuditNavHTMLPath, analyze.ReleaseAuditExtendedHTML(releaseAudit, releaseAuditDiff, releaseWaivers, releaseTodos))
	writeOptional(*releaseVerificationHTMLPath, analyze.ReleaseVerificationHTML(releaseManifest, attestationVerification, sbomDiff, releaseZipCompare, retention))
	if releaseAuditRequested && releaseVerifyGate.ExitCode != 0 {
		ci.Status = "fail"
		ci.ExitCode = releaseVerifyGate.ExitCode
		ci.Messages = append(ci.Messages, releaseVerifyGate.Summary...)
	}
	payload := cliOutput{Integrity: integrity, Signature: sig, CI: ci, Gate: gate, Checklist: checklist, Runner: spec, PolicyTree: policyTree, History: history, ReproPackage: reproPackage, ReproZip: reproInspection, TrendChart: trendChart, Matrix: matrixAggregate, MatrixFlakes: matrixFlakes, ReproChecksums: reproChecksums, ReproVerify: reproVerify, ArtifactIndex: artifactIndex, Release: releaseManifest, SBOM: sbom, Attestation: attestation, ReleaseZip: releaseZipInspection, AttestationVerification: attestationVerification, SBOMDiff: sbomDiff, ReleaseZipCompare: releaseZipCompare, Retention: retention, RetentionAudit: retentionAudit, ReleaseScore: releaseScore, ReleaseVerifyGate: releaseVerifyGate, ReleaseAudit: releaseAudit, ReleaseAuditDiff: releaseAuditDiff, ReleaseWaivers: releaseWaivers, ReleaseTodos: releaseTodos, WaiverCalendar: waiverCalendar, ReleaseChangelog: releaseChangelog, FinalDecision: finalDecision, EvidenceBundle: releaseEvidenceInspection, BundleDiff: bundleDiff, Trend: trend, ArtifactChecks: checks, showReleaseAudit: releaseAuditRequested}

	switch strings.ToLower(*out) {
	case "json":
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(b))
	case "md", "markdown":
		fmt.Print(markdown(payload))
	case "html":
		fmt.Print(htmlReport)
	case "junit":
		fmt.Print(junitXML)
	case "sarif":
		fmt.Print(sarifReport)
	default:
		fmt.Print(analyze.BundleIntegrityReportString(integrity))
		fmt.Print("\n")
		fmt.Print(analyze.CISummaryString(ci))
		fmt.Print("\n")
		fmt.Print(analyze.CIGateReportString(gate))
		if len(bundleDiff) != 0 {
			fmt.Print("\n")
			fmt.Print(analyze.DiagnosticBundleDiffString(bundleDiff))
		}
		fmt.Print("\n")
		fmt.Print(analyze.CIActionChecklistString(checklist))
		if len(trend.Rows) != 0 {
			fmt.Print("\n")
			fmt.Print(analyze.BundleTrendReportString(trend))
			fmt.Print("\n")
			fmt.Print(analyze.BundleHistoryAggregateString(history))
		}
		if len(policyTree.Nodes) != 0 {
			fmt.Print("\n")
			fmt.Print(analyze.PolicyViolationTreeString(policyTree))
		}
		if reproInspection.Status != "" {
			fmt.Print("\n")
			fmt.Print(analyze.InspectMinimalReproZipString(reproInspection))
			if reproChecksums.FileCount != 0 {
				fmt.Print("\n")
				fmt.Print(analyze.ReproZipChecksumManifestString(reproChecksums))
			}
		}
		if len(matrixAggregate.Rows) != 0 {
			fmt.Print("\n")
			fmt.Print(analyze.MatrixResultAggregateString(matrixAggregate))
			fmt.Print("\n")
			fmt.Print(analyze.MatrixFlakeReportString(matrixFlakes))
		}
		if reproVerify.Status != "" {
			fmt.Print("\n")
			fmt.Print(analyze.ReproZipChecksumVerificationString(reproVerify))
		}
		if artifactIndex.FileCount != 0 {
			fmt.Print("\n")
			fmt.Print(analyze.CIArtifactIndexString(artifactIndex))
		}
		if releaseManifest.Status != "" {
			fmt.Print("\n")
			fmt.Print(analyze.ReleaseBundleManifestString(releaseManifest))
		}
		if sbom.SchemaVersion != "" {
			fmt.Print("\n")
			fmt.Print(analyze.DependencyInventoryString(sbom))
		}
		if attestation.SchemaVersion != "" {
			fmt.Print("\n")
			fmt.Print(analyze.ProvenanceAttestationString(attestation))
		}
		if releaseZipInspection.Status != "" {
			fmt.Print("\n")
			fmt.Print(analyze.ReleaseHandoffPackageInspectionString(releaseZipInspection))
		}
		if attestationVerification.Status != "" {
			fmt.Print("\n")
			fmt.Print(analyze.AttestationVerificationString(attestationVerification))
		}
		if sbomDiff.Status != "" {
			fmt.Print("\n")
			fmt.Print(analyze.DependencyInventoryDiffString(sbomDiff))
		}
		if releaseZipCompare.Status != "" {
			fmt.Print("\n")
			fmt.Print(analyze.ReleaseHandoffPackageComparisonString(releaseZipCompare))
		}
		if retention.SchemaVersion != "" {
			fmt.Print("\n")
			fmt.Print(analyze.RetentionManifestString(retention))
		}
		if payload.showReleaseAudit && releaseAudit.SchemaVersion != "" {
			fmt.Print("\n")
			fmt.Print(analyze.ReleaseAuditReportString(releaseAudit))
			fmt.Print("\n")
			fmt.Print(analyze.ReleaseAuditDiffString(releaseAuditDiff))
			fmt.Print("\n")
			fmt.Print(analyze.ReleaseWaiverReportString(releaseWaivers))
			fmt.Print("\n")
			fmt.Print(analyze.ReleaseAuditTodoReportString(releaseTodos))
			fmt.Print("\n")
			fmt.Print(analyze.WaiverExpiryCalendarString(waiverCalendar))
			fmt.Print("\n")
			fmt.Print(analyze.ReleaseAuditChangelogString(releaseChangelog))
			fmt.Print("\n")
			fmt.Print(analyze.FinalReleaseDecisionString(finalDecision))
			if releaseEvidenceInspection.Status != "" {
				fmt.Print("\n")
				fmt.Print(analyze.ReleaseEvidenceBundleInspectionString(releaseEvidenceInspection))
			}
		}
		if len(checks) != 0 {
			fmt.Print("artifact file checks:\n")
			for _, c := range checks {
				state := "mismatch"
				if c.LooksValid {
					state = "ok"
				}
				fmt.Printf("  %-10s %-8s bytes=%d sha=%s\n", c.Role, state, c.Bytes, c.SHA256)
			}
		}
		fmt.Print("\n")
		fmt.Print(analyze.HeadlessSmokeRunnerSpecString(spec))
	}
	if ci.ExitCode != 0 && !strings.EqualFold(*exitCodeMode, "never") {
		os.Exit(ci.ExitCode)
	}
}

func loadBundleOrManifest(bundlePath, manifestPath string) (analyze.DiagnosticBundle, string, error) {
	if bundlePath != "" {
		data, err := os.ReadFile(bundlePath)
		if err != nil {
			return analyze.DiagnosticBundle{}, "", err
		}
		b, raw, err := analyze.ParseDiagnosticBundleText(string(data))
		return b, raw, err
	}
	if manifestPath != "" {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return analyze.DiagnosticBundle{}, "", err
		}
		var m analyze.ArtifactManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return analyze.DiagnosticBundle{}, "", err
		}
		b := analyze.DiagnosticBundle{Manifest: m}
		return b, string(data), nil
	}
	return analyze.DiagnosticBundle{}, "", fmt.Errorf("provide -bundle or -manifest")
}

func mustReadOptional(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot read %s: %v\n", path, err)
		os.Exit(2)
	}
	return string(data)
}

func mustReadOptionalSoft(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func inspectReleaseEvidenceZipOrExit(path string) analyze.ReleaseEvidenceBundleInspection {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot read release evidence zip %s: %v\n", path, err)
		os.Exit(2)
	}
	insp, err := analyze.InspectReleaseEvidenceBundleZipBytes(data)
	if err != nil && insp.Status == "" {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot inspect release evidence zip %s: %v\n", path, err)
		os.Exit(2)
	}
	return insp
}

func inspectReproZipOrExit(path string) analyze.ReproZipInspection {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot read repro zip %s: %v\n", path, err)
		os.Exit(2)
	}
	insp, err := analyze.InspectMinimalReproZipBytes(data)
	if err != nil && insp.Status == "" {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot inspect repro zip %s: %v\n", path, err)
		os.Exit(2)
	}
	return insp
}

func inspectReleaseZipOrExit(path string) analyze.ReleaseHandoffPackageInspection {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot read release zip %s: %v\n", path, err)
		os.Exit(2)
	}
	insp, err := analyze.InspectReleaseHandoffZipBytes(data)
	if err != nil && insp.Status == "" {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot inspect release zip %s: %v\n", path, err)
		os.Exit(2)
	}
	return insp
}

func verifyArtifacts(paths map[string]string, manifest analyze.ArtifactManifest) []analyze.ArtifactIntegrityRow {
	rows := []analyze.ArtifactIntegrityRow{}
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, role := range keys {
		data, err := os.ReadFile(paths[role])
		if err != nil {
			rows = append(rows, analyze.ArtifactIntegrityRow{Role: role, LooksValid: false, SHA256: "read-error:" + err.Error()})
			continue
		}
		row, _ := analyze.VerifyArtifactBytes(role, data, manifest)
		rows = append(rows, row)
	}
	return rows
}

func splitCSV(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func loadPolicy(path, template string) analyze.CIGatePolicy {
	def := analyze.DefaultCIGatePolicy()
	if t, ok := analyze.CIGatePolicyTemplateByName(template); ok {
		def = t.Policy
	}
	if path == "" {
		return def
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot read policy %s: %v\n", path, err)
		os.Exit(2)
	}
	p, err := analyze.ParseCIGatePolicyJSON(string(data), def)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: bad policy %s: %v\n", path, err)
		os.Exit(2)
	}
	return p
}

func loadReleaseVerifyPolicy(path, template string) analyze.ReleaseVerificationGatePolicy {
	def := analyze.DefaultReleaseVerificationGatePolicy()
	if t, ok := analyze.ReleaseVerificationGatePolicyTemplateByName(template); ok {
		def = t.Policy
	}
	if path == "" {
		return def
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot read release verification policy %s: %v\n", path, err)
		os.Exit(2)
	}
	p, err := analyze.ParseReleaseVerificationGatePolicyJSON(string(data), def)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: bad release verification policy %s: %v\n", path, err)
		os.Exit(2)
	}
	return p
}

func writeOptional(path, text string) {
	if path == "" {
		return
	}
	if globalDryRun {
		return
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot write %s: %v\n", path, err)
		os.Exit(2)
	}
}

func writeZip(path string, files map[string]string) {
	if globalDryRun {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot create %s: %v\n", path, err)
		os.Exit(2)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w, err := zw.Create(k)
		if err != nil {
			_ = zw.Close()
			fmt.Fprintf(os.Stderr, "rvsmoke: cannot add %s to %s: %v\n", k, path, err)
			os.Exit(2)
		}
		if _, err := w.Write([]byte(files[k])); err != nil {
			_ = zw.Close()
			fmt.Fprintf(os.Stderr, "rvsmoke: cannot write %s in %s: %v\n", k, path, err)
			os.Exit(2)
		}
	}
	if err := zw.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "rvsmoke: cannot finish %s: %v\n", path, err)
		os.Exit(2)
	}
}

func buildCIArtifactIndexFromPaths(paths map[string]string) analyze.CIArtifactIndex {
	inputs := []analyze.CIArtifactInput{}
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, kind := range keys {
		path := strings.TrimSpace(paths[kind])
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			// Optional outputs may be produced later in the same rvsmoke run. Do not
			// index missing files as zero-byte artifacts; skipping them keeps the
			// artifact index from looking like it verified empty release assets.
			continue
		}
		inputs = append(inputs, analyze.CIArtifactInput{Path: path, Kind: kind, Data: data, Required: false})
	}
	return analyze.BuildCIArtifactIndex(inputs)
}

func buildTrend(paths []string, current analyze.DiagnosticBundle, raw string) analyze.BundleTrendReport {
	if len(paths) == 0 {
		return analyze.BundleTrendReport{}
	}
	items := make([]analyze.NamedDiagnosticBundle, 0, len(paths)+1)
	for i, spec := range paths {
		name, path, ok := strings.Cut(spec, "=")
		if !ok {
			path = spec
			name = analyze.DefaultCompareName(path, i)
		}
		data, err := os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			items = append(items, analyze.NamedDiagnosticBundle{Name: strings.TrimSpace(name), Bundle: analyze.DiagnosticBundle{Notes: []string{"compare read error: " + err.Error()}}, Raw: "{}"})
			continue
		}
		b, rawText, err := analyze.ParseDiagnosticBundleText(string(data))
		if err != nil {
			items = append(items, analyze.NamedDiagnosticBundle{Name: strings.TrimSpace(name), Bundle: analyze.DiagnosticBundle{Notes: []string{"compare parse error: " + err.Error()}}, Raw: string(data)})
			continue
		}
		items = append(items, analyze.NamedDiagnosticBundle{Name: strings.TrimSpace(name), Bundle: b, Raw: rawText})
	}
	items = append(items, analyze.NamedDiagnosticBundle{Name: "current", Bundle: current, Raw: raw})
	return analyze.BuildBundleTrendReport(items)
}

func buildMatrixAggregate(paths []string) analyze.MatrixResultAggregate {
	if len(paths) == 0 {
		return analyze.MatrixResultAggregate{}
	}
	inputs := make([]analyze.MatrixResultInput, 0, len(paths))
	for i, spec := range paths {
		name, path, ok := strings.Cut(spec, "=")
		if !ok {
			path = spec
			name = analyze.DefaultCompareName(path, i)
		}
		data, err := os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			inputs = append(inputs, analyze.MatrixResultInput{Name: strings.TrimSpace(name), JSON: `{"ci":{"status":"fail","exit_code":1},"gate":{"status":"fail","checks":[{"name":"read","status":"fail","detail":"` + strings.ReplaceAll(err.Error(), `"`, `'`) + `"}]}}`})
			continue
		}
		inputs = append(inputs, analyze.MatrixResultInput{Name: strings.TrimSpace(name), JSON: string(data)})
	}
	return analyze.BuildMatrixResultAggregate(inputs)
}

func markdown(p cliOutput) string {
	var b strings.Builder
	b.WriteString("# rvsmoke CI summary\n\n")
	b.WriteString("```text\n")
	b.WriteString(analyze.CISummaryString(p.CI))
	b.WriteString("```\n\n")
	b.WriteString("## Gate checks\n\n```text\n")
	b.WriteString(analyze.CIGateReportString(p.Gate))
	b.WriteString("```\n\n")
	b.WriteString(analyze.CIActionChecklistMarkdown(p.Checklist))
	b.WriteString("\n")
	b.WriteString("## Policy violation tree\n\n```text\n")
	b.WriteString(analyze.PolicyViolationTreeString(p.PolicyTree))
	b.WriteString("```\n\n")
	b.WriteString("## Bundle integrity\n\n```text\n")
	b.WriteString(analyze.BundleIntegrityReportString(p.Integrity))
	b.WriteString("```\n\n")
	if len(p.BundleDiff) != 0 {
		b.WriteString("## Baseline diff\n\n```text\n")
		b.WriteString(analyze.DiagnosticBundleDiffString(p.BundleDiff))
		b.WriteString("```\n\n")
	}
	if len(p.Trend.Rows) != 0 {
		b.WriteString(analyze.BundleTrendReportMarkdown(p.Trend))
		b.WriteString("\n")
		b.WriteString("## Bundle history aggregate\n\n```text\n")
		b.WriteString(analyze.BundleHistoryAggregateString(p.History))
		b.WriteString("```\n\n")
	}
	b.WriteString(analyze.HeadlessSmokeRunnerMarkdown(p.Runner))
	b.WriteString("\n")
	b.WriteString(analyze.MinimalReproPackageMarkdown(p.ReproPackage))
	if p.ReproZip.Status != "" {
		b.WriteString("\n## Minimal repro ZIP inspection\n\n```text\n")
		b.WriteString(analyze.InspectMinimalReproZipString(p.ReproZip))
		b.WriteString("```\n")
		if p.ReproChecksums.FileCount != 0 {
			b.WriteString("\n## Minimal repro checksum manifest\n\n```text\n")
			b.WriteString(analyze.ReproZipChecksumManifestString(p.ReproChecksums))
			b.WriteString("```\n")
		}
	}
	if len(p.Matrix.Rows) != 0 {
		b.WriteString("\n")
		b.WriteString(analyze.MatrixResultAggregateMarkdown(p.Matrix))
		b.WriteString("\n## Matrix flake report\n\n```text\n")
		b.WriteString(analyze.MatrixFlakeReportString(p.MatrixFlakes))
		b.WriteString("```\n")
	}
	if p.ReproVerify.Status != "" {
		b.WriteString("\n## Repro checksum verification\n\n```text\n")
		b.WriteString(analyze.ReproZipChecksumVerificationString(p.ReproVerify))
		b.WriteString("```\n")
	}
	if p.ArtifactIndex.FileCount != 0 {
		b.WriteString("\n## CI artifact index\n\n```text\n")
		b.WriteString(analyze.CIArtifactIndexString(p.ArtifactIndex))
		b.WriteString("```\n")
	}
	if p.Release.Status != "" {
		b.WriteString("\n## Release bundle manifest\n\n```text\n")
		b.WriteString(analyze.ReleaseBundleManifestString(p.Release))
		b.WriteString("```\n")
	}
	if p.SBOM.SchemaVersion != "" {
		b.WriteString("\n## Dependency inventory\n\n```text\n")
		b.WriteString(analyze.DependencyInventoryString(p.SBOM))
		b.WriteString("```\n")
	}
	if p.Attestation.SchemaVersion != "" {
		b.WriteString("\n## Provenance attestation\n\n```text\n")
		b.WriteString(analyze.ProvenanceAttestationString(p.Attestation))
		b.WriteString("```\n")
	}
	if p.ReleaseZip.Status != "" {
		b.WriteString("\n## Release handoff ZIP inspection\n\n```text\n")
		b.WriteString(analyze.ReleaseHandoffPackageInspectionString(p.ReleaseZip))
		b.WriteString("```\n")
	}
	if p.AttestationVerification.Status != "" {
		b.WriteString("\n## Attestation verification\n\n```text\n")
		b.WriteString(analyze.AttestationVerificationString(p.AttestationVerification))
		b.WriteString("```\n")
	}
	if p.SBOMDiff.Status != "" {
		b.WriteString("\n## Dependency inventory diff\n\n```text\n")
		b.WriteString(analyze.DependencyInventoryDiffString(p.SBOMDiff))
		b.WriteString("```\n")
	}
	if p.ReleaseZipCompare.Status != "" {
		b.WriteString("\n## Release handoff ZIP comparison\n\n```text\n")
		b.WriteString(analyze.ReleaseHandoffPackageComparisonString(p.ReleaseZipCompare))
		b.WriteString("```\n")
	}
	if p.Retention.SchemaVersion != "" {
		b.WriteString("\n## Retention manifest\n\n```text\n")
		b.WriteString(analyze.RetentionManifestString(p.Retention))
		b.WriteString("```\n")
	}
	if p.showReleaseAudit && p.ReleaseAudit.SchemaVersion != "" {
		b.WriteString("\n## Release audit\n\n```text\n")
		b.WriteString(analyze.ReleaseAuditReportString(p.ReleaseAudit))
		b.WriteString("```\n")
		b.WriteString("\n## Release audit diff\n\n```text\n")
		b.WriteString(analyze.ReleaseAuditDiffString(p.ReleaseAuditDiff))
		b.WriteString("```\n")
		b.WriteString("\n## Release waivers\n\n```text\n")
		b.WriteString(analyze.ReleaseWaiverReportString(p.ReleaseWaivers))
		b.WriteString("```\n")
		b.WriteString("\n## Waiver expiry calendar\n\n```text\n")
		b.WriteString(analyze.WaiverExpiryCalendarString(p.WaiverCalendar))
		b.WriteString("```\n")
		b.WriteString("\n## Release changelog\n\n")
		b.WriteString(analyze.ReleaseAuditChangelogMarkdown(p.ReleaseChangelog))
		b.WriteString("\n## Final release decision\n\n```text\n")
		b.WriteString(analyze.FinalReleaseDecisionString(p.FinalDecision))
		b.WriteString("```\n")
		if p.EvidenceBundle.Status != "" {
			b.WriteString("\n## Release evidence bundle\n\n```text\n")
			b.WriteString(analyze.ReleaseEvidenceBundleInspectionString(p.EvidenceBundle))
			b.WriteString("```\n")
		}
		b.WriteString("\n")
		b.WriteString(analyze.ReleaseAuditTodoReportMarkdown(p.ReleaseTodos))
	}
	if len(p.ArtifactChecks) != 0 {
		b.WriteString("\n## Artifact file checks\n\n| Role | Result | Bytes | SHA-256 |\n|---|---|---:|---|\n")
		for _, c := range p.ArtifactChecks {
			result := "mismatch"
			if c.LooksValid {
				result = "ok"
			}
			fmt.Fprintf(&b, "| %s | %s | %d | `%s` |\n", c.Role, result, c.Bytes, c.SHA256)
		}
	}
	return b.String()
}
