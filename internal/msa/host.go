package msa

import (
	"strconv"
	"strings"
)

type Host struct {
	Name         string
	DurableID    string
	SerialNumber string
	HostGroup    string
	GroupKey     string
	MemberCount  int
	Properties   map[string]string
}

func HostsFromResponse(response Response) []Host {
	hosts := make([]Host, 0)
	for _, obj := range response.ObjectsWithoutStatus() {
		if !isHostObject(obj) {
			continue
		}
		hosts = append(hosts, hostFromObject(obj))
	}
	return hosts
}

func isHostObject(obj Object) bool {
	return obj.BaseType == "host"
}

func hostFromObject(obj Object) Host {
	props := obj.PropertyMap()
	memberCount := 0
	if value := strings.TrimSpace(props["member-count"]); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			memberCount = parsed
		}
	}

	return Host{
		Name:         firstNonEmpty(props["name"], obj.Name),
		DurableID:    props["durable-id"],
		SerialNumber: props["serial-number"],
		HostGroup:    props["host-group"],
		GroupKey:     props["group-key"],
		MemberCount:  memberCount,
		Properties:   props,
	}
}
