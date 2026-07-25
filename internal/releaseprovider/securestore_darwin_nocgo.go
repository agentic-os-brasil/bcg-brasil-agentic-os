//go:build darwin && !cgo

package releaseprovider

func NewNativeSecureStore() SecureStore {
	return UnavailableStore{}
}
