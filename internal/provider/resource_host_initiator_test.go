package provider

import (
	"testing"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
)

func TestInitiatorMatchesHost(t *testing.T) {
	host := msa.Host{DurableID: "H1", SerialNumber: "SERIAL1"}
	initiator := &msa.Initiator{HostKey: "H1", HostID: "SERIAL1"}
	if !initiatorMatchesHost(initiator, host) {
		t.Fatalf("expected initiator to match host")
	}

	initiator = &msa.Initiator{HostKey: "H2", HostID: "SERIAL2"}
	if initiatorMatchesHost(initiator, host) {
		t.Fatalf("expected initiator not to match host")
	}
}
