package agentrun

import (
	"encoding/json"
	"errors"
	"testing"
)

const testProtocolSHA = ProtocolSHA256

func validContract() Contract {
	return Contract{
		Schema:                 ProtocolVersion,
		ProtocolPackageVersion: ProtocolPackageVersion,
		ProtocolSHA256:         testProtocolSHA,
		RunID:                  "run-1",
		DispatchAuthority: DispatchAuthority{
			System: "multica",
			Actor:  "lead-1",
		},
		SourceRef:     "issue-1",
		Tier:          "M",
		Objective:     "Ship a verified change",
		WorkspaceMode: "worktree",
		Status:        "in_review",
		ActiveWorkers: []string{},
		Steps: []Step{
			{
				StepID:    "implement",
				Role:      "implementer",
				Executor:  "linus",
				Status:    "passed",
				DependsOn: []string{},
				Scope: Scope{
					Workspace:     "/repo",
					WritablePaths: []string{"server/internal/agentrun/**"},
					ForbiddenPaths: []string{
						"server/migrations/**",
					},
					ExternalWrites: false,
				},
				Acceptance: []Acceptance{
					{ID: "tests", Check: "unit tests pass"},
				},
				Verification: []string{"go test ./internal/agentrun"},
				Evidence: []Evidence{
					{
						Kind:           "test",
						Producer:       "linus",
						StepID:         "implement",
						Outcome:        "pass",
						ArtifactRef:    "test://agentrun",
						AcceptanceRefs: []string{"tests"},
						Gaps:           []string{},
					},
				},
			},
			{
				StepID:    "review",
				Role:      "reviewer",
				Executor:  "turing",
				Status:    "passed",
				DependsOn: []string{"implement"},
				Scope: Scope{
					Workspace:      "/repo",
					WritablePaths:  []string{},
					ForbiddenPaths: []string{"**"},
					ExternalWrites: false,
				},
				Acceptance: []Acceptance{
					{ID: "independent", Check: "reviewer verifies existing evidence"},
				},
				Verification: []string{"inspect test result"},
				Evidence: []Evidence{
					{
						Kind:           "decision",
						Producer:       "turing",
						StepID:         "review",
						Outcome:        "pass",
						ArtifactRef:    "review://1",
						AcceptanceRefs: []string{"independent"},
						Gaps:           []string{},
					},
				},
			},
		},
		Review: Review{
			Required:  true,
			Reviewer:  "turing",
			Cycle:     1,
			MaxCycles: 3,
			Verdict:   "PASS",
		},
	}
}

func requireViolation(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected violation %q, got nil", code)
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	for _, violation := range validationErr.Violations {
		if violation.Code == code {
			return
		}
	}
	t.Fatalf("expected violation %q, got %+v", code, validationErr.Violations)
}

func cloneContract(t *testing.T, contract Contract) Contract {
	t.Helper()
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	var cloned Contract
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatalf("unmarshal contract: %v", err)
	}
	return cloned
}

func TestValidateContractAcceptsValidIndependentReview(t *testing.T) {
	if err := Validate(validContract()); err != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}
}

func TestValidateContractRejectsWellFormedButDifferentProtocolIdentity(t *testing.T) {
	contract := validContract()
	contract.ProtocolPackageVersion = "9.9.9"
	contract.ProtocolSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	requireViolation(t, Validate(contract), "protocol_identity_mismatch")
}

func TestValidateContractRejectsEmptyRunAndEmptyAcceptance(t *testing.T) {
	emptyRun := validContract()
	emptyRun.Steps = nil
	requireViolation(t, Validate(emptyRun), "step_required")

	emptyAcceptance := validContract()
	emptyAcceptance.Steps[0].Acceptance = nil
	emptyAcceptance.Steps[0].Verification = nil
	emptyAcceptance.Steps[0].Evidence = nil
	requireViolation(t, Validate(emptyAcceptance), "acceptance_required")
	requireViolation(t, Validate(emptyAcceptance), "verification_required")
}

func TestValidateContractRejectsEvidenceFromDifferentExecutor(t *testing.T) {
	contract := validContract()
	contract.Steps[0].Evidence[0].Producer = "someone-else"

	requireViolation(t, Validate(contract), "evidence_producer_mismatch")
}

func TestValidateContractRejectsPassedMLRunWithoutReviewerStep(t *testing.T) {
	contract := validContract()
	contract.Status = "passed"
	contract.Steps = contract.Steps[:1]
	contract.Review.Verdict = "PASS"

	requireViolation(t, Validate(contract), "passed_without_reviewer_step")
}

