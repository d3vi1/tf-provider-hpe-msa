package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type volumeDeleteProbeClient interface {
	Execute(ctx context.Context, parts ...string) (msa.Response, error)
}

func preDeleteVolumeUsageGuardrail(ctx context.Context, client volumeDeleteProbeClient, resourceKind string, hints ...string) (volumeDeleteGuardrail, bool) {
	if client == nil {
		return volumeDeleteGuardrail{}, false
	}

	identities := volumeIdentityHints(hints...)
	if len(identities) == 0 {
		return volumeDeleteGuardrail{}, false
	}

	resourceKind = strings.TrimSpace(resourceKind)
	if resourceKind == "" {
		resourceKind = "volume"
	}
	resourceLabel := titleCaseWord(resourceKind)
	targetLabel := identities[0]

	mappingCount, mappingCommand, mappingErr := probeVolumeMappings(ctx, client, identities)
	if mappingErr != nil {
		if errors.Is(mappingErr, context.Canceled) || errors.Is(mappingErr, context.DeadlineExceeded) {
			return volumeDeleteGuardrail{
				summary:   fmt.Sprintf("%s deletion interrupted", resourceLabel),
				detail:    withDeleteClassification(true, fmt.Sprintf("Pre-delete mapping probe was interrupted before deletion could continue: %v", mappingErr)),
				retryable: true,
			}, true
		}
		tflog.Warn(ctx, "Volume pre-delete mapping probe failed; falling back to delete command", map[string]any{
			"resource_kind": resourceKind,
			"target":        targetLabel,
			"error":         mappingErr.Error(),
		})
	}
	if mappingCount > 0 {
		return volumeDeleteGuardrail{
			summary: fmt.Sprintf("%s deletion blocked: mapped", resourceLabel),
			detail: withDeleteClassification(false, fmt.Sprintf(
				"%s %q is still mapped (%d %s detected via `%s`). Remove related `hpe_msa_volume_mapping` resources (or unmap directly on the array), then run `terraform apply` again.",
				resourceLabel,
				targetLabel,
				mappingCount,
				pluralize(mappingCount, "mapping entry", "mapping entries"),
				mappingCommand,
			)),
			retryable: false,
		}, true
	}

	copyJob, copyCommand, copyErr := probeActiveVolumeCopyJob(ctx, client, identities)
	if copyErr != nil {
		if errors.Is(copyErr, context.Canceled) || errors.Is(copyErr, context.DeadlineExceeded) {
			return volumeDeleteGuardrail{
				summary:   fmt.Sprintf("%s deletion interrupted", resourceLabel),
				detail:    withDeleteClassification(true, fmt.Sprintf("Pre-delete volume-copy probe was interrupted before deletion could continue: %v", copyErr)),
				retryable: true,
			}, true
		}
		tflog.Warn(ctx, "Volume pre-delete copy probe failed; falling back to delete command", map[string]any{
			"resource_kind": resourceKind,
			"target":        targetLabel,
			"error":         copyErr.Error(),
		})
	}
	if copyJob != nil {
		jobContext := copyJobContext(copyJob)
		return volumeDeleteGuardrail{
			summary: fmt.Sprintf("%s deletion blocked: active copy", resourceLabel),
			detail: withDeleteClassification(true, fmt.Sprintf(
				"%s %q is participating in an active volume-copy job (%s, discovered via `%s`). Wait for the copy to finish, then run `terraform apply` again.",
				resourceLabel,
				targetLabel,
				jobContext,
				copyCommand,
			)),
			retryable: true,
		}, true
	}

	connectionCount, connectionCommand, connectionErr := probeActiveVolumeConnections(ctx, client, identities)
	if connectionErr != nil {
		if errors.Is(connectionErr, context.Canceled) || errors.Is(connectionErr, context.DeadlineExceeded) {
			return volumeDeleteGuardrail{
				summary:   fmt.Sprintf("%s deletion interrupted", resourceLabel),
				detail:    withDeleteClassification(true, fmt.Sprintf("Pre-delete connection/session probe was interrupted before deletion could continue: %v", connectionErr)),
				retryable: true,
			}, true
		}
		tflog.Warn(ctx, "Volume pre-delete connection/session probe failed; falling back to delete command", map[string]any{
			"resource_kind": resourceKind,
			"target":        targetLabel,
			"error":         connectionErr.Error(),
		})
	}
	if connectionCount > 0 {
		return volumeDeleteGuardrail{
			summary: fmt.Sprintf("%s deletion blocked: active sessions", resourceLabel),
			detail: withDeleteClassification(true, fmt.Sprintf(
				"%s %q still has active host/initiator connection %s (detected via `%s`). Disconnect active hosts or end sessions, then run `terraform apply` again.",
				resourceLabel,
				targetLabel,
				pluralize(connectionCount, "entry", "entries"),
				connectionCommand,
			)),
			retryable: true,
		}, true
	}

	return volumeDeleteGuardrail{}, false
}

