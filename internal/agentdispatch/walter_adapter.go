package agentdispatch

import (
	"errors"
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/reviewcustody"
)

// HandleWalterReview is the shared core called by both Claude and Codex
// adapters. The adapters do not interpret the packet, choose a verdict or
// create a second delegation path.
func HandleWalterReview(pilot *Pilot, dispatch Dispatch, body WalterReviewBody, custody reviewcustody.Provider) (Receipt, error) {
	if pilot == nil || dispatch.Packet.TargetAgentID != "walter" || dispatch.Packet.Review == nil {
		return Receipt{}, errors.New("Walter adapter handler requires a sealed direct Walter packet")
	}
	if custody == nil {
		return pilot.MarkWalterUnavailable(dispatch, "review_custody_unavailable")
	}
	signer, err := custody.Load(reviewcustody.WalterReviewScope)
	if err != nil {
		return pilot.MarkWalterUnavailable(dispatch, "review_custody_unavailable")
	}
	if err := reviewcustody.ValidateSigner(signer); err != nil {
		return pilot.MarkWalterUnavailable(dispatch, "review_custody_invalid")
	}
	capability := pilot.dispatcher.credentials["walter"]
	executor, err := NewExecutor(dispatch.Runtime, "walter", capability)
	if err != nil {
		return pilot.MarkWalterUnavailable(dispatch, "walter_runtime_unavailable")
	}
	envelope, err := executor.SealWalterReview(dispatch, body)
	if err != nil {
		return Receipt{}, fmt.Errorf("seal Walter verdict: %w", err)
	}
	return pilot.ReturnWalterReview(envelope, body)
}
