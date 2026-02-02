package msa

type Mapping struct {
	Volume       string
	VolumeSerial string
	LUN          string
	Access       string
	Ports        string
	Properties   map[string]string
}

func MappingsFromResponse(response Response) []Mapping {
	mappings := make([]Mapping, 0)
	for _, obj := range response.ObjectsWithoutStatus() {
		props := obj.PropertyMap()
		volume := firstNonEmpty(props["volume"], props["volume-name"], props["name"])
		if volume == "" {
			continue
		}
		if props["lun"] == "" {
			continue
		}

		mappings = append(mappings, Mapping{
			Volume:       volume,
			VolumeSerial: firstNonEmpty(props["volume-serial"], props["serial-number"]),
			LUN:          props["lun"],
			Access:       props["access"],
			Ports:        props["ports"],
			Properties:   props,
		})
	}
	return mappings
}