func TestValidateContractRejectsDraftWithRunningWork(t *testing.T) {
	contract := validContract()
	contract.Status = "draft"
	contract.Steps[0].Status = "running"
	contract.Steps[0].Evidence = nil
	contract.ActiveWorkers = []string{contract.Steps[0].Executor}
	contract.Steps[1].Status = "planned"
	contract.Steps[1].Evidence = nil
	contract.Review.Cycle = 0
	contract.Review.Verdict = ""

	requireViolation(t, Validate(contract), "draft_has_running_step")
	requireViolation(t, Validate(contract), "draft_has_active_workers")
}

func TestValidateContractRejectsUnsafeRunID(t *testing.T) {
	contract := validContract()
	contract.RunID = "run/one"

	requireViolation(t, Validate(contract), "invalid_run_id")
}

func TestValidateContractRejectsDependencyStartingEarly(t *testing.T) {
	contract := validContract()
	contract.Steps[0].Status = "running"
	contract.Steps[0].Evidence = nil
	contract.Steps[1].Status = "running"
	contract.ActiveWorkers = []string{"linus", "turing"}

	requireViolation(t, Validate(contract), "dependency_not_passed")
}

func TestValidateContractRejectsOverlappingRunningWriters(t *testing.T) {
	contract := validContract()
	contract.Status = "running"
	contract.Review.Verdict = ""
	contract.Steps[0].Status = "running"
	contract.Steps[0].Evidence = nil
	contract.Steps[1].DependsOn = nil
	contract.Steps[1].Role = "implementer"
	contract.Steps[1].Status = "running"
	contract.Steps[1].Scope.WritablePaths = []string{"server/internal/agentrun/contract.go"}
	contract.Steps[1].Scope.ForbiddenPaths = nil
	contract.ActiveWorkers = []string{"linus", "turing"}

	requireViolation(t, Validate(contract), "writable_scope_conflict")
}

func TestValidateContractRejectsMultipleRunningStepsForOneExecutor(t *testing.T) {
	contract := validContract()
	contract.Status = "running"
	contract.Review.Verdict = ""
	contract.Steps[0].Status = "running"
	contract.Steps[0].Evidence = nil
	contract.Steps[1].DependsOn = nil
	contract.Steps[1].Role = "implementer"
	contract.Steps[1].Executor = "linus"
	contract.Steps[1].Status = "running"
	contract.Steps[1].Evidence = nil
	contract.ActiveWorkers = []string{"linus"}

	requireViolation(t, Validate(contract), "executor_has_multiple_running_steps")
}

func TestValidateContractRejectsActiveWorkerWithoutRunningStep(t *testing.T) {
	contract := validContract()
	contract.Status = "running"
	contract.Review.Verdict = ""
	contract.Steps[0].Status = "running"
	contract.Steps[0].Evidence = nil
	contract.Steps[1].Status = "planned"
	contract.Steps[1].Evidence = nil
	contract.ActiveWorkers = []string{"linus", "turing"}

	requireViolation(t, Validate(contract), "active_worker_without_running_step")
}

func TestValidateConcurrentContractsRejectsOverlapAcrossRuns(t *testing.T) {
	left := validContract()
	left.RunID = "left"
	left.Status = "running"
	left.Review.Verdict = ""
	left.Steps = left.Steps[:1]
	left.Steps[0].Status = "running"
	left.Steps[0].Evidence = nil
	left.ActiveWorkers = []string{"linus"}

	right := validContract()
	right.RunID = "right"
	right.Status = "running"
	right.Review.Verdict = ""
	right.Steps = right.Steps[:1]
	right.Steps[0].Status = "running"
	right.Steps[0].Executor = "jobs"
	right.Steps[0].Scope.WritablePaths = []string{"server/internal/agentrun/contract.go"}
	right.Steps[0].Evidence = nil
	right.ActiveWorkers = []string{"jobs"}

	requireViolation(t, ValidateConcurrentContracts(left, right), "writable_scope_conflict")
}

func TestValidateContractRejectsReviewerWhoImplemented(t *testing.T) {
	contract := validContract()
	contract.Review.Reviewer = "linus"

	requireViolation(t, Validate(contract), "reviewer_is_implementer")
}

func TestValidateContractRejectsPassedStepWithoutAcceptanceEvidence(t *testing.T) {
	contract := validContract()
	contract.Steps[0].Evidence[0].AcceptanceRefs = nil

	requireViolation(t, Validate(contract), "acceptance_without_passing_evidence")
}

func TestValidateContractRejectsFourthReviewCycle(t *testing.T) {
	contract := validContract()
	contract.Review.Cycle = 4

	requireViolation(t, Validate(contract), "review_budget_exceeded")
}

func TestValidateContractRequiresBlockedAtThirdNonPassReview(t *testing.T) {
	contract := validContract()
	contract.Status = "in_review"
	contract.Review.Cycle = 3
	contract.Review.Verdict = "FAIL"

	requireViolation(t, Validate(contract), "review_budget_requires_blocked")
}

