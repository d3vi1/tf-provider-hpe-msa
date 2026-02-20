package msa

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var showVolumeCopyCommands = [][]string{
	{"show", "volume-copy"},
	{"show", "volume-copies"},
}

var volumeCopyJobIDKeys = []string{
	"job-id",
	"copy-job-id",
	"serial-number",
	"id",
}

var volumeCopySourceKeys = []string{
	"source-volume-name",
	"source-volume",
	"source-name",
	"source",
	"base-volume",
	"base-volume-name",
	"master-volume-name",
}

var volumeCopyTargetKeys = []string{
	"destination-volume-name",
	"destination-volume",
	"destination-name",
	"destination",
	"target-volume-name",
	"target-volume",
	"target-name",
	"target",
	"volume-name",
	"name",
}

var volumeCopyStatusKeys = []string{
	"copy-status",
	"status",
	"state",
	"job-status",
	"progress-status",
}

var volumeCopyETAKeys = []string{
	"estimated-time-remaining",
	"estimated-time-to-completion",
	"estimated-time-left",
	"time-remaining",
	"time-to-complete",
	"remaining-time",
	"eta",
	"seconds-to-completion",
	"estimated-seconds-to-complete",
}

var volumeCopyProgressKeys = []string{
	"progress",
	"progress-percent",
	"copy-progress",
	"percent-complete",
}

type VolumeCopyJob struct {
	ID         string
	Source     string
	Target     string
	Status     string
	ETARaw     string
	ETA        time.Duration
	HasETA     bool
	Active     bool
	Properties map[string]string
}

func (c *Client) FindActiveVolumeCopyJob(ctx context.Context, sourceHint, targetHint string) (*VolumeCopyJob, error) {
	var commandErrs []error
	commandSucceeded := false

	for _, parts := range showVolumeCopyCommands {
		response, err := c.Execute(ctx, parts...)
		if err != nil {
			commandErrs = append(commandErrs, fmt.Errorf("%s: %w", strings.Join(parts, " "), err))
			continue
		}
		commandSucceeded = true

		jobs := VolumeCopyJobsFromResponse(response)
		job := selectBestActiveVolumeCopyJob(jobs, sourceHint, targetHint)
		if job != nil {
			return job, nil
		}
		continue
	}

	if commandSucceeded {
		return nil, nil
	}
	if len(commandErrs) == 0 {
		return nil, nil
	}
	return nil, errors.Join(commandErrs...)
}

func VolumeCopyJobsFromResponse(response Response) []VolumeCopyJob {
	jobs := make([]VolumeCopyJob, 0)
	for _, obj := range response.ObjectsWithoutStatus() {
		if !isVolumeCopyObject(obj) {
			continue
		}
		jobs = append(jobs, volumeCopyJobFromObject(obj))
	}
	return jobs
}

func selectBestActiveVolumeCopyJob(jobs []VolumeCopyJob, sourceHint, targetHint string) *VolumeCopyJob {
	normalizedSourceHint := strings.ToLower(strings.TrimSpace(sourceHint))
	normalizedTargetHint := strings.ToLower(strings.TrimSpace(targetHint))

	bestScore := -1
	var best *VolumeCopyJob
	for i := range jobs {
		job := jobs[i]
		if !job.Active {
			continue
		}

		score := 0
		if normalizedSourceHint != "" && matchesVolumeCopyHint(job.Source, normalizedSourceHint) {
			score += 4
		}
		if normalizedTargetHint != "" && matchesVolumeCopyHint(job.Target, normalizedTargetHint) {
			score += 6
		}
		if job.HasETA {
			score += 2
		}
		if strings.TrimSpace(job.ID) != "" {
			score++
		}

		if score > bestScore {
			bestScore = score
			candidate := job
			best = &candidate
		}
	}

	return best
}

func matchesVolumeCopyHint(value, hint string) bool {
	if hint == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), hint)
}

func isVolumeCopyObject(obj Object) bool {
	baseType := strings.ToLower(strings.TrimSpace(obj.BaseType))
	name := strings.ToLower(strings.TrimSpace(obj.Name))
	if strings.Contains(baseType, "volume-copy") || strings.Contains(name, "volume-copy") {
		return true
	}
	if strings.Contains(baseType, "copy") && strings.Contains(baseType, "volume") {
		return true
	}

	props := obj.PropertyMap()
	return hasAnyProperty(props, volumeCopySourceKeys...) && hasAnyProperty(props, volumeCopyTargetKeys...)
}

