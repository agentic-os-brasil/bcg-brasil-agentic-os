package maintenance

type ReasonCode string

const (
	ReasonCompleted          ReasonCode = "completed"
	ReasonProposalEmitted    ReasonCode = "proposal_emitted"
	ReasonLeaseBusy          ReasonCode = "lease_busy"
	ReasonHandlerUnavailable ReasonCode = "handler_unavailable"
	ReasonHandlerFailure     ReasonCode = "handler_failure"
	ReasonDeadlineExceeded   ReasonCode = "deadline_exceeded"
	ReasonCatalogUnavailable ReasonCode = "catalog_unavailable"
	ReasonAuthorityRejected  ReasonCode = "authority_rejected"
	ReasonOccurrenceRejected ReasonCode = "occurrence_rejected"
	ReasonReceiptPersisted   ReasonCode = "receipt_persisted"
)

var reasonMessages = map[ReasonCode]string{
	ReasonCompleted:          "maintenance completed within the bounded success boundary",
	ReasonProposalEmitted:    "proposal emitted; approval and application remain separate",
	ReasonLeaseBusy:          "another bounded worker owns this occurrence",
	ReasonHandlerUnavailable: "qualified handler is unavailable",
	ReasonHandlerFailure:     "qualified handler returned a recoverable failure",
	ReasonDeadlineExceeded:   "qualified handler exceeded its explicit deadline",
	ReasonCatalogUnavailable: "qualified local handler is not enrolled",
	ReasonAuthorityRejected:  "local occurrence authority rejected the command",
	ReasonOccurrenceRejected: "bounded wake command was rejected",
	ReasonReceiptPersisted:   "metadata receipt was durably recorded",
}

func validReasonCode(code ReasonCode) bool { _, ok := reasonMessages[code]; return ok }
func reasonMessage(code ReasonCode) string {
	if value, ok := reasonMessages[code]; ok {
		return value
	}
	return reasonMessages[ReasonHandlerFailure]
}
