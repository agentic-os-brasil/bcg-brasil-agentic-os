//go:build darwin && cgo

package releaseprovider

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>

static CFMutableDictionaryRef maestro_keychain_query(const char *service, const char *account) {
	CFStringRef serviceRef = CFStringCreateWithCString(kCFAllocatorDefault, service, kCFStringEncodingUTF8);
	CFStringRef accountRef = CFStringCreateWithCString(kCFAllocatorDefault, account, kCFStringEncodingUTF8);
	if (serviceRef == NULL || accountRef == NULL) {
		if (serviceRef != NULL) CFRelease(serviceRef);
		if (accountRef != NULL) CFRelease(accountRef);
		return NULL;
	}
	CFMutableDictionaryRef query = CFDictionaryCreateMutable(
		kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (query != NULL) {
		CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
		CFDictionarySetValue(query, kSecAttrService, serviceRef);
		CFDictionarySetValue(query, kSecAttrAccount, accountRef);
		CFDictionarySetValue(query, kSecUseDataProtectionKeychain, kCFBooleanTrue);
	}
	CFRelease(serviceRef);
	CFRelease(accountRef);
	return query;
}

static OSStatus maestro_keychain_get(
	const char *service,
	const char *account,
	void **output,
	long *outputLength,
	long maximumLength
) {
	*output = NULL;
	*outputLength = 0;
	CFMutableDictionaryRef query = maestro_keychain_query(service, account);
	if (query == NULL) return errSecAllocate;
	CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);

	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	CFRelease(query);
	if (status != errSecSuccess) return status;
	if (result == NULL || CFGetTypeID(result) != CFDataGetTypeID()) {
		if (result != NULL) CFRelease(result);
		return errSecDecode;
	}

	CFDataRef data = (CFDataRef)result;
	CFIndex length = CFDataGetLength(data);
	if (length <= 0 || length > (CFIndex)maximumLength) {
		CFRelease(result);
		return errSecDecode;
	}
	void *copy = malloc(length > 0 ? (size_t)length : 1);
	if (copy == NULL) {
		CFRelease(result);
		return errSecAllocate;
	}
	if (length > 0) {
		CFDataGetBytes(data, CFRangeMake(0, length), (UInt8 *)copy);
	}
	CFRelease(result);
	*output = copy;
	*outputLength = (long)length;
	return errSecSuccess;
}

static OSStatus maestro_keychain_put(
	const char *service,
	const char *account,
	const void *input,
	long inputLength
) {
	CFMutableDictionaryRef query = maestro_keychain_query(service, account);
	if (query == NULL) return errSecAllocate;
	CFDataRef data = CFDataCreate(kCFAllocatorDefault, (const UInt8 *)input, (CFIndex)inputLength);
	if (data == NULL) {
		CFRelease(query);
		return errSecAllocate;
	}
	CFMutableDictionaryRef attributes = CFDictionaryCreateMutable(
		kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (attributes == NULL) {
		CFRelease(data);
		CFRelease(query);
		return errSecAllocate;
	}
	CFDictionarySetValue(attributes, kSecValueData, data);
	CFDictionarySetValue(attributes, kSecAttrAccessible, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly);

	OSStatus status = SecItemUpdate(query, attributes);
	if (status == errSecItemNotFound) {
		CFDictionarySetValue(query, kSecValueData, data);
		CFDictionarySetValue(query, kSecAttrAccessible, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly);
		status = SecItemAdd(query, NULL);
	}
	CFRelease(attributes);
	CFRelease(data);
	CFRelease(query);
	return status;
}

static OSStatus maestro_keychain_delete(const char *service, const char *account) {
	CFMutableDictionaryRef query = maestro_keychain_query(service, account);
	if (query == NULL) return errSecAllocate;
	OSStatus status = SecItemDelete(query);
	CFRelease(query);
	return status;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

const (
	keychainSuccess               int32 = 0
	keychainUserCanceled          int32 = -128
	keychainNotAvailable          int32 = -25291
	keychainAuthFailed            int32 = -25293
	keychainItemNotFound          int32 = -25300
	keychainInteractionNotAllowed int32 = -25308
)

const maestroKeychainService = "com.bcg.maestro.private-release"

type macOSKeychainBackend struct {
	service string
}

func NewNativeSecureStore() SecureStore {
	return newNativeSecureStore(macOSKeychainBackend{service: maestroKeychainService})
}

func (backend macOSKeychainBackend) Available() error {
	return nil
}

func (backend macOSKeychainBackend) Get(key string) ([]byte, error) {
	service := C.CString(backend.service)
	account := C.CString(key)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))

	var output unsafe.Pointer
	var outputLength C.long
	status := int32(C.maestro_keychain_get(
		service,
		account,
		&output,
		&outputLength,
		C.long(maximumCredentialBytes),
	))
	if err := mapKeychainStatus("read", status); err != nil {
		return nil, err
	}
	if output == nil {
		return nil, errors.New("macOS Keychain returned an empty credential reference")
	}
	defer C.free(output)
	return C.GoBytes(output, C.int(outputLength)), nil
}

func (backend macOSKeychainBackend) Put(key string, value []byte) error {
	service := C.CString(backend.service)
	account := C.CString(key)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))

	status := int32(C.maestro_keychain_put(
		service,
		account,
		unsafe.Pointer(&value[0]),
		C.long(len(value)),
	))
	return mapKeychainStatus("write", status)
}

func (backend macOSKeychainBackend) Delete(key string) error {
	service := C.CString(backend.service)
	account := C.CString(key)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	return mapKeychainStatus("delete", int32(C.maestro_keychain_delete(service, account)))
}

func mapKeychainStatus(operation string, status int32) error {
	switch status {
	case keychainSuccess:
		return nil
	case keychainItemNotFound:
		return ErrCredentialNotFound
	case keychainUserCanceled, keychainNotAvailable, keychainAuthFailed, keychainInteractionNotAllowed:
		return fmt.Errorf("%w: macOS Keychain %s is unavailable", ErrSecureStoreUnavailable, operation)
	default:
		return fmt.Errorf("macOS Keychain %s failed with status %d", operation, status)
	}
}
