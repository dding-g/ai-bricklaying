package worklog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"ai-bricklaying/internal/safeio"
)

const maxMachineRequestBytes = 1 << 20

// RunMachine executes the versioned JSON protocol used by generated skills.
func RunMachine(argv []string, stdin io.Reader, stdout io.Writer, _ io.Writer) int {
	command := strings.Join(argv, ".")
	if command == "" {
		command = "unknown"
	}
	service := NewService()

	switch command {
	case "daily.prepare":
		var request PrepareRequest
		if err := decodeRequest(stdin, &request); err != nil {
			return writeMachineError(stdout, command, validationError("invalid_request", err.Error()))
		}
		flow, err := service.Prepare(request)
		if err != nil {
			return writeMachineError(stdout, command, err)
		}
		return writeMachineSuccess(stdout, command, flow, false, nextActionForFlow(flow))

	case "daily.status":
		var request StatusRequest
		if err := decodeRequest(stdin, &request); err != nil {
			return writeMachineError(stdout, command, validationError("invalid_request", err.Error()))
		}
		flow, err := service.Status(request)
		if err != nil {
			return writeMachineError(stdout, command, err)
		}
		return writeMachineSuccess(stdout, command, flow, false, nextActionForFlow(flow))

	case "daily.disclose":
		var request DiscloseRequest
		if err := decodeRequest(stdin, &request); err != nil {
			return writeMachineError(stdout, command, validationError("invalid_request", err.Error()))
		}
		flow, includeEvidence, err := service.Disclose(request)
		if err != nil {
			return writeMachineError(stdout, command, err)
		}
		nextAction := "ask_user_to_add_work_manually"
		if includeEvidence && len(flow.Evidence) > 0 {
			nextAction = "generate_candidate_titles_from_untrusted_evidence"
		}
		return writeMachineSuccess(stdout, command, flow, includeEvidence, nextAction)

	case "daily.checkpoint":
		var request CheckpointRequest
		if err := decodeRequest(stdin, &request); err != nil {
			return writeMachineError(stdout, command, validationError("invalid_request", err.Error()))
		}
		flow, err := service.Checkpoint(request)
		if err != nil {
			return writeMachineError(stdout, command, err)
		}
		nextAction := "show_final_preview"
		if flow.Interview.NextQuestion != "" {
			nextAction = "ask_next_interview_question"
		}
		return writeMachineSuccess(stdout, command, flow, false, nextAction)

	case "daily.finalize":
		var request FinalizeRequest
		if err := decodeRequest(stdin, &request); err != nil {
			return writeMachineError(stdout, command, validationError("invalid_request", err.Error()))
		}
		flow, err := service.Finalize(request)
		if err != nil {
			return writeMachineError(stdout, command, err)
		}
		return writeMachineSuccess(stdout, command, flow, false, "complete")

	default:
		return writeMachineError(stdout, command, validationError("unknown_command", "machine command must be daily prepare, status, disclose, checkpoint, or finalize"))
	}
}

func decodeRequest(reader io.Reader, destination any) error {
	contents, err := io.ReadAll(io.LimitReader(reader, maxMachineRequestBytes+1))
	if err != nil {
		return fmt.Errorf("could not read stdin: %w", err)
	}
	if len(contents) > maxMachineRequestBytes {
		return fmt.Errorf("stdin JSON object exceeds %d bytes", maxMachineRequestBytes)
	}
	contents = bytes.TrimSpace(contents)
	if len(contents) == 0 {
		return errors.New("stdin must contain one JSON object")
	}
	if contents[0] != '{' {
		return errors.New("stdin must contain a non-null JSON object")
	}

	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("stdin must contain one JSON object")
		}
		return fmt.Errorf("stdin must contain one valid JSON object: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("stdin must contain exactly one JSON object")
		}
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	if version := requestVersion(destination); version != ProtocolVersion {
		return fmt.Errorf("protocol_version must equal %q", ProtocolVersion)
	}
	return nil
}

