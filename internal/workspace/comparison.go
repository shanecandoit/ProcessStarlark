package workspace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/engine"
)

const MaxScenarios = 32

type Scenario struct {
	Name string `json:"name"`
	Call string `json:"call"`
}

type ScenarioOutcome struct {
	Status  string `json:"status"`
	Output  string `json:"output,omitempty"`
	Message string `json:"message,omitempty"`
}

type ScenarioComparison struct {
	Scenario
	Before   ScenarioOutcome `json:"before"`
	After    ScenarioOutcome `json:"after"`
	Category string          `json:"category"`
	Changed  bool            `json:"changed"`
}

// ComparisonReport records the exact inputs and snapshot identities used. It is
// returned/downloadable, not retained in server state or promoted to examples.
type ComparisonReport struct {
	ID string `json:"id"`
	Production
	DraftID             string               `json:"draft_id"`
	DraftRevision       string               `json:"draft_revision"`
	DraftSourceRevision string               `json:"draft_source_revision"`
	Runtime             string               `json:"runtime"`
	ScenarioSetHash     string               `json:"scenario_set_hash"`
	Scenarios           []Scenario           `json:"scenarios"`
	Rows                []ScenarioComparison `json:"rows"`
	CreatedAt           time.Time            `json:"created_at"`
}

func validateScenarios(scenarios []Scenario) error {
	if len(scenarios) == 0 || len(scenarios) > MaxScenarios {
		return fmt.Errorf("%w: supply 1–%d named scenarios", ErrReleaseInput, MaxScenarios)
	}
	names := map[string]bool{}
	for _, s := range scenarios {
		name := strings.TrimSpace(s.Name)
		if name == "" || len(name) > 120 || strings.TrimSpace(s.Call) == "" || len(s.Call) > 4096 {
			return fmt.Errorf("%w: each scenario needs a name (up to 120 bytes) and call (up to 4096 bytes)", ErrReleaseInput)
		}
		if names[name] {
			return fmt.Errorf("%w: repeated scenario name %q", ErrReleaseInput, name)
		}
		names[name] = true
	}
	return nil
}

func compareOutcome(before, after ScenarioOutcome) (string, bool) {
	changed := before.Status != after.Status || before.Output != after.Output
	if before.Status == Error || after.Status == Error {
		return "error", changed || before.Message != after.Message
	}
	if before.Status == Unconfirmed || after.Status == Unconfirmed {
		return "unconfirmed", changed
	}
	if before.Status == Unresolved && after.Status == OK {
		return "newly_answered", true
	}
	if after.Status == Unresolved {
		if before.Status != Unresolved {
			return "became_unresolved", true
		}
		return "unresolved", false
	}
	if changed {
		return "changed", true
	}
	return "unchanged", false
}

// FinishRelease evaluates captured snapshots after the store lock/transaction
// ends. No evaluation writes a draft, stored release, or question queue.
func FinishRelease(ctx context.Context, result ReleaseResult, c ReleaseCommand) (ReleaseResult, error) {
	if c.Action == "run" {
		answer := RunRelease(ctx, *result.Release, result.Production, c.Call)
		result.Release, result.Run = nil, &answer
	}
	if c.Action != "compare" {
		return result, nil
	}
	version, release := result.Candidate, result.Release
	scenarios := append([]Scenario{}, c.Scenarios...)
	for i := range scenarios {
		scenarios[i].Name = strings.TrimSpace(scenarios[i].Name)
		scenarios[i].Call = strings.TrimSpace(scenarios[i].Call)
	}
	report := ComparisonReport{Production: result.Production, DraftID: c.DraftID, DraftRevision: version.ID, DraftSourceRevision: version.SourceRevision,
		Runtime: engine.RuntimeIdentity, ScenarioSetHash: digest(scenarios), Scenarios: scenarios, Rows: []ScenarioComparison{}, CreatedAt: time.Now().UTC()}
	report.ID = digest(struct {
		Runtime, DraftID, Draft, Scenarios string
		Production                         Production
	}{report.Runtime, report.DraftID, report.DraftRevision, report.ScenarioSetHash, report.Production})
	for _, s := range scenarios {
		if err := ctx.Err(); err != nil {
			return ReleaseResult{}, err
		}
		before := Run(ctx, release.Files, s.Call)
		if err := ctx.Err(); err != nil {
			return ReleaseResult{}, err
		}
		after := Run(ctx, version.Files, s.Call)
		if err := ctx.Err(); err != nil {
			return ReleaseResult{}, err
		}
		b := ScenarioOutcome{Status: before.Status, Output: before.Output, Message: before.Message}
		a := ScenarioOutcome{Status: after.Status, Output: after.Output, Message: after.Message}
		category, changed := compareOutcome(b, a)
		report.Rows = append(report.Rows, ScenarioComparison{Scenario: s, Before: b, After: a, Category: category, Changed: changed})
	}
	result.Candidate, result.Release, result.Comparison = nil, nil, &report
	return result, nil
}
