package runner

import (
	"testing"

	"github.com/mattneel/glassroot/internal/model"
)

func TestCloneExecutionResultPreservesEmptyLimitationsAsArrays(t *testing.T) {
	result := ExecutionResult{
		Limitations: []model.Limitation{},
		Attempts: []AttemptResult{{
			Outcome:     AttemptOutcome{Limitations: []model.Limitation{}},
			Limitations: []model.Limitation{},
		}},
	}
	cloned := cloneExecutionResult(result)
	if cloned.Limitations == nil {
		t.Fatal("execution limitations cloned as nil")
	}
	if len(cloned.Attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(cloned.Attempts))
	}
	if cloned.Attempts[0].Limitations == nil {
		t.Fatal("attempt limitations cloned as nil")
	}
	if cloned.Attempts[0].Outcome.Limitations == nil {
		t.Fatal("outcome limitations cloned as nil")
	}
}
