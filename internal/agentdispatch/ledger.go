package agentdispatch

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/reviewcustody"
)

// LedgerApprovalInput binds a Pilot approval to one current execution item.
// The review packet must have carried the same binding; callers cannot choose
// a different item after Walter approved.
type LedgerApprovalInput struct {
	WorkspaceID      string
	ItemID           string
	ExpectedRevision int
	AttemptID        string
}

// RecordApprovedWalterReview is the only bridge from the conversational
// Walter loop to the execution ledger. It requires a completed, typed approved
// Pilot receipt, the same packet's ledger binding and a custody signer whose
// installation identity matches the immutable execution contract.
func (pilot *Pilot) RecordApprovedWalterReview(store execution.Store, custody reviewcustody.Provider, sourceDelegationID string, body WalterReviewBody, input LedgerApprovalInput) (execution.WalterReviewResult, error) {
	if body.Verdict != WalterApproved {
		return execution.WalterReviewResult{}, errors.New("only a typed Walter approved verdict can enter the execution ledger")
	}
	for _, objection := range body.Objections {
		if objection.Blocking {
			return execution.WalterReviewResult{}, errors.New("a blocking Walter objection cannot enter the execution ledger as approval")
		}
	}
	receipt, ok := pilot.Inspect(sourceDelegationID)
	if !ok || receipt.State != StateCompleted || receipt.Review == nil || receipt.Review.State != ReviewApproved {
		return execution.WalterReviewResult{}, errors.New("Walter approval is not current and independently supported")
	}
	if receipt.Review.ExecutionWorkspaceID != input.WorkspaceID || receipt.Review.ExecutionItemID != input.ItemID {
		return execution.WalterReviewResult{}, errors.New("Walter approval is bound to a different execution item")
	}
	if !receipt.Review.WalterRequired || !receipt.Review.Trigger.valid() || receipt.Review.ChainSHA256 == "" {
		return execution.WalterReviewResult{}, errors.New("Walter approval is missing review-chain provenance")
	}
	if receipt.Review.AccountConsultationRequired != receipt.Review.PostAccountValidationRequired {
		return execution.WalterReviewResult{}, errors.New("Walter approval has an invalid Account consultation invariant")
	}
	if body.Intent.IntentPacketSHA256 == "" || body.Intent.IntentPacketSHA256 != receipt.Review.IntentPacketSHA256 {
		return execution.WalterReviewResult{}, errors.New("Walter approval is bound to a different intent packet")
	}
	if receipt.Review.VerdictSHA256 == "" || receipt.Review.VerdictSHA256 != digestBody(normalizeWalterReviewBody(body)) {
		return execution.WalterReviewResult{}, errors.New("Walter approval verdict does not match the authenticated review receipt")
	}
	if receipt.Review.AccountConsultationRequired && (receipt.Review.ValidatedPacketID == "" || receipt.Review.ValidatedPacketSHA256 == "") {
		return execution.WalterReviewResult{}, errors.New("Walter approval is missing Account validation provenance")
	}
	item, err := store.Inspect(input.WorkspaceID, input.ItemID)
	if err != nil {
		return execution.WalterReviewResult{}, err
	}
	if !item.Contract.RequireWalterReview || item.State.StateRevision != input.ExpectedRevision ||
		item.State.ActiveAttemptID != input.AttemptID {
		return execution.WalterReviewResult{}, errors.New("execution item is not at the reviewed revision and attempt")
	}
	if custody == nil {
		return execution.WalterReviewResult{}, reviewcustody.ErrUnavailable
	}
	signer, err := custody.Load(reviewcustody.WalterReviewScope)
	if err != nil {
		return execution.WalterReviewResult{}, err
	}
	if err := reviewcustody.ValidateSigner(signer); err != nil {
		return execution.WalterReviewResult{}, err
	}
	encodedPublicKey := base64.RawURLEncoding.EncodeToString(signer.PublicKey())
	if encodedPublicKey != item.Contract.WalterPublicKey || signer.KeyID() != item.Contract.WalterKeyID ||
		signer.InstallationID() != item.Contract.WalterInstallationID {
		return execution.WalterReviewResult{}, errors.New("Walter custody signer does not match the execution contract")
	}
	nonce, err := randomID()
	if err != nil {
		return execution.WalterReviewResult{}, err
	}
	now := time.Now().UTC()
	if store.Now != nil {
		now = store.Now().UTC()
	}
	envelope := execution.WalterReviewEnvelope{
		SchemaVersion: 1, ItemID: input.ItemID, WorkspaceID: input.WorkspaceID,
		AttemptID: input.AttemptID, ReviewedRevision: input.ExpectedRevision,
		ContractSHA256: item.State.ContractSHA256, SignerKeyID: signer.KeyID(),
		InstallationID: signer.InstallationID(), CustodyScope: signer.Scope(),
		Decision: execution.WalterReviewApproved, Nonce: nonce, IssuedAt: now,
	}
	payload, err := execution.WalterReviewSigningPayload(envelope)
	if err != nil {
		return execution.WalterReviewResult{}, err
	}
	signature, err := signer.Sign(payload)
	if err != nil {
		return execution.WalterReviewResult{}, err
	}
	envelope.Signature = base64.RawStdEncoding.EncodeToString(signature)
	result, err := store.RecordWalterReview(input.WorkspaceID, input.ItemID, execution.WalterReviewInput{
		ExpectedRevision: input.ExpectedRevision, AttemptID: input.AttemptID, Envelope: envelope,
	})
	if err != nil {
		return execution.WalterReviewResult{}, fmt.Errorf("record authenticated Walter approval: %w", err)
	}
	return result, nil
}
