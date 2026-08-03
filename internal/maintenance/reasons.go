package maintenance

type ReasonCode string

const (
	ReasonCompleted               ReasonCode = "completed"
	ReasonReviewedNoChange        ReasonCode = "reviewed_no_change"
	ReasonRecoveryRequired        ReasonCode = "recovery_required"
	ReasonRecoveryIntent          ReasonCode = "recovery_intent"
	ReasonRecoveryCompleted       ReasonCode = "recovery_completed"
	ReasonRecoveryFailed          ReasonCode = "recovery_failed"
	ReasonRecoveryAuditIncomplete ReasonCode = "recovery_committed_audit_incomplete"
	ReasonProposalEmitted         ReasonCode = "proposal_emitted"
	ReasonLeaseBusy               ReasonCode = "lease_busy"
	ReasonHandlerUnavailable      ReasonCode = "handler_unavailable"
	ReasonHandlerFailure          ReasonCode = "handler_failure"
	ReasonDeadlineExceeded        ReasonCode = "deadline_exceeded"
	ReasonCatalogUnavailable      ReasonCode = "catalog_unavailable"
	ReasonAuthorityRejected       ReasonCode = "authority_rejected"
	ReasonOccurrenceRejected      ReasonCode = "occurrence_rejected"
	ReasonReceiptPersisted        ReasonCode = "receipt_persisted"
	ReasonIdleUnknown             ReasonCode = "idle_state_unknown"
	ReasonUserActive              ReasonCode = "user_active"
)

var reasonMessages = map[ReasonCode]string{
	ReasonCompleted:               "maintenance completed within the bounded success boundary",
	ReasonReviewedNoChange:        "review completed with no bounded change proposal",
	ReasonRecoveryRequired:        "completion cleanup failed and requires operator recovery",
	ReasonRecoveryIntent:          "operator recovery intent durably attested",
	ReasonRecoveryCompleted:       "operator recovery completed after fenced removal",
	ReasonRecoveryFailed:          "operator recovery failed after durable intent",
	ReasonRecoveryAuditIncomplete: "recovery committed but completion audit needs repair",
	ReasonProposalEmitted:         "proposal emitted; approval and application remain separate",
	ReasonLeaseBusy:               "another bounded worker owns this occurrence",
	ReasonHandlerUnavailable:      "qualified handler is unavailable",
	ReasonHandlerFailure:          "qualified handler returned a recoverable failure",
	ReasonDeadlineExceeded:        "qualified handler exceeded its explicit deadline",
	ReasonCatalogUnavailable:      "qualified local handler is not enrolled",
	ReasonAuthorityRejected:       "local occurrence authority rejected the command",
	ReasonOccurrenceRejected:      "bounded wake command was rejected",
	ReasonReceiptPersisted:        "metadata receipt was durably recorded",
	ReasonIdleUnknown:             "idle state is unknown and therefore not eligible",
	ReasonUserActive:              "user activity suppresses background continuity work",
}

func validReasonCode(code ReasonCode) bool { _, ok := reasonMessages[code]; return ok }
func reasonMessage(code ReasonCode) string {
	if value, ok := reasonMessages[code]; ok {
		return value
	}
	return reasonMessages[ReasonHandlerFailure]
}
