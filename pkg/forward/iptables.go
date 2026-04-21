package forward

import (
	"net"
	"regexp"
	"strconv"
	"strings"
)

const portCommentPrefix = "nvair cli port:"

var (
	portCommentPattern   = regexp.MustCompile(`nvair cli port:\s*([0-9]{1,5})`)
	toDestinationPattern = regexp.MustCompile(`--to-destination '?([^'\s]+)'?`)
)

// IPTablesTarget is the parsed DNAT destination for a managed forward rule.
type IPTablesTarget struct {
	Host string
	Port int
}

func PortComment(listenPort int) string {
	return portCommentPrefix + " " + strconv.Itoa(listenPort)
}

func ParseCommentPort(line string) (int, bool) {
	match := portCommentPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return 0, false
	}

	port, err := strconv.Atoi(match[1])
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

func ParseCommentPorts(output string) map[int]struct{} {
	ports := make(map[int]struct{})
	for _, match := range portCommentPattern.FindAllStringSubmatch(output, -1) {
		if len(match) != 2 {
			continue
		}
		port, err := strconv.Atoi(match[1])
		if err == nil && port > 0 && port <= 65535 {
			ports[port] = struct{}{}
		}
	}
	return ports
}

func ParseToDestination(line string) (IPTablesTarget, bool) {
	match := toDestinationPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return IPTablesTarget{}, false
	}

	host, portText, err := net.SplitHostPort(match[1])
	if err != nil {
		colonIdx := strings.LastIndex(match[1], ":")
		if colonIdx <= 0 || colonIdx >= len(match[1])-1 {
			return IPTablesTarget{}, false
		}
		host = match[1][:colonIdx]
		portText = match[1][colonIdx+1:]
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return IPTablesTarget{}, false
	}
	return IPTablesTarget{Host: strings.Trim(host, "[]"), Port: port}, true
}

func ParseIPTablesTargets(output string) map[int]IPTablesTarget {
	targets := make(map[int]IPTablesTarget)
	for _, line := range strings.Split(output, "\n") {
		listenPort, ok := ParseCommentPort(line)
		if !ok {
			continue
		}
		target, ok := ParseToDestination(line)
		if ok {
			targets[listenPort] = target
		}
	}
	return targets
}