func volumeCopyJobFromObject(obj Object) VolumeCopyJob {
	props := obj.PropertyMap()
	etaRaw := firstPropertyValue(props, volumeCopyETAKeys...)
	eta, hasETA := parseVolumeCopyETA(etaRaw)
	status := firstPropertyValue(props, volumeCopyStatusKeys...)

	job := VolumeCopyJob{
		ID:         firstNonEmpty(firstPropertyValue(props, volumeCopyJobIDKeys...), strings.TrimSpace(obj.OID)),
		Source:     firstPropertyValue(props, volumeCopySourceKeys...),
		Target:     firstPropertyValue(props, volumeCopyTargetKeys...),
		Status:     status,
		ETARaw:     etaRaw,
		ETA:        eta,
		HasETA:     hasETA,
		Active:     isVolumeCopyJobActive(status, props),
		Properties: props,
	}

	return job
}

func hasAnyProperty(props map[string]string, keys ...string) bool {
	for _, key := range keys {
		if value := strings.TrimSpace(props[key]); value != "" {
			return true
		}
	}
	return false
}

func firstPropertyValue(props map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(props[key]); value != "" {
			return value
		}
	}
	return ""
}

func isVolumeCopyJobActive(status string, props map[string]string) bool {
	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	if normalizedStatus != "" {
		for _, terminal := range []string{
			"complete",
			"completed",
			"success",
			"succeeded",
			"failed",
			"failure",
			"error",
			"aborted",
			"canceled",
			"cancelled",
			"stopped",
			"done",
		} {
			if strings.Contains(normalizedStatus, terminal) {
				return false
			}
		}

		for _, active := range []string{
			"progress",
			"running",
			"copy",
			"active",
			"queued",
			"pending",
			"starting",
			"in-progress",
		} {
			if strings.Contains(normalizedStatus, active) {
				return true
			}
		}
	}

	progress := firstPropertyValue(props, volumeCopyProgressKeys...)
	if percent, ok := parseProgressPercent(progress); ok {
		return percent < 100
	}

	return true
}

func parseProgressPercent(value string) (float64, bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(value, "%"))
	if trimmed == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func parseVolumeCopyETA(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	normalized := strings.ToLower(value)
	switch normalized {
	case "n/a", "na", "none", "unknown", "-", "--":
		return 0, false
	}

	if parsed, ok := parseColonDuration(value); ok {
		return parsed, true
	}

	if parsed, err := strconv.Atoi(value); err == nil {
		if parsed < 0 {
			return 0, false
		}
		return time.Duration(parsed) * time.Second, true
	}

	compact := strings.ReplaceAll(normalized, " ", "")
	if parsed, err := time.ParseDuration(compact); err == nil {
		return parsed, true
	}

	if parsed, ok := parseHumanDuration(normalized); ok {
		return parsed, true
	}

	return 0, false
}

func parseColonDuration(value string) (time.Duration, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, false
	}

	values := make([]int, len(parts))
	for i, part := range parts {
		parsed, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || parsed < 0 {
			return 0, false
		}
		values[i] = parsed
	}

	if len(values) == 2 {
		return time.Duration(values[0])*time.Minute + time.Duration(values[1])*time.Second, true
	}

	return time.Duration(values[0])*time.Hour +
		time.Duration(values[1])*time.Minute +
		time.Duration(values[2])*time.Second, true
}

func parseHumanDuration(value string) (time.Duration, bool) {
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return 0, false
	}

	total := time.Duration(0)
	matched := false
	for i := 0; i < len(fields)-1; i++ {
		amount, err := strconv.Atoi(strings.TrimSpace(fields[i]))
		if err != nil || amount < 0 {
			continue
		}

		unit := strings.Trim(strings.TrimSpace(fields[i+1]), ",")
		switch {
		case strings.HasPrefix(unit, "d"):
			total += time.Duration(amount) * 24 * time.Hour
		case strings.HasPrefix(unit, "h"):
			total += time.Duration(amount) * time.Hour
		case strings.HasPrefix(unit, "m"):
			total += time.Duration(amount) * time.Minute
		case strings.HasPrefix(unit, "s"):
			total += time.Duration(amount) * time.Second
		default:
			continue
		}

		matched = true
		i++
	}

	if !matched {
		return 0, false
	}

	return total, true
}
