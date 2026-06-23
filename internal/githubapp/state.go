package githubapp

type DeliveryState string

const (
	DeliveryStateReceived  DeliveryState = "received"
	DeliveryStateVerified  DeliveryState = "verified"
	DeliveryStatePersisted DeliveryState = "persisted"
	DeliveryStateEnqueued  DeliveryState = "enqueued"
	DeliveryStateIgnored   DeliveryState = "ignored"
	DeliveryStateRejected  DeliveryState = "rejected"
)

func TransitionDelivery(from, to DeliveryState) (DeliveryState, error) {
	if from == to {
		return from, nil
	}
	if from == DeliveryStateEnqueued || from == DeliveryStateIgnored || from == DeliveryStateRejected {
		return from, errCode(CodeInvalidStateTransition, "delivery", "terminal delivery state cannot transition", nil)
	}
	allowed := map[DeliveryState][]DeliveryState{
		DeliveryStateReceived:  {DeliveryStateVerified, DeliveryStateRejected},
		DeliveryStateVerified:  {DeliveryStatePersisted, DeliveryStateIgnored, DeliveryStateRejected},
		DeliveryStatePersisted: {DeliveryStateEnqueued, DeliveryStateIgnored, DeliveryStateRejected},
	}
	for _, x := range allowed[from] {
		if x == to {
			return to, nil
		}
	}
	return from, errCode(CodeInvalidStateTransition, "delivery", "delivery transition rejected", nil)
}

type JobState string

const (
	JobStateQueued           JobState = "queued"
	JobStateImportingSource  JobState = "importing-source"
	JobStatePlanning         JobState = "planning"
	JobStateAwaitingRunner   JobState = "awaiting-runner"
	JobStateRunning          JobState = "running"
	JobStateValidatingReport JobState = "validating-report"
	JobStateReadyToPublish   JobState = "ready-to-publish"
	JobStateCompleted        JobState = "completed"
	JobStateFailed           JobState = "failed"
	JobStateSuperseded       JobState = "superseded"
	JobStateCancelled        JobState = "cancelled"
)

type AttemptState string

const (
	AttemptStateQueued       AttemptState = "queued"
	AttemptStateLeased       AttemptState = "leased"
	AttemptStateRunning      AttemptState = "running"
	AttemptStateCompleted    AttemptState = "completed"
	AttemptStateFailed       AttemptState = "failed"
	AttemptStateCancelled    AttemptState = "cancelled"
	AttemptStateLeaseExpired AttemptState = "lease-expired"
)

type CheckPublicationState string

const (
	CheckPublicationAbsent            CheckPublicationState = "absent"
	CheckPublicationCreatePending     CheckPublicationState = "create-pending"
	CheckPublicationQueued            CheckPublicationState = "queued"
	CheckPublicationInProgress        CheckPublicationState = "in-progress"
	CheckPublicationCompletionPending CheckPublicationState = "completion-pending"
	CheckPublicationCompleted         CheckPublicationState = "completed"
	CheckPublicationCancelled         CheckPublicationState = "cancelled"
	CheckPublicationAmbiguous         CheckPublicationState = "ambiguous"
	CheckPublicationFailed            CheckPublicationState = "failed"
)

func TransitionJob(from, to JobState) (JobState, error) {
	if from == to {
		return from, nil
	}
	if jobTerminal(from) {
		return from, errCode(CodeInvalidStateTransition, "job", "terminal job state cannot transition", nil)
	}
	allowed := map[JobState][]JobState{
		JobStateQueued:           {JobStateImportingSource, JobStateSuperseded, JobStateCancelled, JobStateFailed},
		JobStateImportingSource:  {JobStatePlanning, JobStateSuperseded, JobStateCancelled, JobStateFailed},
		JobStatePlanning:         {JobStateAwaitingRunner, JobStateSuperseded, JobStateCancelled, JobStateFailed},
		JobStateAwaitingRunner:   {JobStateRunning, JobStateSuperseded, JobStateCancelled, JobStateFailed},
		JobStateRunning:          {JobStateValidatingReport, JobStateSuperseded, JobStateCancelled, JobStateFailed},
		JobStateValidatingReport: {JobStateReadyToPublish, JobStateSuperseded, JobStateCancelled, JobStateFailed},
		JobStateReadyToPublish:   {JobStateCompleted, JobStateSuperseded, JobStateCancelled, JobStateFailed},
	}
	if containsJob(allowed[from], to) {
		return to, nil
	}
	return from, errCode(CodeInvalidStateTransition, "job", "job transition rejected", nil)
}

func jobTerminal(s JobState) bool {
	return s == JobStateCompleted || s == JobStateFailed || s == JobStateSuperseded || s == JobStateCancelled
}
func containsJob(xs []JobState, y JobState) bool {
	for _, x := range xs {
		if x == y {
			return true
		}
	}
	return false
}

func TransitionAttempt(from, to AttemptState) (AttemptState, error) {
	if from == to {
		return from, nil
	}
	allowed := map[AttemptState][]AttemptState{
		AttemptStateQueued:       {AttemptStateLeased, AttemptStateCancelled},
		AttemptStateLeased:       {AttemptStateRunning, AttemptStateLeaseExpired, AttemptStateCancelled, AttemptStateFailed},
		AttemptStateRunning:      {AttemptStateCompleted, AttemptStateFailed, AttemptStateCancelled, AttemptStateLeaseExpired},
		AttemptStateLeaseExpired: {AttemptStateQueued, AttemptStateCancelled, AttemptStateFailed},
	}
	if attemptTerminal(from) {
		return from, errCode(CodeInvalidStateTransition, "attempt", "terminal attempt state cannot transition", nil)
	}
	for _, x := range allowed[from] {
		if x == to {
			return to, nil
		}
	}
	return from, errCode(CodeInvalidStateTransition, "attempt", "attempt transition rejected", nil)
}

func attemptTerminal(s AttemptState) bool {
	return s == AttemptStateCompleted || s == AttemptStateFailed || s == AttemptStateCancelled
}

func TransitionPublication(from, to CheckPublicationState) (CheckPublicationState, error) {
	if from == to {
		return from, nil
	}
	if publicationTerminal(from) {
		return from, errCode(CodeInvalidStateTransition, "publication", "terminal publication state cannot transition", nil)
	}
	allowed := map[CheckPublicationState][]CheckPublicationState{
		CheckPublicationAbsent:            {CheckPublicationCreatePending, CheckPublicationCancelled, CheckPublicationFailed},
		CheckPublicationCreatePending:     {CheckPublicationQueued, CheckPublicationAmbiguous, CheckPublicationFailed, CheckPublicationCancelled},
		CheckPublicationQueued:            {CheckPublicationInProgress, CheckPublicationCompletionPending, CheckPublicationCancelled, CheckPublicationFailed},
		CheckPublicationInProgress:        {CheckPublicationCompletionPending, CheckPublicationCancelled, CheckPublicationFailed},
		CheckPublicationCompletionPending: {CheckPublicationCompleted, CheckPublicationAmbiguous, CheckPublicationFailed, CheckPublicationCancelled},
		CheckPublicationAmbiguous:         {CheckPublicationQueued, CheckPublicationFailed},
	}
	for _, x := range allowed[from] {
		if x == to {
			return to, nil
		}
	}
	return from, errCode(CodeInvalidStateTransition, "publication", "publication transition rejected", nil)
}

func publicationTerminal(s CheckPublicationState) bool {
	return s == CheckPublicationCompleted || s == CheckPublicationCancelled || s == CheckPublicationFailed
}
