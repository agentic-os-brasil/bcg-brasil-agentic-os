//go:build !darwin

package releaseprovider

func NewNativeSecureStore() SecureStore {
	return UnavailableStore{}
}
