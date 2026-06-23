package githubapp

const (
	AbsoluteMaxWebhookBodyBytes      = 4 << 20
	AbsoluteMaxWebhookSecretBytes    = 256
	AbsoluteMaxHeaderValueBytes      = 1024
	AbsoluteMaxDeliveryIDBytes       = 128
	AbsoluteMaxEventNameBytes        = 64
	AbsoluteMaxExternalIDBytes       = 128
	AbsoluteMaxRepositoryNameBytes   = 256
	AbsoluteMaxProjectionStringBytes = 4 << 10
	AbsoluteMaxJSONDepth             = 64
	AbsoluteMaxJSONTokens            = 250000
	AbsoluteMaxMembersPerObject      = 25000
	AbsoluteMaxArrayElements         = 100000
	AbsoluteMaxJSONStringBytes       = 1 << 20
	AbsoluteMaxJSONNumberBytes       = 128
	AbsoluteMaxJobLimitations        = 1000
	AbsoluteMaxCheckTitleBytes       = 255
	AbsoluteMaxCheckSummaryBytes     = 32 << 10
	AbsoluteMaxCheckTextBytes        = 32 << 10
	AbsoluteMaxStateTransitions      = 1000
	AbsoluteMaxAttemptsPerTarget     = 32
	AbsoluteMaxProtocolDocumentBytes = 4 << 20
	AbsoluteMinWebhookSecretBytes    = 1
	AbsoluteMaxSignatureHeaderBytes  = 128
	AbsoluteMaxControllerGeneration  = int64(1<<63 - 1)
)

type Limits struct {
	MaxWebhookBodyBytes      int
	MinWebhookSecretBytes    int
	MaxWebhookSecretBytes    int
	MaxSignatureHeaderBytes  int
	MaxHeaderValueBytes      int
	MaxDeliveryIDBytes       int
	MaxEventNameBytes        int
	MaxExternalIDBytes       int
	MaxRepositoryNameBytes   int
	MaxProjectionStringBytes int
	MaxJSONDepth             int
	MaxJSONTokens            int
	MaxMembersPerObject      int
	MaxArrayElements         int
	MaxJSONStringBytes       int
	MaxJSONNumberBytes       int
	MaxJobLimitations        int
	MaxCheckTitleBytes       int
	MaxCheckSummaryBytes     int
	MaxCheckTextBytes        int
	MaxStateTransitions      int
	MaxAttemptsPerTarget     int
	MaxProtocolDocumentBytes int
	MaxControllerGeneration  int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxWebhookBodyBytes:      AbsoluteMaxWebhookBodyBytes,
		MinWebhookSecretBytes:    32,
		MaxWebhookSecretBytes:    AbsoluteMaxWebhookSecretBytes,
		MaxSignatureHeaderBytes:  AbsoluteMaxSignatureHeaderBytes,
		MaxHeaderValueBytes:      AbsoluteMaxHeaderValueBytes,
		MaxDeliveryIDBytes:       AbsoluteMaxDeliveryIDBytes,
		MaxEventNameBytes:        AbsoluteMaxEventNameBytes,
		MaxExternalIDBytes:       AbsoluteMaxExternalIDBytes,
		MaxRepositoryNameBytes:   AbsoluteMaxRepositoryNameBytes,
		MaxProjectionStringBytes: AbsoluteMaxProjectionStringBytes,
		MaxJSONDepth:             AbsoluteMaxJSONDepth,
		MaxJSONTokens:            AbsoluteMaxJSONTokens,
		MaxMembersPerObject:      AbsoluteMaxMembersPerObject,
		MaxArrayElements:         AbsoluteMaxArrayElements,
		MaxJSONStringBytes:       AbsoluteMaxJSONStringBytes,
		MaxJSONNumberBytes:       AbsoluteMaxJSONNumberBytes,
		MaxJobLimitations:        AbsoluteMaxJobLimitations,
		MaxCheckTitleBytes:       AbsoluteMaxCheckTitleBytes,
		MaxCheckSummaryBytes:     AbsoluteMaxCheckSummaryBytes,
		MaxCheckTextBytes:        AbsoluteMaxCheckTextBytes,
		MaxStateTransitions:      AbsoluteMaxStateTransitions,
		MaxAttemptsPerTarget:     AbsoluteMaxAttemptsPerTarget,
		MaxProtocolDocumentBytes: AbsoluteMaxProtocolDocumentBytes,
		MaxControllerGeneration:  AbsoluteMaxControllerGeneration,
	}
}

func validateLimits(l Limits) error {
	checks := []struct{ got, max int }{
		{l.MaxWebhookBodyBytes, AbsoluteMaxWebhookBodyBytes},
		{l.MaxWebhookSecretBytes, AbsoluteMaxWebhookSecretBytes},
		{l.MaxSignatureHeaderBytes, AbsoluteMaxSignatureHeaderBytes},
		{l.MaxHeaderValueBytes, AbsoluteMaxHeaderValueBytes},
		{l.MaxDeliveryIDBytes, AbsoluteMaxDeliveryIDBytes},
		{l.MaxEventNameBytes, AbsoluteMaxEventNameBytes},
		{l.MaxExternalIDBytes, AbsoluteMaxExternalIDBytes},
		{l.MaxRepositoryNameBytes, AbsoluteMaxRepositoryNameBytes},
		{l.MaxProjectionStringBytes, AbsoluteMaxProjectionStringBytes},
		{l.MaxJSONDepth, AbsoluteMaxJSONDepth},
		{l.MaxJSONTokens, AbsoluteMaxJSONTokens},
		{l.MaxMembersPerObject, AbsoluteMaxMembersPerObject},
		{l.MaxArrayElements, AbsoluteMaxArrayElements},
		{l.MaxJSONStringBytes, AbsoluteMaxJSONStringBytes},
		{l.MaxJSONNumberBytes, AbsoluteMaxJSONNumberBytes},
		{l.MaxJobLimitations, AbsoluteMaxJobLimitations},
		{l.MaxCheckTitleBytes, AbsoluteMaxCheckTitleBytes},
		{l.MaxCheckSummaryBytes, AbsoluteMaxCheckSummaryBytes},
		{l.MaxCheckTextBytes, AbsoluteMaxCheckTextBytes},
		{l.MaxStateTransitions, AbsoluteMaxStateTransitions},
		{l.MaxAttemptsPerTarget, AbsoluteMaxAttemptsPerTarget},
		{l.MaxProtocolDocumentBytes, AbsoluteMaxProtocolDocumentBytes},
	}
	for _, c := range checks {
		if c.got <= 0 || c.got > c.max {
			return deterministicErr(CodeInvalidLimits, "limits")
		}
	}
	if l.MinWebhookSecretBytes < AbsoluteMinWebhookSecretBytes || l.MinWebhookSecretBytes > l.MaxWebhookSecretBytes {
		return deterministicErr(CodeInvalidLimits, "limits")
	}
	if l.MaxControllerGeneration <= 0 || l.MaxControllerGeneration > AbsoluteMaxControllerGeneration {
		return deterministicErr(CodeInvalidLimits, "limits")
	}
	return nil
}
