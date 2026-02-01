package msa

import "strings"

type Volume struct {
	Name         string
	SerialNumber string
	DurableID    string
	PoolName     string
	VDiskName    string
	Size         string
	SizeNumeric  string
	Properties   map[string]string
}

func VolumesFromResponse(response Response) []Volume {
	volumes := make([]Volume, 0)
	for _, obj := range response.ObjectsWithoutStatus() {
		if !isVolumeObject(obj) {
			continue
		}
		volumes = append(volumes, volumeFromObject(obj))
	}
	return volumes
}

func isVolumeObject(obj Object) bool {
	if obj.BaseType == "volumes" {
		return true
	}
	_, ok := obj.PropertyValue("volume-name")
	return ok
}

func volumeFromObject(obj Object) Volume {
	props := obj.PropertyMap()

	return Volume{
		Name:         firstNonEmpty(props["volume-name"], props["name"], obj.Name),
		SerialNumber: props["serial-number"],
		DurableID:    props["durable-id"],
		PoolName:     firstNonEmpty(props["storage-pool-name"], props["storage-poolname"], props["pool-name"]),
		VDiskName:    firstNonEmpty(props["virtual-disk-name"], props["virtual-diskname"], props["vdisk-name"]),
		Size:         props["size"],
		SizeNumeric:  props["size-numeric"],
		Properties:   props,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
