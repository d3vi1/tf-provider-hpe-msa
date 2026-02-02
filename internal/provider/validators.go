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
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid initiator_id",
			"initiator_id must be a WWPN (hex, with or without separators) or an iSCSI name (iqn., eui., naa.).",
		)
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
	switch {
	case strings.HasPrefix(lower, "iqn."):
		return isValidIQN(trimmed)
	case strings.HasPrefix(lower, "eui."):
		return isValidEUI(trimmed)
	case strings.HasPrefix(lower, "naa."):
		return isValidNAA(trimmed)
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

func isValidIQN(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	lower := strings.ToLower(value)
	parts := strings.SplitN(lower, ":", 2)
	if len(parts) != 2 {
		return false
	}
	prefix := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(prefix, "iqn.") {
		return false
	}
	base := strings.TrimPrefix(prefix, "iqn.")
	dateAndAuth := strings.SplitN(base, ".", 2)
	if len(dateAndAuth) != 2 {
		return false
	}
	if len(dateAndAuth[0]) != 7 || dateAndAuth[0][4] != '-' {
		return false
	}
	year := dateAndAuth[0][:4]
	month := dateAndAuth[0][5:]
	if !isDigits(year) || !isDigits(month) {
		return false
	}
	if !isHostnameLike(dateAndAuth[1]) {
		return false
	}
	if strings.TrimSpace(parts[1]) == "" {
		return false
	}
	return true
}

func isValidEUI(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	body := strings.TrimPrefix(strings.ToLower(value), "eui.")
	return len(body) == 16 && isHexString(body)
}

func isValidNAA(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	body := strings.TrimPrefix(strings.ToLower(value), "naa.")
	if len(body) != 16 && len(body) != 32 {
		return false
	}
	return isHexString(body)
}

func isHexString(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func isDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func isHostnameLike(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			continue
		}
		return false
	}
	return true
}
