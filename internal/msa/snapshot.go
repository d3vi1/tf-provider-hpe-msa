package msa

type Snapshot struct {
	Name           string
	SerialNumber   string
	DurableID      string
	BaseVolumeName string
	PoolName       string
	VDiskName      string
	Size           string
	SizeNumeric    string
	Properties     map[string]string
}

func SnapshotsFromResponse(response Response) []Snapshot {
	snapshots := make([]Snapshot, 0)
	for _, obj := range response.ObjectsWithoutStatus() {
		if !isSnapshotObject(obj) {
			continue
		}
		snapshots = append(snapshots, snapshotFromObject(obj))
	}
	return snapshots
}

func isSnapshotObject(obj Object) bool {
	if obj.BaseType == "snapshots" {
		return true
	}
	if _, ok := obj.PropertyValue("base-volume"); ok {
		return true
	}
	if _, ok := obj.PropertyValue("master-volume-name"); ok {
		return true
	}
	if _, ok := obj.PropertyValue("volume-parent"); ok {
		return true
	}
	return false
}

func snapshotFromObject(obj Object) Snapshot {
	props := obj.PropertyMap()

	return Snapshot{
		Name:           firstNonEmpty(props["name"], obj.Name),
		SerialNumber:   props["serial-number"],
		DurableID:      props["durable-id"],
		BaseVolumeName: firstNonEmpty(props["base-volume"], props["master-volume-name"], props["volume-parent"]),
		PoolName:       firstNonEmpty(props["storage-pool-name"], props["storage-poolname"], props["pool-name"]),
		VDiskName:      firstNonEmpty(props["virtual-disk-name"], props["virtual-diskname"], props["vdisk-name"]),
		Size:           firstNonEmpty(props["total-size"], props["size"]),
		SizeNumeric:    firstNonEmpty(props["total-size-numeric"], props["size-numeric"]),
		Properties:     props,
	}
}
