package provider

import (
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func stringOrEnv(value types.String, env string) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	if value.IsUnknown() {
		diags.AddError("Invalid configuration", env+" is unknown")
		return "", diags
	}

	if !value.IsNull() {
		return strings.TrimSpace(value.ValueString()), diags
	}

	return strings.TrimSpace(os.Getenv(env)), diags
}

func boolOrEnv(value types.Bool, env string) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	if value.IsUnknown() {
		diags.AddError("Invalid configuration", env+" is unknown")
		return false, diags
	}

	if !value.IsNull() {
		return value.ValueBool(), diags
	}

	envValue := strings.TrimSpace(os.Getenv(env))
	if envValue == "" {
		return false, diags
	}

	parsed, err := strconv.ParseBool(envValue)
	if err != nil {
		diags.AddError("Invalid configuration", env+" must be true or false")
		return false, diags
	}

	return parsed, diags
}
