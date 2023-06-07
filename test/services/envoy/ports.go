package envoy

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"sync/atomic"

	"github.com/solo-io/gloo/test/ginkgo/parallel"
)

// NOTE TO DEVELOPERS:
// This file contains definitions for port values that the test suite will use
// Ideally these ports would be owned only by the envoy package.
// However, the challenge is that we have some default resources, which are created using the defaults package.
// Therefore, I tried to keep the defaults package as the source of truth, but allow for tests to reference
// the envoy package for the ports.

var (
	adminPort = uint32(20000)
	bindPort  = uint32(10080)

	HttpPort   = defaults.HttpPort
	HttpsPort  = defaults.HttpsPort
	TcpPort    = defaults.TcpPort
	HybridPort = defaults.HybridPort
)

func NextBindPort() uint32 {
	return AdvancePort(&bindPort)
}

func NextAdminPort() uint32 {
	return AdvancePort(&adminPort)
}

func AdvanceRequestPorts() {
	defaults.AdvanceRequestPorts()
}

func AdvancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(parallel.GetPortOffset())
}
