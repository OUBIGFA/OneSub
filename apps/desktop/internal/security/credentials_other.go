//go:build !windows

package security

import "errors"

func SaveKey(endpoint, key string) error {
	return errors.New("当前平台的系统凭据存储尚未验收")
}
func ReadKey(endpoint string) (string, error) {
	return "", errors.New("当前平台的系统凭据存储尚未验收")
}
func DeleteKey(endpoint string) error {
	return errors.New("当前平台的系统凭据存储尚未验收")
}
