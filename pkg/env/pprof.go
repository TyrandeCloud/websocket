package env

import (
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"os"
	"strconv"
)

const (
	LllidanGatewayPProfDebug = "LLLIDAN_GATEWAY_PPROF_DEBUG"
)

// GetGatewayPProfDebug determines that whether the gateway will open pprof routers
func GetGatewayPProfDebug() bool {
	a := os.Getenv(LllidanGatewayPProfDebug)
	if a == "" {
		return false
	}
	t, err := strconv.ParseBool(a)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return false
	}
	return t
}
