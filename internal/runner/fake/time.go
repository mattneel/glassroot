package fake

import (
	"time"

	"github.com/mattneel/glassroot/internal/runner"
)

func runnerSyntheticTime(request runner.AttemptRequest, offsetMillis int64) time.Time {
	base := request.PlanCreatedAt.UTC().Round(0)
	if request.GlobalOrdinal > 0 {
		base = base.Add(time.Duration(request.GlobalOrdinal-1) * time.Second)
	}
	return base.Add(time.Duration(offsetMillis) * time.Millisecond)
}