func requestVersion(request any) string {
	switch typed := request.(type) {
	case *PrepareRequest:
		return typed.ProtocolVersion
	case *StatusRequest:
		return typed.ProtocolVersion
	case *DiscloseRequest:
		return typed.ProtocolVersion
	case *CheckpointRequest:
		return typed.ProtocolVersion
	case *FinalizeRequest:
		return typed.ProtocolVersion
	default:
		return ""
	}
}

func writeMachineSuccess(writer io.Writer, command string, flow DailyFlow, includeEvidence bool, nextAction string) int {
	public := ProjectPublicFlow(flow)
	if includeEvidence {
		disclosed, err := ProjectDisclosedFlow(flow)
		if err != nil {
			return writeMachineError(writer, command, err)
		}
		public = disclosed
	}
	envelope := Envelope{
		ProtocolVersion: ProtocolVersion,
		Command:         command,
		OK:              true,
		FlowID:          flow.FlowID,
		Revision:        flow.Revision,
		State:           flow.State,
		NextAction:      nextAction,
		Flow:            &public,
	}
	if err := writeEnvelope(writer, envelope); err != nil {
		return 1
	}
	return 0
}

func writeMachineError(writer io.Writer, command string, err error) int {
	serviceErr := ServiceError{Code: "internal_error", Message: "machine operation failed", Internal: true}
	if !errors.As(err, &serviceErr) {
		serviceErr.Message = safeio.RedactString(err.Error())
	}
	envelope := Envelope{
		ProtocolVersion: ProtocolVersion,
		Command:         command,
		OK:              false,
		Revision:        serviceErr.LatestRevision,
		NextAction:      errorNextAction(serviceErr),
		Error: &MachineError{
			Code:           serviceErr.Code,
			Message:        safeio.RedactString(serviceErr.Message),
			Retryable:      serviceErr.Retryable,
			LatestRevision: serviceErr.LatestRevision,
		},
	}
	if writeErr := writeEnvelope(writer, envelope); writeErr != nil {
		return 1
	}
	if serviceErr.Internal {
		return 1
	}
	return 2
}

func writeEnvelope(writer io.Writer, envelope Envelope) error {
	contents, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "%s\n", contents)
	return err
}

// ProjectPublicFlow is the safe default projection for machine and future MCP
// adapters. Evidence and idempotency history are intentionally unavailable.
func ProjectPublicFlow(flow DailyFlow) PublicDailyFlow {
	return projectFlow(flow, false)
}

// ProjectDisclosedFlow returns evidence only after the service has persisted an
// affirmative consent decision.
func ProjectDisclosedFlow(flow DailyFlow) (PublicDailyFlow, error) {
	if flow.Consent.RemoteEvidence != "granted" {
		return PublicDailyFlow{}, validationError("consent_required", "evidence projection requires persisted affirmative consent")
	}
	return projectFlow(flow, true), nil
}

func projectFlow(flow DailyFlow, includeEvidence bool) PublicDailyFlow {
	consent := flow.Consent
	consent.RemoteEvidence = sanitizeText(consent.RemoteEvidence, 20)
	consent.UpdatedAt = cloneTimePointer(consent.UpdatedAt)
	interview, err := sanitizeInterview(flow.Interview)
	if err != nil {
		interview = Interview{}
	}
	public := PublicDailyFlow{
		SchemaVersion:       sanitizeText(flow.SchemaVersion, 20),
		FlowID:              sanitizeText(flow.FlowID, 128),
		Kind:                sanitizeText(flow.Kind, 40),
		Date:                sanitizeText(flow.Date, 20),
		Timezone:            sanitizeText(flow.Timezone, 100),
		Language:            NormalizeLanguage(flow.Language),
		State:               sanitizeText(flow.State, 40),
		Revision:            flow.Revision,
		OutputDir:           minimizePublicPath(flow.OutputDir),
		ConfigDir:           minimizePublicPath(flow.ConfigDir),
		StatePath:           minimizePublicPath(flow.StatePath),
		WorklogMarkdownPath: minimizePublicPath(flow.WorklogMarkdownPath),
		WorklogJSONPath:     minimizePublicPath(flow.WorklogJSONPath),
		Consent:             consent,
		Coverage:            sanitizePublicCoverage(flow.Coverage),
		WorkItems:           sanitizePublicWorkItems(flow.WorkItems),
		NoWorkConfirmed:     flow.NoWorkConfirmed,
		Reflection:          sanitizeReflection(flow.Reflection),
		Interview:           interview,
		CreatedAt:           flow.CreatedAt,
		UpdatedAt:           flow.UpdatedAt,
		ConfirmedAt:         cloneTimePointer(flow.ConfirmedAt),
	}
	if includeEvidence {
		public.Evidence = sanitizePublicEvidence(flow.Evidence)
	}
	return public
}

