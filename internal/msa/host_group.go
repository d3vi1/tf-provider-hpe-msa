package msa

import (
	"strconv"
	"strings"
)

type HostGroup struct {
	Name         string
	DurableID    string
	SerialNumber string
	MemberCount  int
	Hosts        []Host
	Properties   map[string]string
}

func HostGroupsFromResponse(response Response) []HostGroup {
	groups := make([]HostGroup, 0)
	for _, obj := range response.ObjectsWithoutStatus() {
		if !isHostGroupObject(obj) {
			continue
		}
		groups = append(groups, hostGroupFromObject(obj))
	}
	return groups
}

func isHostGroupObject(obj Object) bool {
	return obj.BaseType == "host-group"
}

func hostGroupFromObject(obj Object) HostGroup {
	props := obj.PropertyMap()
	memberCount := 0
	if value := strings.TrimSpace(props["member-count"]); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			memberCount = parsed
		}
	}

	hosts := make([]Host, 0)
	for _, child := range obj.AllObjects() {
		if !isHostObject(child) {
			continue
		}
		hosts = append(hosts, hostFromObject(child))
	}

	return HostGroup{
		Name:         firstNonEmpty(props["name"], obj.Name),
		DurableID:    props["durable-id"],
		SerialNumber: props["serial-number"],
		MemberCount:  memberCount,
		Hosts:        hosts,
		Properties:   props,
	}
}