func TestValidateContractRejectsTerminalRunWithActiveWork(t *testing.T) {
	contract := validContract()
	contract.Status = "passed"
	contract.ActiveWorkers = []string{"turing"}

	requireViolation(t, Validate(contract), "terminal_run_has_active_workers")
}

func TestValidateContractRejectsUnapprovedExternalWrite(t *testing.T) {
	contract := validContract()
	contract.Status = "running"
	contract.Steps[0].Status = "running"
	contract.Steps[0].Evidence = nil
	contract.Steps[0].Scope.ExternalWrites = true
	contract.ActiveWorkers = []string{"linus"}
	contract.ApprovalBoundaries = []ApprovalBoundary{
		{ID: "deploy", Required: true, Satisfied: false},
	}

	requireViolation(t, Validate(contract), "external_write_without_approval")
}

func TestValidateTransitionRejectsTerminalReopen(t *testing.T) {
	previous := validContract()
	previous.Status = "passed"
	next := previous
	next.Status = "running"

	requireViolation(t, ValidateTransition(previous, next), "terminal_run_reopened")
}

func TestValidateTransitionRejectsReviewCycleSkip(t *testing.T) {
	previous := validContract()
	previous.Review.Cycle = 0
	next := validContract()
	next.Review.Cycle = 2

	requireViolation(t, ValidateTransition(previous, next), "review_cycle_not_atomic")
}

func TestValidateTransitionRejectsDispatchAuthorityChange(t *testing.T) {
	previous := validContract()
	next := cloneContract(t, previous)
	next.DispatchAuthority.Actor = "other-lead"

	requireViolation(t, ValidateTransition(previous, next), "immutable_field_changed")
}

func TestValidateTransitionRejectsAcceptanceRemovalAfterDispatch(t *testing.T) {
	previous := validContract()
	next := cloneContract(t, previous)
	next.Steps[0].Acceptance = nil
	next.Steps[0].Evidence = nil

	requireViolation(t, ValidateTransition(previous, next), "immutable_step_field_changed")
}

func TestValidateTransitionRejectsExecutorChangeAfterDispatch(t *testing.T) {
	previous := validContract()
	next := cloneContract(t, previous)
	next.Steps[0].Executor = "jobs"

	requireViolation(t, ValidateTransition(previous, next), "immutable_step_field_changed")
}

func TestValidateTransitionRejectsPassedStepReopen(t *testing.T) {
	previous := validContract()
	next := cloneContract(t, previous)
	next.Status = "running"
	next.Review.Verdict = ""
	next.Steps[0].Status = "running"
	next.Steps[0].Evidence = nil
	next.ActiveWorkers = []string{"linus"}

	requireViolation(t, ValidateTransition(previous, next), "invalid_step_status_transition")
}

func TestValidateTransitionRejectsExistingStepRemoval(t *testing.T) {
	previous := validContract()
	next := cloneContract(t, previous)
	next.Steps = next.Steps[1:]

	requireViolation(t, ValidateTransition(previous, next), "existing_step_removed")
}

func TestValidateTransitionAllowsAppendingRemediationStep(t *testing.T) {
	previous := validContract()
	next := cloneContract(t, previous)
	next.Status = "running"
	next.Review.Cycle = 2
	next.Review.Verdict = ""
	next.Steps = append(next.Steps, Step{
		StepID:    "remediate-2",
		Role:      "implementer",
		Executor:  "linus",
		DependsOn: []string{"review"},
		Status:    "planned",
		Scope: Scope{
			Workspace:      "/repo",
			WritablePaths:  []string{"server/internal/agentrun/**"},
			ForbiddenPaths: []string{"server/migrations/**"},
		},
		Acceptance: []Acceptance{
			{ID: "fix-review", Check: "review finding is remediated"},
		},
		Verification: []string{"go test ./internal/agentrun"},
	})

	if err := ValidateTransition(previous, next); err != nil {
		t.Fatalf("valid remediation append rejected: %v", err)
	}
}

func TestValidateTransitionRejectsTerminalEvidenceRewrite(t *testing.T) {
	previous := validContract()
	previous.Status = "passed"
	next := cloneContract(t, previous)
	next.Steps[0].Evidence[0].ArtifactRef = "test://rewritten"

	requireViolation(t, ValidateTransition(previous, next), "terminal_run_mutated")
}

func TestValidateTransitionRejectsSatisfiedApprovalRewrite(t *testing.T) {
	previous := validContract()
	evidence := "approval://one"
	previous.ApprovalBoundaries = []ApprovalBoundary{
		{ID: "deploy", Required: true, Satisfied: true, EvidenceRef: &evidence},
	}
	next := cloneContract(t, previous)
	rewritten := "approval://two"
	next.ApprovalBoundaries[0].EvidenceRef = &rewritten

	requireViolation(t, ValidateTransition(previous, next), "satisfied_approval_mutated")
}