func sanitizePublicCoverage(coverage []SourceCoverage) []SourceCoverage {
	result := make([]SourceCoverage, 0, len(coverage))
	for _, item := range coverage {
		result = append(result, SourceCoverage{
			SourceKey:      sanitizeText(item.SourceKey, 80),
			SourceLabel:    sanitizeText(item.SourceLabel, 100),
			Status:         sanitizeText(item.Status, 40),
			CandidateFiles: item.CandidateFiles,
			UsedRecords:    item.UsedRecords,
			Reason:         sanitizeText(item.Reason, 500),
		})
	}
	return result
}

func sanitizePublicWorkItems(items []WorkItem) []WorkItem {
	result := make([]WorkItem, 0, len(items))
	for _, item := range items {
		result = append(result, WorkItem{
			ID:              sanitizeText(item.ID, 128),
			Title:           sanitizeText(item.Title, maxTitleChars),
			EvidenceSummary: sanitizeOneLine(item.EvidenceSummary, maxWorkItemNote),
			Uncertainty:     sanitizeOneLine(item.Uncertainty, maxWorkItemNote),
			Performed:       sanitizeText(item.Performed, maxFieldChars),
			Outcome:         sanitizeText(item.Outcome, maxFieldChars),
			Verification:    sanitizeText(item.Verification, maxFieldChars),
			Issues:          sanitizeText(item.Issues, maxFieldChars),
			EvidenceIDs:     deduplicateStrings(item.EvidenceIDs),
			Status:          sanitizeText(item.Status, 40),
			Origin:          sanitizeText(item.Origin, 40),
		})
	}
	return result
}

func sanitizePublicEvidence(evidence []Evidence) []Evidence {
	result := make([]Evidence, 0, len(evidence))
	for _, item := range evidence {
		result = append(result, Evidence{
			ID:             sanitizeText(item.ID, 128),
			SourceKey:      sanitizeText(item.SourceKey, 80),
			SourceLabel:    sanitizeText(item.SourceLabel, 100),
			ModifiedAt:     item.ModifiedAt,
			TimestampBasis: sanitizeText(item.TimestampBasis, 80),
			Excerpt:        sanitizeText(item.Excerpt, maxEvidenceExcerpt),
			Untrusted:      true,
		})
	}
	return result
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func minimizePublicPath(path string) string {
	return sanitizeText(path, maxFieldChars)
}

func nextActionForFlow(flow DailyFlow) string {
	switch flow.State {
	case StateConfirmed:
		return "complete"
	case StateInterviewing:
		if isEmptyInterview(flow.Interview) {
			return "draft_candidate_titles_and_checkpoint_title_review"
		}
		if flow.Interview.NextQuestion != "" {
			return "resume_interview"
		}
		if flow.Interview.Stage == InterviewPreview {
			return "show_final_preview"
		}
		return "reload_status_and_repair_interview_progress"
	default:
		if len(flow.Evidence) > 0 {
			return "ask_remote_evidence_consent"
		}
		return "review_coverage_and_add_work_manually"
	}
}

func errorNextAction(err ServiceError) string {
	if err.Code == "revision_conflict" || err.Code == "flow_busy" || err.Code == "artifact_busy" {
		return "reload_status_and_retry_with_latest_revision"
	}
	if err.Code == "flow_not_found" {
		return "prepare_daily_flow"
	}
	return "fix_request_and_retry"
}
