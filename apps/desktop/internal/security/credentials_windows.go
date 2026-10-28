package security

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"golang.org/x/sys/windows"
)

var credentialDLL = windows.NewLazySystemDLL("advapi32.dll")
var credWrite = credentialDLL.NewProc("CredWriteW")
var credRead = credentialDLL.NewProc("CredReadW")
var credFree = credentialDLL.NewProc("CredFree")
var credDelete = credentialDLL.NewProc("CredDeleteW")

type nativeCredential struct {
	Flags          uint32
	Type           uint32
	TargetName     *uint16
	Comment        *uint16
	LastWritten    windows.Filetime
	BlobSize       uint32
	Blob           *byte
	Persist        uint32
	AttributeCount uint32
	Attributes     uintptr
	TargetAlias    *uint16
	UserName       *uint16
}

func SaveKey(endpoint, key string) error {
	if strings.TrimSpace(endpoint) == "" {
		return errors.New(locales.ErrCredentialsEndpoint())
	}
	if len(key) > 2560 || strings.ContainsAny(key, "\r\n") {
		return errors.New(locales.ErrCredentialsKey())
	}
	if key == "" {
		return DeleteKey(endpoint)
	}
	target, err := windows.UTF16PtrFromString(credentialTarget(endpoint))
	if err != nil {
		return err
	}
	data := []byte(key)
	defer func() {
		for i := range data {
			data[i] = 0
		}
	}()
	credential := nativeCredential{Type: 1, TargetName: target, BlobSize: uint32(len(data)), Blob: &data[0], Persist: 2}
	result, _, callErr := credWrite.Call(uintptr(unsafe.Pointer(&credential)), 0)
	if result == 0 {
		return apperr.Wrap(locales.ErrCredentialsSave(callErr.Error()), callErr)
	}
	return nil
}
func ReadKey(endpoint string) (string, error) {
	target, err := windows.UTF16PtrFromString(credentialTarget(endpoint))
	if err != nil {
		return "", err
	}
	var credential *nativeCredential
	result, _, callErr := credRead.Call(uintptr(unsafe.Pointer(target)), 1, 0, uintptr(unsafe.Pointer(&credential)))
	if result == 0 {
		if errors.Is(callErr, syscall.Errno(1168)) {
			return "", nil
		}
		return "", apperr.Wrap(locales.ErrCredentialsRead(callErr.Error()), callErr)
	}
	defer credFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential.BlobSize == 0 {
		return "", nil
	}
	if credential.BlobSize > 2560 {
		return "", errors.New(locales.ErrCredentialsSize())
	}
	return string(unsafe.Slice(credential.Blob, int(credential.BlobSize))), nil
}
func DeleteKey(endpoint string) error {
	target, err := windows.UTF16PtrFromString(credentialTarget(endpoint))
	if err != nil {
		return err
	}
	result, _, callErr := credDelete.Call(uintptr(unsafe.Pointer(target)), 1, 0)
	if result == 0 && !errors.Is(callErr, syscall.Errno(1168)) {
		return callErr
	}
	return nil
}
