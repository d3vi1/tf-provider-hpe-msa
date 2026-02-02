package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type initiatorIDValidator struct{}

func (v initiatorIDValidator) Description(_ context.Context) string {
	return "Initiator ID must be a WWPN (hex, with or without separators) or an iSCSI name (iqn., eui., naa.)."
}

func (v initiatorIDValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v initiatorIDValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
		return
	}

	value := strings.TrimSpace(req.ConfigValue.ValueString())
	if value == "" {
		return
	}

	if !isValidInitiatorID(value) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid initiator_id",
			"initiator_id must be a WWPN (hex, with or without separators) or an iSCSI name (iqn., eui., naa.).",
		)
	}
}

func isValidInitiatorID(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "iqn.") || strings.HasPrefix(lower, "eui.") || strings.HasPrefix(lower, "naa.") {
		if len(trimmed) <= 4 {
			return false
		}
		if strings.ContainsAny(trimmed, " \t\r\n") {
			return false
		}
		return true
	}

	cleaned := strings.ReplaceAll(trimmed, ":", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, ".", "")
	if len(cleaned) != 16 {
		return false
	}
	for _, r := range cleaned {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
