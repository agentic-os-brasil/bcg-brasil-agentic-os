package datahome

import "errors"

// InitializeFresh explicitly confirms a new, empty owner home. Resolve remains
// read-only. Existing migrated, populated or ambiguous state is never promoted.
// On an IO failure after creation, bootstrap-partial preserves all state for
// read-only inspection and support-assisted recovery; retry cannot promote it.
func InitializeFresh(options Options) (Result, error) {
	result, err := Resolve(options)
	if err != nil {
		return result, err
	}
	if !ownershipVerified() {
		return failed(result, "ownership-unverified", errors.New("fresh bootstrap requires verified ownership"))
	}
	if result.Writable && result.Diagnostic == "ready" && result.Origin == "fresh-confirmed" {
		return result, nil
	}
	if result.Origin != "fresh-unconfirmed" || result.Diagnostic != "fresh-unconfirmed" {
		return failed(result, "bootstrap-refused", errors.New("only a new or empty unconfirmed home can be initialized"))
	}
	if err = initializeFresh(options, result); err != nil {
		return failed(result, "bootstrap-partial", errors.New("bootstrap did not complete; preserve the selected home and locator, inspect read-only before support-assisted recovery"))
	}
	resolved, err := Resolve(options)
	if err != nil || !resolved.Writable {
		return failed(result, "bootstrap-partial", errors.New("bootstrap could not be verified; preserve the selected home and locator for read-only inspection"))
	}
	return resolved, nil
}
