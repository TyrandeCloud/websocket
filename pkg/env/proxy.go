package env

import (
	"fmt"
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"os"
	"strconv"
)

const (
	LllidanProxyType              = "LLLIDAN_PROXY_TYPE"
	LllidanProxyPodUploadInterval = "LLLIDAN_PROXY_POD_UPLOAD_INTERNAL"
)

// GetProxyType returns the run mode of the proxy
func GetProxyType() (string, error) {
	a := os.Getenv(LllidanProxyType)
	if a != "" {
		return a, nil
	}
	return "", fmt.Errorf("no env variable getting from '%s' in the container", LllidanProxyType)
}

// GetProxyPodUploadInterval returns the uploading interval of the pod statistic
func GetProxyPodUploadInterval(defaultValue int64) int64 {
	a := os.Getenv(LllidanProxyPodUploadInterval)
	if a == "" {
		return defaultValue
	}
	t, err := strconv.Atoi(a)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return defaultValue
	}
	return int64(t)
}
