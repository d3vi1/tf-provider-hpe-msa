package msa

import (
	"encoding/xml"
	"strconv"
	"strings"
)

type Response struct {
	XMLName xml.Name `xml:"RESPONSE"`
	Version string   `xml:"VERSION,attr"`
	Objects []Object `xml:"OBJECT"`
}

type Object struct {
	BaseType   string     `xml:"basetype,attr"`
	Name       string     `xml:"name,attr"`
	OID        string     `xml:"oid,attr"`
	Properties []Property `xml:"PROPERTY"`
}

type Property struct {
	Name  string `xml:"name,attr"`
	Type  string `xml:"type,attr"`
	Size  string `xml:"size,attr"`
	Value string `xml:",chardata"`
}

type Status struct {
	ResponseType        string
	ResponseTypeNumeric int
	Response            string
	ReturnCode          int
	ComponentID         string
	TimeStamp           string
}

func (o Object) PropertyValue(name string) (string, bool) {
	for _, prop := range o.Properties {
		if prop.Name == name {
			return strings.TrimSpace(prop.Value), true
		}
	}
	return "", false
}

func (r Response) Status() (Status, bool) {
	for _, obj := range r.Objects {
		if obj.BaseType == "status" || obj.Name == "status" {
			status := Status{}
			if value, ok := obj.PropertyValue("response-type"); ok {
				status.ResponseType = value
			}
			if value, ok := obj.PropertyValue("response-type-numeric"); ok {
				status.ResponseTypeNumeric = parseInt(value)
			}
			if value, ok := obj.PropertyValue("response"); ok {
				status.Response = value
			}
			if value, ok := obj.PropertyValue("return-code"); ok {
				status.ReturnCode = parseInt(value)
			}
			if value, ok := obj.PropertyValue("component-id"); ok {
				status.ComponentID = value
			}
			if value, ok := obj.PropertyValue("time-stamp"); ok {
				status.TimeStamp = value
			}
			return status, true
		}
	}
	return Status{}, false
}

func (s Status) Success() bool {
	if s.ResponseTypeNumeric != 0 {
		return false
	}
	if s.ResponseType != "" && strings.ToLower(s.ResponseType) != "success" {
		return false
	}
	return true
}

func parseInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}
