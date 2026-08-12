package agentstate

import "errors"

var (
	errBudgetNonPositive       = errors.New("agentstate: rune budget must be positive")
	errSingleSectionOverBudget = errors.New("agentstate: newest section exceeds the rune budget")
	errRootRequired            = errors.New("agentstate: store root is required")
	errNonMonotonicTimestamp   = errors.New("agentstate: snapshot update timestamp precedes the active commit")
)
