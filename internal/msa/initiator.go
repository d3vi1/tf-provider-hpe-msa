package msa

import "strings"

type Initiator struct {
	ID          string
	Nickname    string
	Profile     string
	HostID      string
	HostKey     string
	HostBusType string
	Discovered  string
	Mapped      string
	Properties  map[string]string
}

func InitiatorsFromResponse(response Response) []Initiator {
	initiators := make([]Initiator, 0)
	for _, obj := range response.ObjectsWithoutStatus() {
		if !isInitiatorObject(obj) {
			continue
		}
		initiators = append(initiators, initiatorFromObject(obj))
	}
	return initiators
}

func isInitiatorObject(obj Object) bool {
	if obj.BaseType == "initiator" {
		return true
	}
	_, ok := obj.PropertyValue("id")
	return ok
}

func initiatorFromObject(obj Object) Initiator {
	props := obj.PropertyMap()

	return Initiator{
		ID:          strings.TrimSpace(props["id"]),
		Nickname:    strings.TrimSpace(props["nickname"]),
		Profile:     strings.TrimSpace(props["profile"]),
		HostID:      strings.TrimSpace(props["host-id"]),
		HostKey:     strings.TrimSpace(props["host-key"]),
		HostBusType: strings.TrimSpace(props["host-bus-type"]),
		Discovered:  strings.TrimSpace(props["discovered"]),
		Mapped:      strings.TrimSpace(props["mapped"]),
		Properties:  props,
	}
}
