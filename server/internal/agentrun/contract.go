package agentrun

import (
	"fmt"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

const (
	ProtocolVersion        = "agent-run/v1"
	ProtocolPackageVersion = "1.1.0"
	ProtocolSHA256         = "7326e0ce5cca7258c2ba304c934656811e614b94739d603485d090472bc5bf68"
)

var (
	protocolSHA256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)
	runIDRE          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

var (
	validSystems     = setOf("multica", "hermes")
	validTiers       = setOf("S", "M", "L")
	validModes       = setOf("scratch", "directory", "worktree", "external")
	validRunStatuses = setOf(
		"draft", "running", "in_review", "passed", "blocked", "failed", "cancelled",
	)
	validStepStatuses = setOf(
		"planned", "ready", "running", "passed", "blocked", "failed", "cancelled",
	)
	validRoles = setOf(
		"lead", "product", "implementer", "reviewer", "release", "verifier", "other",
	)
	validVerdicts      = setOf("", "PASS", "FAIL", "BLOCKED", "NOT_IMPLEMENTED")
	validEvidenceKinds = setOf(
		"command", "test", "diff", "commit", "api", "db", "log", "ui", "artifact", "decision",
	)
	validEvidenceOutcomes = setOf("pass", "fail", "blocked", "observed")
	terminalRunStatuses   = setOf("passed", "blocked", "failed", "cancelled")
	terminalStepStatuses  = setOf("passed", "blocked", "failed", "cancelled")
)

type Contract struct {
	Schema                 string             `json:"schema"`
	ProtocolPackageVersion string             `json:"protocol_package_version"`
	ProtocolSHA256         string             `json:"protocol_sha256"`
	RunID                  string             `json:"run_id"`
	DispatchAuthority      DispatchAuthority  `json:"dispatch_authority"`
	SourceRef              string             `json:"source_ref"`
	Tier                   string             `json:"tier"`
	Objective              string             `json:"objective"`
	BaseRevision           *string            `json:"base_revision"`
	WorkspaceMode          string             `json:"workspace_mode"`
	Status                 string             `json:"status"`
	ApprovalBoundaries     []ApprovalBoundary `json:"approval_boundaries"`
	ActiveWorkers          []string           `json:"active_workers"`
	Steps                  []Step             `json:"steps"`
	Review                 Review             `json:"review"`
}

type DispatchAuthority struct {
	System string `json:"system"`
	Actor  string `json:"actor"`
}

type ApprovalBoundary struct {
	ID          string  `json:"id"`
	Required    bool    `json:"required"`
	Satisfied   bool    `json:"satisfied"`
	EvidenceRef *string `json:"evidence_ref"`
}

type Review struct {
	Required  bool   `json:"required"`
	Reviewer  string `json:"reviewer"`
	Cycle     int    `json:"cycle"`
	MaxCycles int    `json:"max_cycles"`
	Verdict   string `json:"verdict"`
}

type Step struct {
	StepID       string       `json:"step_id"`
	Role         string       `json:"role"`
	Executor     string       `json:"executor"`
	DependsOn    []string     `json:"depends_on"`
	Status       string       `json:"status"`
	Scope        Scope        `json:"scope"`
	Acceptance   []Acceptance `json:"acceptance"`
	Verification []string     `json:"verification"`
	Evidence     []Evidence   `json:"evidence"`
}

type Scope struct {
	Workspace      string   `json:"workspace"`
	WritablePaths  []string `json:"writable_paths"`
	ForbiddenPaths []string `json:"forbidden_paths"`
	ExternalWrites bool     `json:"external_writes"`
}

type Acceptance struct {
	ID    string `json:"id"`
	Check string `json:"check"`
}

type Evidence struct {
	Kind           string   `json:"kind"`
	Producer       string   `json:"producer"`
	StepID         string   `json:"step_id"`
	CommandOrQuery *string  `json:"command_or_query"`
	Outcome        string   `json:"outcome"`
	ArtifactRef    string   `json:"artifact_ref"`
	AcceptanceRefs []string `json:"acceptance_refs"`
	Gaps           []string `json:"gaps"`
}

type Violation struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ValidationError struct {
	Violations []Violation `json:"violations"`
}

func (e *ValidationError) Error() string {
	if len(e.Violations) == 0 {
		return "agent run contract is invalid"
	}
	return fmt.Sprintf("agent run contract is invalid: %s", e.Violations[0].Message)
}

type validator struct {
	violations []Violation
}

func (v *validator) add(code, fieldPath, message string) {
	v.violations = append(v.violations, Violation{
		Code:    code,
		Path:    fieldPath,
		Message: message,
	})
}

func (v *validator) err() error {
	if len(v.violations) == 0 {
		return nil
	}
	sort.SliceStable(v.violations, func(i, j int) bool {
		if v.violations[i].Path == v.violations[j].Path {
			return v.violations[i].Code < v.violations[j].Code
		}
		return v.violations[i].Path < v.violations[j].Path
	})
	return &ValidationError{Violations: v.violations}
}

func Validate(contract Contract) error {
	v := &validator{}

	v.require(contract.Schema == ProtocolVersion, "invalid_schema", "schema", "schema must be agent-run/v1")
	v.require(contract.ProtocolPackageVersion == ProtocolPackageVersion, "protocol_identity_mismatch", "protocol_package_version", "protocol package version does not match the Multica runtime")
	v.require(protocolSHA256RE.MatchString(contract.ProtocolSHA256), "invalid_protocol_sha256", "protocol_sha256", "protocol SHA-256 must be 64 lowercase hex characters")
	v.require(contract.ProtocolSHA256 == ProtocolSHA256, "protocol_identity_mismatch", "protocol_sha256", "protocol SHA-256 does not match the Multica runtime")
	v.require(runIDRE.MatchString(contract.RunID), "invalid_run_id", "run_id", "run id must be 1-128 URL-safe characters")
	v.require(validSystems[contract.DispatchAuthority.System], "invalid_dispatch_system", "dispatch_authority.system", "dispatch system must be multica or hermes")
	v.require(strings.TrimSpace(contract.DispatchAuthority.Actor) != "", "required", "dispatch_authority.actor", "dispatch authority actor is required")
	v.require(strings.TrimSpace(contract.SourceRef) != "", "required", "source_ref", "source reference is required")
	v.require(validTiers[contract.Tier], "invalid_tier", "tier", "tier must be S, M, or L")
	v.require(strings.TrimSpace(contract.Objective) != "", "required", "objective", "objective is required")
	v.require(validModes[contract.WorkspaceMode], "invalid_workspace_mode", "workspace_mode", "workspace mode is invalid")
	v.require(validRunStatuses[contract.Status], "invalid_run_status", "status", "run status is invalid")
	v.require(contract.Review.MaxCycles == 3, "invalid_review_budget", "review.max_cycles", "review max cycles must be 3")
	v.require(contract.Review.Cycle >= 0, "invalid_review_cycle", "review.cycle", "review cycle cannot be negative")
	v.require(contract.Review.Cycle <= contract.Review.MaxCycles, "review_budget_exceeded", "review.cycle", "review cycle cannot exceed max cycles")
	v.require(validVerdicts[contract.Review.Verdict], "invalid_review_verdict", "review.verdict", "review verdict is invalid")

	validateApprovals(v, contract.ApprovalBoundaries)
	validateWorkers(v, contract.ActiveWorkers)
	v.require(len(contract.Steps) > 0, "step_required", "steps", "run must contain at least one step")
	validateSteps(v, contract)
	validateReview(v, contract)
	validateRunStatusCoherence(v, contract)
	validateTerminal(v, contract)

	return v.err()
}

func ValidateTransition(previous, next Contract) error {
	v := &validator{}

	if err := Validate(next); err != nil {
		var validationErr *ValidationError
		if ok := asValidationError(err, &validationErr); ok {
			v.violations = append(v.violations, validationErr.Violations...)
		}
	}

	immutable := []struct {
		name     string
		previous string
		next     string
	}{
		{"schema", previous.Schema, next.Schema},
		{"protocol_package_version", previous.ProtocolPackageVersion, next.ProtocolPackageVersion},
		{"protocol_sha256", previous.ProtocolSHA256, next.ProtocolSHA256},
		{"run_id", previous.RunID, next.RunID},
		{"dispatch_authority.system", previous.DispatchAuthority.System, next.DispatchAuthority.System},
		{"dispatch_authority.actor", previous.DispatchAuthority.Actor, next.DispatchAuthority.Actor},
		{"source_ref", previous.SourceRef, next.SourceRef},
		{"tier", previous.Tier, next.Tier},
	}
	for _, field := range immutable {
		if field.previous != field.next {
			v.add("immutable_field_changed", field.name, field.name+" cannot change after run creation")
		}
	}

	if terminalRunStatuses[previous.Status] && next.Status != previous.Status {
		v.add("terminal_run_reopened", "status", "terminal run cannot transition to another status")
	} else if !allowedRunTransition(previous.Status, next.Status) {
		v.add("invalid_status_transition", "status", "run status transition is not allowed")
	}
	if terminalRunStatuses[previous.Status] && !reflect.DeepEqual(previous, next) {
		v.add("terminal_run_mutated", "contract", "terminal run contract is immutable")
	}

	if next.Review.Cycle < previous.Review.Cycle || next.Review.Cycle > previous.Review.Cycle+1 {
		v.add("review_cycle_not_atomic", "review.cycle", "review cycle may stay unchanged or increment by exactly one")
	}
	if previous.Status != "draft" {
		validateActiveStructureTransition(v, previous, next)
	}

	return v.err()
}

// Once dispatch begins, an authority may append remediation/review steps and
// add evidence, but it cannot weaken or rewrite already-declared work. Draft
// runs remain editable so the lead can finish planning before the first
// dispatch.
func validateActiveStructureTransition(v *validator, previous, next Contract) {
	v.require(previous.Objective == next.Objective, "immutable_field_changed", "objective", "objective cannot change after dispatch starts")
	v.require(reflect.DeepEqual(previous.BaseRevision, next.BaseRevision), "immutable_field_changed", "base_revision", "base revision cannot change after dispatch starts")
	v.require(previous.WorkspaceMode == next.WorkspaceMode, "immutable_field_changed", "workspace_mode", "workspace mode cannot change after dispatch starts")
	v.require(previous.Review.Required == next.Review.Required, "immutable_field_changed", "review.required", "review requirement cannot change after dispatch starts")
	v.require(previous.Review.Reviewer == next.Review.Reviewer, "immutable_field_changed", "review.reviewer", "reviewer cannot change after dispatch starts")
	v.require(previous.Review.MaxCycles == next.Review.MaxCycles, "immutable_field_changed", "review.max_cycles", "review budget cannot change after dispatch starts")

	nextSteps := make(map[string]Step, len(next.Steps))
	for _, step := range next.Steps {
		nextSteps[step.StepID] = step
	}
	for index, oldStep := range previous.Steps {
		fieldPath := fmt.Sprintf("steps[%d]", index)
		newStep, exists := nextSteps[oldStep.StepID]
		if !exists {
			v.add("existing_step_removed", fieldPath, "existing step cannot be removed after dispatch starts")
			continue
		}
		if oldStep.Role != newStep.Role {
			v.add("immutable_step_field_changed", fieldPath+".role", "existing step role cannot change after dispatch starts")
		}
		if oldStep.Executor != newStep.Executor {
			v.add("immutable_step_field_changed", fieldPath+".executor", "existing step executor cannot change after dispatch starts")
		}
		if !reflect.DeepEqual(oldStep.DependsOn, newStep.DependsOn) {
			v.add("immutable_step_field_changed", fieldPath+".depends_on", "existing step dependencies cannot change after dispatch starts")
		}
		if !reflect.DeepEqual(oldStep.Scope, newStep.Scope) {
			v.add("immutable_step_field_changed", fieldPath+".scope", "existing step scope cannot change after dispatch starts")
		}
		if !reflect.DeepEqual(oldStep.Acceptance, newStep.Acceptance) {
			v.add("immutable_step_field_changed", fieldPath+".acceptance", "existing step acceptance cannot change after dispatch starts")
		}
		if !reflect.DeepEqual(oldStep.Verification, newStep.Verification) {
			v.add("immutable_step_field_changed", fieldPath+".verification", "existing step verification plan cannot change after dispatch starts")
		}
		if !allowedStepTransition(oldStep.Status, newStep.Status) {
			v.add(
				"invalid_step_status_transition",
				fieldPath+".status",
				fmt.Sprintf("step status transition %q -> %q is not allowed", oldStep.Status, newStep.Status),
			)
		}
	}

	previousBoundaries := make(map[string]ApprovalBoundary, len(previous.ApprovalBoundaries))
	for _, boundary := range previous.ApprovalBoundaries {
		previousBoundaries[boundary.ID] = boundary
	}
	nextBoundaries := make(map[string]ApprovalBoundary, len(next.ApprovalBoundaries))
	for _, boundary := range next.ApprovalBoundaries {
		nextBoundaries[boundary.ID] = boundary
	}
	for id, oldBoundary := range previousBoundaries {
		newBoundary, exists := nextBoundaries[id]
		if !exists {
			v.add("approval_boundary_removed", "approval_boundaries", fmt.Sprintf("approval boundary %q cannot be removed after dispatch starts", id))
			continue
		}
		if oldBoundary.Required != newBoundary.Required {
			v.add("immutable_approval_boundary_changed", "approval_boundaries", fmt.Sprintf("approval boundary %q required flag cannot change after dispatch starts", id))
		}
		approvalRevoked := !newBoundary.Satisfied ||
			!reflect.DeepEqual(oldBoundary.EvidenceRef, newBoundary.EvidenceRef)
		if oldBoundary.Satisfied && approvalRevoked {
			v.add("satisfied_approval_mutated", "approval_boundaries", fmt.Sprintf("satisfied approval boundary %q cannot be revoked or rewritten", id))
		}
	}
}

func allowedStepTransition(previous, next string) bool {
	if previous == next {
		return true
	}
	switch previous {
	case "planned":
		// agent-run/v1 treats ready as an observable scheduling state, not a
		// mandatory hop. A dispatch authority may atomically start a planned
		// step once dependency and active-worker validation succeeds.
		return next == "ready" || next == "running" || next == "blocked" || next == "cancelled"
	case "ready":
		return next == "running" || next == "blocked" || next == "cancelled"
	case "running":
		return next == "passed" || next == "blocked" || next == "failed" || next == "cancelled"
	default:
		return false
	}
}

// ValidateConcurrentContracts rejects writable-scope overlap between running
// steps that belong to different runs in the same control plane. Callers must
// serialize this check with persistence (for example, under row/advisory
// locks); otherwise two individually valid updates can race past each other.
func ValidateConcurrentContracts(contracts ...Contract) error {
	v := &validator{}
	for i := 0; i < len(contracts); i++ {
		for j := i + 1; j < len(contracts); j++ {
			if contracts[i].RunID == contracts[j].RunID {
				continue
			}
			for _, left := range contracts[i].Steps {
				if left.Status != "running" {
					continue
				}
				for rightIndex, right := range contracts[j].Steps {
					if right.Status != "running" || left.Scope.Workspace != right.Scope.Workspace {
						continue
					}
					for _, leftPath := range left.Scope.WritablePaths {
						for _, rightPath := range right.Scope.WritablePaths {
							if scopePatternsOverlap(leftPath, rightPath) {
								v.add(
									"writable_scope_conflict",
									fmt.Sprintf("runs[%d].steps[%d].scope.writable_paths", j, rightIndex),
									fmt.Sprintf(
										"running steps %q/%q and %q/%q have overlapping writable scopes",
										contracts[i].RunID,
										left.StepID,
										contracts[j].RunID,
										right.StepID,
									),
								)
							}
						}
					}
				}
			}
		}
	}
	return v.err()
}

func validateApprovals(v *validator, boundaries []ApprovalBoundary) {
	seen := make(map[string]struct{}, len(boundaries))
	for i, boundary := range boundaries {
		base := fmt.Sprintf("approval_boundaries[%d]", i)
		id := strings.TrimSpace(boundary.ID)
		v.require(id != "", "required", base+".id", "approval boundary id is required")
		if _, ok := seen[id]; ok && id != "" {
			v.add("duplicate_approval_boundary", base+".id", "approval boundary id must be unique")
		}
		seen[id] = struct{}{}
		if boundary.Satisfied {
			v.require(boundary.EvidenceRef != nil && strings.TrimSpace(*boundary.EvidenceRef) != "", "approval_without_evidence", base+".evidence_ref", "satisfied approval requires evidence")
		}
	}
}

func validateWorkers(v *validator, workers []string) {
	seen := make(map[string]struct{}, len(workers))
	for i, worker := range workers {
		worker = strings.TrimSpace(worker)
		fieldPath := fmt.Sprintf("active_workers[%d]", i)
		v.require(worker != "", "required", fieldPath, "active worker identity is required")
		if _, ok := seen[worker]; ok && worker != "" {
			v.add("duplicate_active_worker", fieldPath, "active worker identity must be unique")
		}
		seen[worker] = struct{}{}
	}
}

func validateSteps(v *validator, contract Contract) {
	steps := make(map[string]Step, len(contract.Steps))
	for i, step := range contract.Steps {
		base := fmt.Sprintf("steps[%d]", i)
		stepID := strings.TrimSpace(step.StepID)
		v.require(stepID != "", "required", base+".step_id", "step id is required")
		if _, ok := steps[stepID]; ok && stepID != "" {
			v.add("duplicate_step_id", base+".step_id", "step id must be unique")
		}
		steps[stepID] = step
		v.require(validRoles[step.Role], "invalid_role", base+".role", "step role is invalid")
		v.require(strings.TrimSpace(step.Executor) != "", "required", base+".executor", "step executor is required")
		v.require(validStepStatuses[step.Status], "invalid_step_status", base+".status", "step status is invalid")
		v.require(strings.TrimSpace(step.Scope.Workspace) != "", "required", base+".scope.workspace", "scope workspace is required")
		validateAcceptance(v, base, step)
		validateEvidence(v, base, step)
		validateScope(v, base, step.Scope)
	}

	validateDependencyGraph(v, contract.Steps, steps)
	validateRunningScopeConflicts(v, contract.Steps)
	validateActiveWorkerAlignment(v, contract)
	validateExternalWrites(v, contract)
}

func validateAcceptance(v *validator, base string, step Step) {
	v.require(len(step.Acceptance) > 0, "acceptance_required", base+".acceptance", "step must declare at least one acceptance check")
	v.require(len(step.Verification) > 0, "verification_required", base+".verification", "step must declare at least one verification check")
	for i, verification := range step.Verification {
		v.require(strings.TrimSpace(verification) != "", "required", fmt.Sprintf("%s.verification[%d]", base, i), "verification check cannot be empty")
	}
	seen := make(map[string]struct{}, len(step.Acceptance))
	for i, acceptance := range step.Acceptance {
		fieldPath := fmt.Sprintf("%s.acceptance[%d]", base, i)
		id := strings.TrimSpace(acceptance.ID)
		v.require(id != "", "required", fieldPath+".id", "acceptance id is required")
		v.require(strings.TrimSpace(acceptance.Check) != "", "required", fieldPath+".check", "acceptance check is required")
		if _, ok := seen[id]; ok && id != "" {
			v.add("duplicate_acceptance_id", fieldPath+".id", "acceptance id must be unique within a step")
		}
		seen[id] = struct{}{}
	}

	if step.Status != "passed" {
		return
	}
	passing := make(map[string]struct{})
	for _, evidence := range step.Evidence {
		if evidence.Outcome != "pass" {
			continue
		}
		for _, ref := range evidence.AcceptanceRefs {
			passing[ref] = struct{}{}
		}
	}
	for i, acceptance := range step.Acceptance {
		if _, ok := passing[acceptance.ID]; !ok {
			v.add(
				"acceptance_without_passing_evidence",
				fmt.Sprintf("%s.acceptance[%d]", base, i),
				"passed step acceptance must be referenced by passing evidence",
			)
		}
	}
}

func validateEvidence(v *validator, base string, step Step) {
	acceptanceIDs := make(map[string]struct{}, len(step.Acceptance))
	for _, acceptance := range step.Acceptance {
		acceptanceIDs[acceptance.ID] = struct{}{}
	}
	for i, evidence := range step.Evidence {
		fieldPath := fmt.Sprintf("%s.evidence[%d]", base, i)
		v.require(validEvidenceKinds[evidence.Kind], "invalid_evidence_kind", fieldPath+".kind", "evidence kind is invalid")
		v.require(strings.TrimSpace(evidence.Producer) != "", "required", fieldPath+".producer", "evidence producer is required")
		v.require(evidence.Producer == step.Executor, "evidence_producer_mismatch", fieldPath+".producer", "step evidence producer must match the step executor")
		v.require(evidence.StepID == step.StepID, "evidence_step_mismatch", fieldPath+".step_id", "evidence step id must match its containing step")
		v.require(validEvidenceOutcomes[evidence.Outcome], "invalid_evidence_outcome", fieldPath+".outcome", "evidence outcome is invalid")
		v.require(strings.TrimSpace(evidence.ArtifactRef) != "", "required", fieldPath+".artifact_ref", "evidence artifact reference is required")
		for j, ref := range evidence.AcceptanceRefs {
			if _, ok := acceptanceIDs[ref]; !ok {
				v.add("unknown_acceptance_ref", fmt.Sprintf("%s.acceptance_refs[%d]", fieldPath, j), "evidence references an unknown acceptance id")
			}
		}
	}
}

func validateScope(v *validator, base string, scope Scope) {
	for i, writable := range scope.WritablePaths {
		fieldPath := fmt.Sprintf("%s.scope.writable_paths[%d]", base, i)
		v.require(strings.TrimSpace(writable) != "", "required", fieldPath, "writable path cannot be empty")
		for _, forbidden := range scope.ForbiddenPaths {
			if scopePatternsOverlap(writable, forbidden) {
				v.add("writable_path_forbidden", fieldPath, "writable path overlaps a forbidden path")
				break
			}
		}
	}
}

func validateDependencyGraph(v *validator, ordered []Step, steps map[string]Step) {
	for i, step := range ordered {
		base := fmt.Sprintf("steps[%d].depends_on", i)
		for j, dependencyID := range step.DependsOn {
			dependency, ok := steps[dependencyID]
			if !ok {
				v.add("unknown_dependency", fmt.Sprintf("%s[%d]", base, j), "dependency references an unknown step")
				continue
			}
			if dependencyID == step.StepID {
				v.add("dependency_cycle", fmt.Sprintf("%s[%d]", base, j), "step cannot depend on itself")
			}
			if (step.Status == "ready" || step.Status == "running" || step.Status == "passed") && dependency.Status != "passed" {
				v.add("dependency_not_passed", fmt.Sprintf("%s[%d]", base, j), "step cannot become ready, running, or passed before dependency passes")
			}
		}
	}

	visiting := make(map[string]bool, len(steps))
	visited := make(map[string]bool, len(steps))
	var visit func(string) bool
	visit = func(stepID string) bool {
		if visiting[stepID] {
			return true
		}
		if visited[stepID] {
			return false
		}
		visiting[stepID] = true
		for _, dependencyID := range steps[stepID].DependsOn {
			if _, ok := steps[dependencyID]; ok && visit(dependencyID) {
				return true
			}
		}
		visiting[stepID] = false
		visited[stepID] = true
		return false
	}
	for stepID := range steps {
		if visit(stepID) {
			v.add("dependency_cycle", "steps", "step dependency graph contains a cycle")
			break
		}
	}
}

func validateRunningScopeConflicts(v *validator, steps []Step) {
	for i := 0; i < len(steps); i++ {
		if steps[i].Status != "running" {
			continue
		}
		for j := i + 1; j < len(steps); j++ {
			if steps[j].Status != "running" || steps[i].Scope.Workspace != steps[j].Scope.Workspace {
				continue
			}
			for _, left := range steps[i].Scope.WritablePaths {
				for _, right := range steps[j].Scope.WritablePaths {
					if scopePatternsOverlap(left, right) {
						v.add(
							"writable_scope_conflict",
							fmt.Sprintf("steps[%d].scope.writable_paths", j),
							fmt.Sprintf("running steps %q and %q have overlapping writable scopes", steps[i].StepID, steps[j].StepID),
						)
					}
				}
			}
		}
	}
}

func validateActiveWorkerAlignment(v *validator, contract Contract) {
	active := make(map[string]struct{}, len(contract.ActiveWorkers))
	for _, worker := range contract.ActiveWorkers {
		active[worker] = struct{}{}
	}
	runningByExecutor := make(map[string]int)
	for i, step := range contract.Steps {
		if step.Status != "running" {
			continue
		}
		runningByExecutor[step.Executor]++
		if runningByExecutor[step.Executor] > 1 {
			v.add("executor_has_multiple_running_steps", fmt.Sprintf("steps[%d].executor", i), "one executor may have only one running step per run")
		}
		if _, ok := active[step.Executor]; !ok {
			v.add("running_step_missing_active_worker", fmt.Sprintf("steps[%d].executor", i), "running step executor must be listed in active_workers")
		}
	}
	for i, worker := range contract.ActiveWorkers {
		if runningByExecutor[worker] == 0 {
			v.add("active_worker_without_running_step", fmt.Sprintf("active_workers[%d]", i), "active worker must map to exactly one running step")
		}
	}
}

func validateExternalWrites(v *validator, contract Contract) {
	approvalsSatisfied := true
	for _, boundary := range contract.ApprovalBoundaries {
		if boundary.Required && (!boundary.Satisfied || boundary.EvidenceRef == nil || strings.TrimSpace(*boundary.EvidenceRef) == "") {
			approvalsSatisfied = false
			break
		}
	}
	for i, step := range contract.Steps {
		if !step.Scope.ExternalWrites || (step.Status != "running" && step.Status != "passed") {
			continue
		}
		if !approvalsSatisfied || len(contract.ApprovalBoundaries) == 0 {
			v.add("external_write_without_approval", fmt.Sprintf("steps[%d].scope.external_writes", i), "external write step requires satisfied approval evidence")
		}
	}
}

func validateReview(v *validator, contract Contract) {
	if contract.Tier == "M" || contract.Tier == "L" {
		v.require(contract.Review.Required, "independent_review_required", "review.required", "M/L run requires independent review")
	}
	if contract.Review.Required {
		v.require(strings.TrimSpace(contract.Review.Reviewer) != "", "reviewer_required", "review.reviewer", "required review must name a reviewer")
	}
	reviewerPassed := false
	for i, step := range contract.Steps {
		if step.Role == "implementer" && step.Executor == contract.Review.Reviewer && contract.Review.Reviewer != "" {
			v.add("reviewer_is_implementer", fmt.Sprintf("steps[%d].executor", i), "reviewer cannot be an implementation executor")
		}
		if step.Role == "reviewer" && step.Executor == contract.Review.Reviewer && step.Status == "passed" {
			reviewerPassed = true
		}
	}
	if contract.Review.Required && contract.Review.Verdict != "" {
		v.require(contract.Review.Cycle >= 1, "invalid_review_cycle", "review.cycle", "a review verdict requires cycle >= 1")
	}
	if contract.Review.Cycle == contract.Review.MaxCycles && contract.Review.Verdict != "" && contract.Review.Verdict != "PASS" && contract.Status != "blocked" {
		v.add("review_budget_requires_blocked", "status", "third non-PASS review requires blocked run status")
	}
	if contract.Status == "passed" && contract.Review.Required && contract.Review.Verdict != "PASS" {
		v.add("passed_without_review", "review.verdict", "passed M/L run requires PASS review verdict")
	}
	if contract.Status == "passed" && contract.Review.Required && !reviewerPassed {
		v.add("passed_without_reviewer_step", "steps", "passed M/L run requires a passed step owned by the declared reviewer")
	}
	if contract.Status == "passed" && contract.Review.Verdict == "NOT_IMPLEMENTED" {
		v.add("not_implemented_cannot_pass", "review.verdict", "NOT_IMPLEMENTED cannot map to a passed run")
	}
}

func validateRunStatusCoherence(v *validator, contract Contract) {
	if contract.Status != "draft" {
		return
	}
	v.require(len(contract.ActiveWorkers) == 0, "draft_has_active_workers", "active_workers", "draft run cannot have active workers")
	for i, step := range contract.Steps {
		if step.Status == "running" {
			v.add("draft_has_running_step", fmt.Sprintf("steps[%d].status", i), "draft run cannot contain a running step")
		}
	}
}

func validateTerminal(v *validator, contract Contract) {
	if !terminalRunStatuses[contract.Status] {
		return
	}
	if len(contract.ActiveWorkers) > 0 {
		v.add("terminal_run_has_active_workers", "active_workers", "terminal run cannot have active workers")
	}
	for i, step := range contract.Steps {
		if !terminalStepStatuses[step.Status] {
			v.add("terminal_run_has_active_step", fmt.Sprintf("steps[%d].status", i), "terminal run cannot contain a planned, ready, or running step")
		}
	}
}

func allowedRunTransition(previous, next string) bool {
	if previous == next {
		return true
	}
	switch previous {
	case "draft":
		return next == "running" || next == "blocked" || next == "cancelled"
	case "running":
		return next == "in_review" || next == "passed" || next == "blocked" || next == "failed" || next == "cancelled"
	case "in_review":
		return next == "running" || next == "passed" || next == "blocked" || next == "failed" || next == "cancelled"
	default:
		return false
	}
}

func scopePatternsOverlap(left, right string) bool {
	left = normalizeScopePattern(left)
	right = normalizeScopePattern(right)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}

	leftGlob := hasGlob(left)
	rightGlob := hasGlob(right)
	if !leftGlob && !rightGlob {
		return pathContains(left, right) || pathContains(right, left)
	}
	if leftGlob {
		if matched, err := path.Match(left, right); err == nil && matched {
			return true
		}
	}
	if rightGlob {
		if matched, err := path.Match(right, left); err == nil && matched {
			return true
		}
	}

	leftPrefix := fixedScopePrefix(left)
	rightPrefix := fixedScopePrefix(right)
	if leftPrefix == "" || rightPrefix == "" {
		return true
	}
	return pathContains(leftPrefix, rightPrefix) || pathContains(rightPrefix, leftPrefix)
}

func normalizeScopePattern(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean("/"+value), "/")
}

func hasGlob(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func fixedScopePrefix(value string) string {
	if index := strings.IndexAny(value, "*?["); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSuffix(strings.TrimSpace(value), "/")
}

func pathContains(parent, child string) bool {
	if parent == child {
		return true
	}
	return strings.HasPrefix(child, strings.TrimSuffix(parent, "/")+"/")
}

func (v *validator) require(condition bool, code, fieldPath, message string) {
	if !condition {
		v.add(code, fieldPath, message)
	}
}

func setOf(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func asValidationError(err error, target **ValidationError) bool {
	validationErr, ok := err.(*ValidationError)
	if !ok {
		return false
	}
	*target = validationErr
	return true
}
