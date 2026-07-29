package priorwork

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

// BuildUnsignedImportReceipt returns the canonical body a trusted external
// collector must sign. It never accepts, loads or stores a private key.
func BuildUnsignedImportReceipt(
	snapshot Snapshot,
	enrollment Enrollment,
	receiptID string,
	emittedAt time.Time,
) (ImportReceipt, []byte, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return ImportReceipt{}, nil, err
	}
	if err := ValidateEnrollment(enrollment); err != nil {
		return ImportReceipt{}, nil, err
	}
	if !opaqueRefPattern.MatchString(receiptID) {
		return ImportReceipt{}, nil, errors.New("invalid prior-work receipt ID")
	}
	snapshotDigest, err := fingerprintSnapshot(snapshot)
	if err != nil {
		return ImportReceipt{}, nil, err
	}
	enrollmentFingerprint, err := fingerprintEnrollment(enrollment)
	if err != nil {
		return ImportReceipt{}, nil, err
	}
	receipt := ImportReceipt{
		SchemaVersion: 1, ReceiptID: receiptID, EvidenceClass: "adapter_command",
		Capability: "sharepoint_work_collection", ProducerRuntime: "claude",
		Outcome: "succeeded", EmittedAt: emittedAt.UTC(),
		TenantRef: snapshot.TenantRef, Roots: append([]RootRef(nil), snapshot.Roots...),
		PolicyVersion: enrollment.PolicyVersion, EnrollmentFingerprint: enrollmentFingerprint,
		CollectionSequence: snapshot.CollectionSequence, Watermark: snapshot.Watermark,
		SnapshotDigest: snapshotDigest, KeyID: enrollment.CollectorKeyID,
	}
	body, err := receiptSigningBody(receipt)
	if err != nil {
		return ImportReceipt{}, nil, err
	}
	return receipt, body, nil
}

func VerifyImportReceipt(receipt ImportReceipt, snapshot Snapshot, enrollment Enrollment) error {
	if err := ValidateImportReceipt(receipt, snapshot); err != nil {
		return err
	}
	enrollmentFingerprint, err := fingerprintEnrollment(enrollment)
	if err != nil {
		return err
	}
	if receipt.PolicyVersion != enrollment.PolicyVersion ||
		receipt.EnrollmentFingerprint != enrollmentFingerprint ||
		receipt.KeyID != enrollment.CollectorKeyID {
		return errors.New("SharePoint import receipt does not bind the active enrollment")
	}
	publicKey, err := base64.StdEncoding.Strict().DecodeString(enrollment.CollectorPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("prior-work collector public key is unavailable")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(receipt.Signature)
	if err != nil {
		return errors.New("SharePoint import receipt signature is malformed")
	}
	body, err := receiptSigningBody(receipt)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), body, signature) {
		return errors.New("SharePoint import receipt authentication failed")
	}
	return nil
}

func receiptSigningBody(receipt ImportReceipt) ([]byte, error) {
	unsigned := receipt
	unsigned.Signature = ""
	return json.Marshal(unsigned)
}
