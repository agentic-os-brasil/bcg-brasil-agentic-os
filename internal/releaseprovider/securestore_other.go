//go:build !darwin && !windows

package releaseprovider

func NewNativeSecureStore() SecureStore {
	return UnavailableStore{}
}
