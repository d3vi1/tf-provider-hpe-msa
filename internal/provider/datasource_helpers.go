package provider

import (
	"fmt"
	"strings"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func findObjectByName(response msa.Response, name string, keys []string, entity string) (msa.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	original := strings.TrimSpace(name)
	name = normalizeName(name)
	if name == "" {
		diags.AddError("Invalid name", "name must not be empty")
		return msa.Object{}, diags
	}

	for _, obj := range response.ObjectsWithoutStatus() {
		props := obj.PropertyMap()
		candidates := append([]string{obj.Name}, propertyValues(props, keys)...)
		for _, candidate := range candidates {
			if normalizeName(candidate) == name {
				return obj, diags
			}
		}
	}

	diags.AddError(title(entity)+" not found", fmt.Sprintf("No %s named %q was returned by the array", entity, original))
	return msa.Object{}, diags
}

func propertyValues(props map[string]string, keys []string) []string {
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		if value := props[key]; value != "" {
			values = append(values, value)
		}
	}
	return values
}

func normalizeName(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func title(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
