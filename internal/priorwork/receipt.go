package priorwork

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
)

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