func probeVolumeMappings(ctx context.Context, client volumeDeleteProbeClient, identities []string) (int, string, error) {
	commands := make([][]string, 0, len(identities)+1)
	for _, identity := range identities {
		commands = append(commands, []string{"show", "maps", "volume", identity})
	}
	commands = append(commands, []string{"show", "maps"})

	var lastErr error
	for _, parts := range commands {
		response, err := client.Execute(ctx, parts...)
		if err != nil {
			if isSkippableUsageProbeError(err) {
				continue
			}
			lastErr = err
			continue
		}

		count := 0
		for _, mapping := range msa.MappingsFromResponse(response) {
			if volumeIdentityMatches(mapping.Volume, identities) || volumeIdentityMatches(mapping.VolumeSerial, identities) {
				count++
			}
		}
		if count > 0 {
			return count, strings.Join(parts, " "), nil
		}
	}

	return 0, "", lastErr
}

func probeActiveVolumeCopyJob(ctx context.Context, client volumeDeleteProbeClient, identities []string) (*msa.VolumeCopyJob, string, error) {
	commands := [][]string{
		{"show", "volume-copy"},
		{"show", "volume-copies"},
	}

	var lastErr error
	for _, parts := range commands {
		response, err := client.Execute(ctx, parts...)
		if err != nil {
			if isSkippableUsageProbeError(err) {
				continue
			}
			lastErr = err
			continue
		}

		jobs := msa.VolumeCopyJobsFromResponse(response)
		for i := range jobs {
			job := jobs[i]
			if !job.Active {
				continue
			}
			if volumeIdentityMatches(job.Source, identities) || volumeIdentityMatches(job.Target, identities) {
				candidate := job
				return &candidate, strings.Join(parts, " "), nil
			}
		}
	}

	return nil, "", lastErr
}

func probeActiveVolumeConnections(ctx context.Context, client volumeDeleteProbeClient, identities []string) (int, string, error) {
	commands := make([][]string, 0, len(identities)*2+3)
	for _, identity := range identities {
		commands = append(commands, []string{"show", "connections", "volume", identity})
		commands = append(commands, []string{"show", "sessions", "volume", identity})
	}
	commands = append(commands,
		[]string{"show", "connections"},
		[]string{"show", "sessions"},
		[]string{"show", "host-connections"},
	)

	var lastErr error
	for _, parts := range commands {
		response, err := client.Execute(ctx, parts...)
		if err != nil {
			if isSkippableUsageProbeError(err) {
				continue
			}
			lastErr = err
			continue
		}

		count := activeVolumeConnectionCount(response, identities)
		if count > 0 {
			return count, strings.Join(parts, " "), nil
		}
	}

	return 0, "", lastErr
}

func activeVolumeConnectionCount(response msa.Response, identities []string) int {
	count := 0
	for _, obj := range response.ObjectsWithoutStatus() {
		props := obj.PropertyMap()
		if !isConnectionOrSessionObject(obj, props) {
			continue
		}
		if !connectionObjectReferencesVolume(props, identities) {
			continue
		}
		if !connectionObjectActive(props) {
			continue
		}
		count++
	}
	return count
}

func isConnectionOrSessionObject(obj msa.Object, props map[string]string) bool {
	shape := strings.ToLower(strings.TrimSpace(obj.BaseType + " " + obj.Name))
	if containsAny(shape, "connection", "session") {
		return true
	}
	return hasPropertyKeyContaining(props, "connection", "session")
}

