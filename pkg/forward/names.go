package forward

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

// ServiceName describes a parsed managed forward service name.
type ServiceName struct {
	ServiceType string
	ListenPort  int
	TargetPort  int
	TargetHost  string
}

// BuildServiceName returns the managed forward service name for a mapping.
func BuildServiceName(listenPort, targetPort int, targetHost string) string {
	targetHost = strings.TrimSpace(targetHost)
	if targetPort <= 0 {
		targetPort = listenPort
	}

	return fmt.Sprintf("forward-%d->%s:%d", listenPort, targetHost, targetPort)
}

// ParseServiceName parses managed forward names in the format:
// forward-{listenPort}->{targetHost}:{targetPort}
//
// It also accepts the legacy format:
// forward-{type}-{listenPort}->{targetHost}:{targetPort}
func ParseServiceName(name string) (ServiceName, bool) {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "forward-") {
		return ServiceName{}, false
	}

	body := strings.TrimPrefix(name, "forward-")
	if arrowIdx := strings.Index(body, "->"); arrowIdx > 0 {
		left := body[:arrowIdx]
		right := body[arrowIdx+2:]

		serviceType, listenPort, ok := parseServiceNameLeft(left)
		if ok {
			colonIdx := strings.LastIndex(right, ":")
			if colonIdx > 0 && colonIdx < len(right)-1 {
				targetHost := strings.TrimSpace(right[:colonIdx])
				targetPort, err := strconv.Atoi(right[colonIdx+1:])
				if targetHost != "" && err == nil && targetPort > 0 {
					return ServiceName{
						ServiceType: serviceType,
						ListenPort:  listenPort,
						TargetPort:  targetPort,
						TargetHost:  targetHost,
					}, true
				}
			}
		}
	}

	return ServiceName{}, false
}

func parseServiceNameLeft(left string) (string, int, bool) {
	left = strings.TrimSpace(left)
	if left == "" {
		return "", 0, false
	}

	if listenPort, err := strconv.Atoi(left); err == nil && listenPort > 0 {
		return "", listenPort, true
	}

	sep := strings.LastIndex(left, "-")
	if sep <= 0 || sep >= len(left)-1 {
		return "", 0, false
	}

	serviceType := strings.TrimSpace(left[:sep])
	listenPort, err := strconv.Atoi(left[sep+1:])
	if serviceType == "" || err != nil || listenPort <= 0 {
		return "", 0, false
	}

	return serviceType, listenPort, true
}

// BuildBastionSSHServiceName returns the managed name for the default bastion SSH service.
func BuildBastionSSHServiceName() string {
	return BuildServiceName(22, 22, constant.OOBMgmtServerName)
}

// IsBastionSSHServiceName reports whether the name identifies the default bastion SSH service.
func IsBastionSSHServiceName(name string) bool {
	name = strings.TrimSpace(name)
	if strings.EqualFold(name, constant.OOBMgmtServerName+" SSH") {
		return true
	}

	parsed, ok := ParseServiceName(name)
	return ok &&
		(parsed.ServiceType == "" || parsed.ServiceType == "ssh") &&
		parsed.ListenPort == 22 &&
		parsed.TargetPort == 22 &&
		parsed.TargetHost == constant.OOBMgmtServerName
}
