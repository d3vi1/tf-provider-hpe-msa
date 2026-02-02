package msa

import "strings"

func (o Object) PropertyMap() map[string]string {
	props := make(map[string]string, len(o.Properties))
	for _, prop := range o.Properties {
		props[prop.Name] = strings.TrimSpace(prop.Value)
	}
	return props
}

func (o Object) AllObjects() []Object {
	objects := make([]Object, 0, len(o.Objects))
	for _, obj := range o.Objects {
		objects = append(objects, obj)
		objects = append(objects, obj.AllObjects()...)
	}
	return objects
}

func (r Response) AllObjects() []Object {
	objects := make([]Object, 0, len(r.Objects))
	for _, obj := range r.Objects {
		objects = append(objects, obj)
		objects = append(objects, obj.AllObjects()...)
	}
	return objects
}

func (r Response) ObjectsWithoutStatus() []Object {
	objects := make([]Object, 0, len(r.Objects))
	for _, obj := range r.AllObjects() {
		if obj.BaseType == "status" || obj.Name == "status" {
			continue
		}
		objects = append(objects, obj)
	}
	return objects
}
