package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type initiatorIDValidator struct{}

const maxHostNameLength = 255
const maxHostGroupNameBytes = 32

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

type hostNameValidator struct{}

func (v hostNameValidator) Description(_ context.Context) string {
	return "Host name must be 1-255 characters after trimming whitespace."
}

func (v hostNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v hostNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
		return
	}

	trimmed := strings.TrimSpace(req.ConfigValue.ValueString())
	if trimmed == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid host_name",
			"host_name must be non-empty after trimming whitespace.",
		)
		return
	}

	if len([]rune(trimmed)) > maxHostNameLength {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid host_name",
			"host_name must be 255 characters or fewer.",
		)
	}
}

type hostNamesSetValidator struct{}

func (v hostNamesSetValidator) Description(_ context.Context) string {
	return "Host names must be non-empty after trimming whitespace and 255 characters or fewer."
}

func (v hostNamesSetValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v hostNamesSetValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
		return
	}

	var items []string
	diags := req.ConfigValue.ElementsAs(ctx, &items, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid hosts",
				"host names must be non-empty after trimming whitespace.",
			)
			return
		}
		if len([]rune(trimmed)) > maxHostNameLength {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid hosts",
				"host names must be 255 characters or fewer.",
			)
			return
		}
	}
}

type hostGroupNameValidator struct{}

func (v hostGroupNameValidator) Description(_ context.Context) string {
	return "Host group name must be 1-32 bytes after trimming whitespace and must not include \", . < or \\."
}

func (v hostGroupNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v hostGroupNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
		return
	}

	trimmed := strings.TrimSpace(req.ConfigValue.ValueString())
	if trimmed == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid host group name",
			"host group name must be non-empty after trimming whitespace.",
		)
		return
	}

	if len([]byte(trimmed)) > maxHostGroupNameBytes {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid host group name",
			"host group name must be 32 bytes or fewer.",
		)
		return
	}

	if strings.ContainsAny(trimmed, "\",.<\\\\") {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid host group name",
			"host group name cannot include \", . < or \\.",
		)
	}
}