func connectionObjectReferencesVolume(props map[string]string, identities []string) bool {
	for key, value := range props {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if lowerKey == "" {
			continue
		}
		if containsAny(lowerKey, "volume", "serial", "durable", "wwn", "wwid") {
			if volumeIdentityMatches(value, identities) {
				return true
			}
			continue
		}
		if lowerKey == "name" && volumeIdentityMatches(value, identities) {
			if hasPropertyKeyContaining(props, "volume", "serial", "durable", "lun") {
				return true
			}
		}
	}
	return false
}

func connectionObjectActive(props map[string]string) bool {
	statusValues := make([]string, 0)
	for key, value := range props {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if !containsAny(lowerKey, "status", "state", "session", "connection", "login") {
			continue
		}
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		statusValues = append(statusValues, trimmed)
	}

	if len(statusValues) == 0 {
		return true
	}
	for _, status := range statusValues {
		if containsAny(status, "disconnected", "logged out", "logout", "inactive", "offline", "closed", "down", "failed", "not connected", "no session") {
			return false
		}
	}
	return true
}

func hasPropertyKeyContaining(props map[string]string, candidates ...string) bool {
	for key := range props {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if containsAny(lowerKey, candidates...) {
			return true
		}
	}
	return false
}

func volumeIdentityHints(values ...string) []string {
	seen := map[string]struct{}{}
	identities := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		identities = append(identities, trimmed)
	}
	return identities
}

func volumeIdentityMatches(value string, identities []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	normalizedValue := strings.ToLower(value)
	normalizedValueCompact := compactIdentity(normalizedValue)
	tokens := splitIdentityTokens(normalizedValue)

	for _, identity := range identities {
		identity = strings.TrimSpace(identity)
		if identity == "" {
			continue
		}
		normalizedIdentity := strings.ToLower(identity)
		if normalizedValue == normalizedIdentity {
			return true
		}

		if compactIdentity(normalizedIdentity) != "" && normalizedValueCompact == compactIdentity(normalizedIdentity) {
			return true
		}

		for _, token := range tokens {
			if token == normalizedIdentity {
				return true
			}
			if compactIdentity(token) != "" && compactIdentity(token) == compactIdentity(normalizedIdentity) {
				return true
			}
		}

		if len(normalizedIdentity) >= 8 && strings.Contains(normalizedValue, normalizedIdentity) {
			return true
		}
	}

	return false
}

func splitIdentityTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.')
	})
}

func compactIdentity(value string) string {
	replacer := strings.NewReplacer(":", "", "-", "", "_", "", ".", "", " ", "")
	return replacer.Replace(strings.TrimSpace(value))
}

func isSkippableUsageProbeError(err error) bool {
	return isUnsupportedUsageProbeError(err) || isNotFoundUsageProbeError(err)
}

func isUnsupportedUsageProbeError(err error) bool {
	message, ok := volumeProbeAPIErrorMessage(err)
	if !ok {
		return false
	}

	return containsAny(message,
		"invalid command",
		"unknown command",
		"unrecognized command",
		"command not recognized",
		"not supported",
		"unsupported",
		"not available",
		"syntax error",
		"invalid option",
		"illegal parameter",
		"invalid parameter",
	)
}

func isNotFoundUsageProbeError(err error) bool {
	message, ok := volumeProbeAPIErrorMessage(err)
	if !ok {
		return false
	}

	return containsAny(message,
		"no such volume",
		"volume does not exist",
		"no object",
		"not found",
		"does not exist",
	)
}

func volumeProbeAPIErrorMessage(err error) (string, bool) {
	var apiErr msa.APIError
	if !errors.As(err, &apiErr) {
		return "", false
	}

	message := strings.ToLower(strings.TrimSpace(apiErr.Status.Response))
	if message == "" {
		return "", false
	}

	return message, true
}

func copyJobContext(job *msa.VolumeCopyJob) string {
	if job == nil {
		return "job details unavailable"
	}

	parts := make([]string, 0, 4)
	if value := strings.TrimSpace(job.ID); value != "" {
		parts = append(parts, "job id="+value)
	}
	if value := strings.TrimSpace(job.Source); value != "" {
		parts = append(parts, "source="+value)
	}
	if value := strings.TrimSpace(job.Target); value != "" {
		parts = append(parts, "target="+value)
	}
	if job.HasETA {
		parts = append(parts, "eta="+job.ETA.String())
	}

	if len(parts) == 0 {
		return "job details unavailable"
	}
	return strings.Join(parts, " ")
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
