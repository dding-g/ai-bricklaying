package worklog

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ai-bricklaying/internal/config"
	"ai-bricklaying/internal/safeio"
	"ai-bricklaying/internal/sources"
)

const (
	maxEvidenceExcerpt = 1_200
	maxWorkItems       = 20
	maxTitleChars      = 200
	maxWorkItemNote    = 500
	maxFieldChars      = 4_000
	maxIdempotency     = 100
)

var safeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

var interviewStepIDs = map[string]struct{}{
	InterviewTitleReview:              {},
	InterviewReflectionResult:         {},
	InterviewReflectionDifficultyFeel: {},
	InterviewReflectionLearningNext:   {},
	InterviewPreview:                  {},
	InterviewComplete:                 {},
}

var previewStepIDs = []string{
	InterviewTitleReview,
	InterviewReflectionResult,
	InterviewReflectionDifficultyFeel,
	InterviewReflectionLearningNext,
}

type ServiceError struct {
	Code           string
	Message        string
	Retryable      bool
	LatestRevision int
	Internal       bool
}

func (err ServiceError) Error() string { return err.Message }

type Service struct {
	Store Store
	Now   func() time.Time
}

func NewService() Service {
	return Service{Store: Store{}, Now: time.Now}
}

func (service Service) Prepare(request PrepareRequest) (DailyFlow, error) {
	settings, err := service.resolveSettings(request.ConfigDir, request.OutputDir, request.Language, request.Timezone, request.Date)
	if err != nil {
		return DailyFlow{}, err
	}
	release, err := service.acquire(settings.configDir, settings.date)
	if err != nil {
		return DailyFlow{}, err
	}
	defer release()
	sourceKeys := request.Sources
	if len(sourceKeys) == 0 {
		sourceKeys = []string{settings.source}
	}

	exists, err := service.Store.Exists(settings.configDir, settings.date)
	if err != nil {
		return DailyFlow{}, storageError(err)
	}
	if exists {
		flow, err := service.Store.Load(settings.configDir, settings.date)
		if err != nil {
			return DailyFlow{}, storageError(err)
		}
		return flow, nil
	}

	coverage, evidence, err := collectEvidence(sourceKeys, settings.day)
	if err != nil {
		return DailyFlow{}, err
	}
	flowID, err := newFlowID()
	if err != nil {
		return DailyFlow{}, internalError(err)
	}
	statePath := service.Store.StatePath(settings.configDir, settings.date)
	markdownPath, jsonPath := service.Store.WorklogPaths(settings.outputDir, settings.date)
	now := service.now()
	flow := DailyFlow{
		SchemaVersion:       SchemaVersion,
		FlowID:              flowID,
		Kind:                "daily_worklog",
		Date:                settings.date,
		Timezone:            settings.location.String(),
		Language:            settings.language,
		State:               StateDraft,
		Revision:            1,
		OutputDir:           settings.outputDir,
		ConfigDir:           settings.configDir,
		StatePath:           statePath,
		WorklogMarkdownPath: markdownPath,
		WorklogJSONPath:     jsonPath,
		Consent:             Consent{RemoteEvidence: "pending"},
		Coverage:            coverage,
		Evidence:            evidence,
		WorkItems:           []WorkItem{},
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := service.Store.Save(flow); err != nil {
		return DailyFlow{}, storageError(err)
	}
	return flow, nil
}

func (service Service) Status(request StatusRequest) (DailyFlow, error) {
	settings, err := service.resolveSettings(request.ConfigDir, "", "", request.Timezone, request.Date)
	if err != nil {
		return DailyFlow{}, err
	}
	flow, err := service.Store.Load(settings.configDir, settings.date)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DailyFlow{}, validationError("flow_not_found", "no daily worklog flow exists for the requested date")
		}
		return DailyFlow{}, storageError(err)
	}
	return flow, nil
}

func (service Service) Disclose(request DiscloseRequest) (DailyFlow, bool, error) {
	if err := requireMutationDate(request.Date); err != nil {
		return DailyFlow{}, false, err
	}
	if request.Consent == nil {
		return DailyFlow{}, false, validationError("consent_decision_required", "disclose requires an explicit consent true or false decision")
	}
	release, err := service.acquireForMutation(request.MutationIdentity)
	if err != nil {
		return DailyFlow{}, false, err
	}
	defer release()
	requestHash := hashRequest(request)
	flow, duplicate, err := service.loadForMutation(request.MutationIdentity, "daily.disclose", requestHash)
	if err != nil {
		return DailyFlow{}, false, err
	}
	if duplicate {
		return flow, *request.Consent && flow.Consent.RemoteEvidence == "granted", nil
	}
	if flow.State == StateConfirmed {
		return DailyFlow{}, false, validationError("flow_confirmed", "a confirmed flow cannot disclose evidence or return to interviewing")
	}
	if err := ensureMutationCapacity(flow, "daily.disclose"); err != nil {
		return DailyFlow{}, false, err
	}

	now := service.now()
	decision := "denied"
	if *request.Consent {
		decision = "granted"
	}
	flow.Consent = Consent{RemoteEvidence: decision, UpdatedAt: &now}
	flow.State = StateInterviewing
	flow.Revision++
	flow.UpdatedAt = now
	flow.addIdempotency(request.IdempotencyKey, "daily.disclose", requestHash, now)
	if err := service.Store.Save(flow); err != nil {
		return DailyFlow{}, false, storageError(err)
	}
	return flow, *request.Consent, nil
}

func (service Service) Checkpoint(request CheckpointRequest) (DailyFlow, error) {
	if err := requireMutationDate(request.Date); err != nil {
		return DailyFlow{}, err
	}
	release, err := service.acquireForMutation(request.MutationIdentity)
	if err != nil {
		return DailyFlow{}, err
	}
	defer release()
	requestHash := hashRequest(request)
	flow, duplicate, err := service.loadForMutation(request.MutationIdentity, "daily.checkpoint", requestHash)
	if err != nil {
		return DailyFlow{}, err
	}
	if duplicate {
		return flow, nil
	}
	if flow.State == StateConfirmed {
		return DailyFlow{}, validationError("flow_confirmed", "a confirmed flow cannot be changed")
	}
	if err := requireEvidenceConsent(flow); err != nil {
		return DailyFlow{}, err
	}
	if err := ensureMutationCapacity(flow, "daily.checkpoint"); err != nil {
		return DailyFlow{}, err
	}
	if request.WorkItems == nil {
		return DailyFlow{}, validationError("work_items_required", "checkpoint requires an explicit work_items array")
	}
	if request.Reflection == nil {
		return DailyFlow{}, validationError("reflection_required", "checkpoint requires an explicit reflection object")
	}
	if request.Interview == nil {
		return DailyFlow{}, validationError("interview_required", "checkpoint requires an explicit interview object")
	}
	if request.NoWorkConfirmed == nil {
		return DailyFlow{}, validationError("no_work_decision_required", "checkpoint requires an explicit no_work_confirmed true or false decision")
	}
	workItems, err := sanitizeWorkItems(request.WorkItems, flow.Evidence)
	if err != nil {
		return DailyFlow{}, err
	}
	reflection := sanitizeReflection(*request.Reflection)
	interview, err := sanitizeInterview(*request.Interview)
	if err != nil {
		return DailyFlow{}, err
	}
	if err := validateCheckpointInterview(interview); err != nil {
		return DailyFlow{}, err
	}
	if err := validateInterviewTransition(flow.Interview, interview); err != nil {
		return DailyFlow{}, err
	}
	if interview.Stage != InterviewTitleReview {
		if err := validateFinalWorkItems(workItems, *request.NoWorkConfirmed); err != nil {
			return DailyFlow{}, err
		}
	}
	now := service.now()
	flow.WorkItems = workItems
	flow.NoWorkConfirmed = *request.NoWorkConfirmed
	flow.Reflection = reflection
	flow.Interview = interview
	flow.State = StateInterviewing
	flow.Revision++
	flow.UpdatedAt = now
	flow.addIdempotency(request.IdempotencyKey, "daily.checkpoint", requestHash, now)
	if err := service.Store.Save(flow); err != nil {
		return DailyFlow{}, storageError(err)
	}
	return flow, nil
}

func (service Service) Finalize(request FinalizeRequest) (DailyFlow, error) {
	if err := requireMutationDate(request.Date); err != nil {
		return DailyFlow{}, err
	}
	release, err := service.acquireForMutation(request.MutationIdentity)
	if err != nil {
		return DailyFlow{}, err
	}
	defer release()
	requestHash := hashRequest(request)
	flow, duplicate, err := service.loadForMutation(request.MutationIdentity, "daily.finalize", requestHash)
	if err != nil {
		return DailyFlow{}, err
	}
	if duplicate {
		return flow, nil
	}
	if !request.UserConfirmed {
		return DailyFlow{}, validationError("confirmation_required", "finalize requires user_confirmed=true after showing the final preview")
	}
	if flow.State == StateConfirmed {
		return DailyFlow{}, validationError("flow_confirmed", "the daily worklog is already confirmed")
	}
	if err := requireEvidenceConsent(flow); err != nil {
		return DailyFlow{}, err
	}
	if flow.State != StateInterviewing {
		return DailyFlow{}, validationError("interview_required", "finalize requires an interviewing flow with a saved user review")
	}
	if flow.Interview.Stage != InterviewPreview || !hasCompletedPreview(flow.Interview) {
		return DailyFlow{}, validationError("preview_required", "finalize requires a saved preview checkpoint after title and reflection review")
	}
	if _, err := sanitizeWorkItems(flow.WorkItems, flow.Evidence); err != nil {
		return DailyFlow{}, err
	}
	if err := validateFinalWorkItems(flow.WorkItems, flow.NoWorkConfirmed); err != nil {
		return DailyFlow{}, err
	}
	if err := ensureMutationCapacity(flow, "daily.finalize"); err != nil {
		return DailyFlow{}, err
	}
	releaseArtifacts, err := service.acquireArtifacts(flow.OutputDir, flow.Date)
	if err != nil {
		return DailyFlow{}, err
	}
	defer releaseArtifacts()

	now := service.now()
	flow.Language = NormalizeLanguage(flow.Language)
	flow.State = StateConfirmed
	flow.Revision++
	flow.UpdatedAt = now
	flow.ConfirmedAt = &now
	flow.Interview.Stage = InterviewComplete
	flow.Interview.NextQuestion = ""
	flow.addIdempotency(request.IdempotencyKey, "daily.finalize", requestHash, now)
	confirmed := ConfirmedWorklog{
		SchemaVersion:   SchemaVersion,
		FlowID:          flow.FlowID,
		Date:            flow.Date,
		Timezone:        flow.Timezone,
		Language:        flow.Language,
		State:           flow.State,
		Revision:        flow.Revision,
		Coverage:        append([]SourceCoverage(nil), flow.Coverage...),
		WorkItems:       confirmedWorkItems(flow.WorkItems),
		NoWorkConfirmed: flow.NoWorkConfirmed,
		Reflection:      flow.Reflection,
		ConfirmedAt:     now,
	}
	flow, err = service.Store.CommitConfirmedFlow(flow, confirmed, RenderMarkdown(confirmed))
	if err != nil {
		return DailyFlow{}, storageError(err)
	}
	return flow, nil
}

type resolvedSettings struct {
	configDir string
	outputDir string
	language  string
	source    string
	date      string
	day       time.Time
	location  *time.Location
}

func absoluteRoot(value string) (string, error) {
	expanded := config.ExpandHome(strings.TrimSpace(value))
	absolute, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func requireMutationDate(date string) error {
	if strings.TrimSpace(date) == "" {
		return validationError("date_required", "mutation requests require an explicit date in YYYY-MM-DD format")
	}
	return nil
}

func (service Service) resolveSettings(configDir, outputDir, language, timezone, date string) (resolvedSettings, error) {
	if strings.TrimSpace(configDir) == "" {
		configDir = "~/.config/ai-bricklaying"
	}
	configDir, err := absoluteRoot(configDir)
	if err != nil {
		return resolvedSettings{}, validationError("invalid_config_dir", "config_dir must resolve to a valid local path")
	}
	if err := safeio.RejectSymlinkAncestors(filepath.Join(configDir, "config.json")); err != nil {
		return resolvedSettings{}, storageError(err)
	}
	stored, _, err := config.Load(configDir)
	if err != nil {
		return resolvedSettings{}, validationError("config_read_failed", safeio.RedactString(err.Error()))
	}
	if strings.TrimSpace(outputDir) == "" {
		outputDir = stored.Defaults.OutputDir
	}
	if strings.TrimSpace(outputDir) == "" {
		outputDir = "~/ai-bricklaying"
	}
	outputDir, err = absoluteRoot(outputDir)
	if err != nil {
		return resolvedSettings{}, validationError("invalid_output_dir", "output_dir must resolve to a valid local path")
	}
	if err := safeio.RejectSymlinkAncestors(outputDir); err != nil {
		return resolvedSettings{}, storageError(err)
	}
	if strings.TrimSpace(language) == "" {
		language = stored.Defaults.Language
	}
	if strings.TrimSpace(language) == "" {
		language = "English"
	}
	defaultSource := strings.TrimSpace(stored.Defaults.Source)
	if defaultSource == "" {
		defaultSource = "claude-code"
	}

	location := service.now().Location()
	if strings.TrimSpace(timezone) != "" {
		loaded, err := time.LoadLocation(timezone)
		if err != nil {
			return resolvedSettings{}, validationError("invalid_timezone", "timezone must be a valid IANA timezone name")
		}
		location = loaded
	}
	now := service.now().In(location)
	if strings.TrimSpace(date) == "" {
		date = now.Format("2006-01-02")
	}
	day, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return resolvedSettings{}, validationError("invalid_date", "date must use YYYY-MM-DD")
	}
	return resolvedSettings{
		configDir: configDir,
		outputDir: outputDir,
		language:  NormalizeLanguage(language),
		source:    defaultSource,
		date:      day.Format("2006-01-02"),
		day:       day,
		location:  location,
	}, nil
}

func (service Service) loadForMutation(identity MutationIdentity, command, requestHash string) (DailyFlow, bool, error) {
	if err := requireMutationDate(identity.Date); err != nil {
		return DailyFlow{}, false, err
	}
	if !safeIDPattern.MatchString(identity.FlowID) {
		return DailyFlow{}, false, validationError("invalid_flow_id", "flow_id is required and must be path-safe")
	}
	if !safeIDPattern.MatchString(identity.IdempotencyKey) {
		return DailyFlow{}, false, validationError("invalid_idempotency_key", "idempotency_key is required and must be path-safe")
	}
	if identity.ExpectedRevision < 1 {
		return DailyFlow{}, false, validationError("expected_revision_required", "expected_revision is required and must be at least 1")
	}
	settings, err := service.resolveSettings(identity.ConfigDir, "", "", "", identity.Date)
	if err != nil {
		return DailyFlow{}, false, err
	}
	flow, err := service.Store.Load(settings.configDir, settings.date)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DailyFlow{}, false, validationError("flow_not_found", "no daily worklog flow exists for the requested date")
		}
		return DailyFlow{}, false, storageError(err)
	}
	if flow.FlowID != identity.FlowID {
		return DailyFlow{}, false, validationError("flow_mismatch", "flow_id does not match the requested date")
	}
	if previous, ok := flow.idempotency(identity.IdempotencyKey); ok {
		if previous.Command != command || previous.RequestHash != requestHash {
			return DailyFlow{}, false, validationError("idempotency_key_reused", "idempotency_key was already used for a different request")
		}
		return flow, true, nil
	}
	if identity.ExpectedRevision != flow.Revision {
		return DailyFlow{}, false, ServiceError{
			Code:           "revision_conflict",
			Message:        "expected_revision does not match the latest daily flow revision",
			Retryable:      true,
			LatestRevision: flow.Revision,
		}
	}
	return flow, false, nil
}

func (service Service) acquireForMutation(identity MutationIdentity) (func(), error) {
	if err := requireMutationDate(identity.Date); err != nil {
		return nil, err
	}
	settings, err := service.resolveSettings(identity.ConfigDir, "", "", "", identity.Date)
	if err != nil {
		return nil, err
	}
	return service.acquire(settings.configDir, settings.date)
}

func (service Service) acquire(configDir, date string) (func(), error) {
	release, err := service.Store.Acquire(configDir, date)
	if err == nil {
		return release, nil
	}
	if errors.Is(err, ErrFlowBusy) {
		return nil, ServiceError{Code: "flow_busy", Message: "daily flow is being updated by another process", Retryable: true}
	}
	return nil, storageError(err)
}

func (service Service) acquireArtifacts(outputDir, date string) (func(), error) {
	release, err := service.Store.AcquireArtifacts(outputDir, date)
	if err == nil {
		return release, nil
	}
	if errors.Is(err, ErrFlowBusy) {
		return nil, ServiceError{Code: "artifact_busy", Message: "daily worklog artifacts are being updated by another process", Retryable: true}
	}
	return nil, storageError(err)
}

func collectEvidence(requested []string, day time.Time) ([]SourceCoverage, []Evidence, error) {
	if len(requested) == 0 {
		return nil, nil, validationError("source_required", "prepare requires at least one configured source")
	}
	selected := make([]sources.Source, 0, len(requested))
	seen := map[string]bool{}
	for _, key := range requested {
		key = strings.TrimSpace(key)
		if seen[key] {
			continue
		}
		source, ok := sources.Find(key)
		if !ok {
			return nil, nil, validationError("unknown_source", fmt.Sprintf("unknown source %q", sanitizeText(key, 80)))
		}
		seen[key] = true
		selected = append(selected, source)
	}
	coverage := make([]SourceCoverage, 0, len(selected))
	var evidence []Evidence
	for _, source := range selected {
		report := sources.DiscoverWorklogReport(source, sources.DiscoverOptions{Today: day, Limit: 6, MaxChars: 4_000})
		coverageItem := SourceCoverage{
			SourceKey:      report.Coverage.SourceKey,
			SourceLabel:    report.Coverage.SourceLabel,
			Status:         report.Coverage.Status,
			CandidateFiles: report.Coverage.CandidateFiles,
			UsedRecords:    report.Coverage.UsedRecords,
			Reason:         report.Coverage.Reason,
		}
		serviceTruncated := false
		for _, record := range report.Records {
			excerpt, truncated := sanitizeTextWithTruncation(record.Text, maxEvidenceExcerpt)
			serviceTruncated = serviceTruncated || truncated
			evidence = append(evidence, Evidence{
				ID:             fmt.Sprintf("e%d", len(evidence)+1),
				SourceKey:      record.SourceKey,
				SourceLabel:    record.SourceLabel,
				ModifiedAt:     record.ModifiedAt,
				TimestampBasis: record.TimestampBasis,
				Excerpt:        excerpt,
				Untrusted:      true,
			})
		}
		if serviceTruncated && coverageItem.Status == "complete" {
			coverageItem.Status = "truncated"
			coverageItem.Reason = "worklog evidence exceeded the private excerpt limit"
		}
		coverage = append(coverage, coverageItem)
	}
	return coverage, evidence, nil
}

func sanitizeWorkItems(items []WorkItem, evidence []Evidence) ([]WorkItem, error) {
	if len(items) > maxWorkItems {
		return nil, validationError("too_many_work_items", fmt.Sprintf("work_items is limited to %d entries", maxWorkItems))
	}
	evidenceIDs := map[string]bool{}
	for _, item := range evidence {
		evidenceIDs[item.ID] = true
	}
	seen := map[string]bool{}
	result := make([]WorkItem, 0, len(items))
	for _, item := range items {
		if !safeIDPattern.MatchString(item.ID) || seen[item.ID] {
			return nil, validationError("invalid_work_item_id", "each work item needs a unique path-safe id")
		}
		seen[item.ID] = true
		item.Title = sanitizeText(item.Title, maxTitleChars)
		if item.Title == "" {
			return nil, validationError("missing_work_item_title", "each work item needs a non-empty title")
		}
		item.EvidenceSummary = sanitizeOneLine(item.EvidenceSummary, maxWorkItemNote)
		item.Uncertainty = sanitizeOneLine(item.Uncertainty, maxWorkItemNote)
		item.Performed = sanitizeText(item.Performed, maxFieldChars)
		item.Outcome = sanitizeText(item.Outcome, maxFieldChars)
		item.Verification = sanitizeText(item.Verification, maxFieldChars)
		item.Issues = sanitizeText(item.Issues, maxFieldChars)
		item.Status = strings.ToLower(sanitizeText(item.Status, 40))
		if item.Status == "" {
			item.Status = WorkItemCandidate
		}
		switch item.Status {
		case WorkItemCandidate, WorkItemConfirmed, WorkItemExcluded:
		default:
			return nil, validationError("invalid_work_item_status", "work item status must be candidate, confirmed, or excluded")
		}
		item.Origin = strings.ToLower(sanitizeText(item.Origin, 40))
		switch item.Origin {
		case "", WorkItemOriginSession, WorkItemOriginUser, WorkItemOriginBoth:
		default:
			return nil, validationError("invalid_work_item_origin", "work item origin must be session_inference, user_recall, or session_and_user")
		}
		for _, evidenceID := range item.EvidenceIDs {
			if !evidenceIDs[evidenceID] {
				return nil, validationError("unknown_evidence_id", fmt.Sprintf("work item %s references an unknown evidence id", item.ID))
			}
		}
		item.EvidenceIDs = deduplicateStrings(item.EvidenceIDs)
		result = append(result, item)
	}
	return result, nil
}

func validateFinalWorkItems(items []WorkItem, noWorkConfirmed bool) error {
	confirmedCount := 0
	for _, item := range items {
		if item.Status == WorkItemCandidate {
			return validationError("candidate_work_item", "preview must confirm or exclude every work item")
		}
		if item.Status == WorkItemConfirmed {
			confirmedCount++
		}
	}
	if confirmedCount == 0 && !noWorkConfirmed {
		return validationError("no_work_decision_required", "an empty preview requires no_work_confirmed=true")
	}
	if confirmedCount > 0 && noWorkConfirmed {
		return validationError("invalid_no_work_decision", "no_work_confirmed cannot be true when confirmed work items exist")
	}
	return nil
}

func confirmedWorkItems(items []WorkItem) []WorkItem {
	result := make([]WorkItem, 0, len(items))
	for _, item := range items {
		if item.Status != WorkItemConfirmed {
			continue
		}
		item.EvidenceIDs = append([]string(nil), item.EvidenceIDs...)
		result = append(result, item)
	}
	return result
}

func sanitizeReflection(value Reflection) Reflection {
	return Reflection{
		MeaningfulResult: sanitizeText(value.MeaningfulResult, maxFieldChars),
		Difficulty:       sanitizeText(value.Difficulty, maxFieldChars),
		Feeling:          sanitizeText(value.Feeling, maxFieldChars),
		Learning:         sanitizeText(value.Learning, maxFieldChars),
		NextAction:       sanitizeText(value.NextAction, maxFieldChars),
	}
}

func sanitizeInterview(value Interview) (Interview, error) {
	if len(value.CompletedQuestions) > 20 {
		return Interview{}, validationError("too_many_interview_steps", "completed_questions is limited to 20 entries")
	}
	stage, err := sanitizeInterviewStep(value.Stage, true, "stage")
	if err != nil {
		return Interview{}, err
	}
	nextQuestion, err := sanitizeInterviewStep(value.NextQuestion, true, "next_question")
	if err != nil {
		return Interview{}, err
	}
	completed := make([]string, 0, len(value.CompletedQuestions))
	seen := map[string]bool{}
	for _, raw := range value.CompletedQuestions {
		step, err := sanitizeInterviewStep(raw, false, "completed_questions")
		if err != nil {
			return Interview{}, err
		}
		if !seen[step] {
			seen[step] = true
			completed = append(completed, step)
		}
	}
	return Interview{
		Stage:              stage,
		CompletedQuestions: completed,
		NextQuestion:       nextQuestion,
	}, nil
}

func sanitizeInterviewStep(value string, allowEmpty bool, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" && allowEmpty {
		return "", nil
	}
	if _, ok := interviewStepIDs[value]; !ok {
		return "", validationError("invalid_interview_step", fmt.Sprintf("%s must use a documented interview step id", field))
	}
	return value, nil
}

func validateCheckpointInterview(interview Interview) error {
	if interview.Stage == InterviewComplete {
		return validationError("invalid_interview_step", "complete is reserved for a successful finalize operation")
	}
	if _, ok := interviewProgressIndex(interview); !ok {
		return validationError("invalid_interview_progress", "interview stage, completed_questions, and next_question must match the documented daily progression")
	}
	return nil
}

func validateInterviewTransition(previous, next Interview) error {
	nextIndex, ok := interviewProgressIndex(next)
	if !ok {
		return validationError("invalid_interview_progress", "interview stage, completed_questions, and next_question must match the documented daily progression")
	}
	if isEmptyInterview(previous) {
		if nextIndex != 0 {
			return validationError("invalid_interview_transition", "the first checkpoint must start at title_review")
		}
		return nil
	}
	previousIndex, ok := interviewProgressIndex(previous)
	if !ok {
		return validationError("invalid_interview_transition", "saved interview progress is not a valid checkpoint snapshot")
	}
	delta := nextIndex - previousIndex
	if delta < -1 || delta > 1 {
		return validationError("invalid_interview_transition", "checkpoint may stay at the current step, advance one step, or restore only the immediately previous valid snapshot")
	}
	return nil
}

func interviewProgressIndex(interview Interview) (int, bool) {
	progression := []struct {
		stage     string
		completed []string
		next      string
	}{
		{stage: InterviewTitleReview, completed: []string{}, next: InterviewTitleReview},
		{stage: InterviewReflectionResult, completed: []string{InterviewTitleReview}, next: InterviewReflectionResult},
		{stage: InterviewReflectionDifficultyFeel, completed: []string{InterviewTitleReview, InterviewReflectionResult}, next: InterviewReflectionDifficultyFeel},
		{stage: InterviewReflectionLearningNext, completed: []string{InterviewTitleReview, InterviewReflectionResult, InterviewReflectionDifficultyFeel}, next: InterviewReflectionLearningNext},
		{stage: InterviewPreview, completed: previewStepIDs, next: ""},
	}
	for index, expected := range progression {
		if interview.Stage == expected.stage && interview.NextQuestion == expected.next && equalStrings(interview.CompletedQuestions, expected.completed) {
			return index, true
		}
	}
	return -1, false
}

func isEmptyInterview(interview Interview) bool {
	return interview.Stage == "" && interview.NextQuestion == "" && len(interview.CompletedQuestions) == 0
}

func hasCompletedPreview(interview Interview) bool {
	completed := map[string]bool{}
	for _, step := range interview.CompletedQuestions {
		completed[step] = true
	}
	for _, required := range previewStepIDs {
		if !completed[required] {
			return false
		}
	}
	return true
}

func sanitizeText(value string, maxChars int) string {
	value, _ = sanitizeTextWithTruncation(value, maxChars)
	return value
}

func sanitizeTextWithTruncation(value string, maxChars int) (string, bool) {
	value = safeio.RedactString(value)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		value = strings.ReplaceAll(value, home, "~")
	}
	value = safeio.SanitizeControl(value)
	value = strings.TrimSpace(value)
	if maxChars <= 0 {
		return "", value != ""
	}
	if utf8.RuneCountInString(value) <= maxChars {
		return value, false
	}
	runes := []rune(value)
	if maxChars == 1 {
		return "…", true
	}
	return strings.TrimSpace(string(runes[:maxChars-1])) + "…", true
}

func sanitizeOneLine(value string, maxChars int) string {
	value = sanitizeText(value, maxFieldChars)
	return sanitizeText(strings.Join(strings.Fields(value), " "), maxChars)
}

func deduplicateStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = sanitizeText(value, 128)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (flow *DailyFlow) addIdempotency(key, command, requestHash string, now time.Time) {
	flow.Idempotency = append(flow.Idempotency, IdempotencyRecord{Key: key, Command: command, RequestHash: requestHash, Revision: flow.Revision, CreatedAt: now})
}

func ensureMutationCapacity(flow DailyFlow, command string) error {
	if len(flow.Idempotency) >= maxIdempotency {
		return validationError("mutation_limit_reached", fmt.Sprintf("%s cannot add more than %d durable mutation records to one daily flow", command, maxIdempotency))
	}
	return nil
}

func requireEvidenceConsent(flow DailyFlow) error {
	if len(flow.Evidence) > 0 && flow.Consent.RemoteEvidence == "pending" {
		return validationError("consent_decision_required", "evidence-backed interview progress requires an explicit disclose consent true or false decision")
	}
	return nil
}

func hashRequest(value any) string {
	contents, _ := json.Marshal(value)
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func (flow DailyFlow) idempotency(key string) (IdempotencyRecord, bool) {
	for _, record := range flow.Idempotency {
		if record.Key == key {
			return record, true
		}
	}
	return IdempotencyRecord{}, false
}

func newFlowID() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "daily-" + hex.EncodeToString(raw), nil
}

func (service Service) now() time.Time {
	if service.Now == nil {
		return time.Now()
	}
	return service.Now()
}

func validationError(code, message string) error {
	return ServiceError{Code: code, Message: message}
}

func internalError(err error) error {
	var serviceErr ServiceError
	if errors.As(err, &serviceErr) {
		return err
	}
	return ServiceError{Code: "internal_error", Message: safeio.RedactString(err.Error()), Internal: true}
}

func storageError(err error) error {
	if errors.Is(err, safeio.ErrSymlinkTarget) {
		return validationError("unsafe_path", "configured worklog paths must not contain symbolic links")
	}
	if errors.Is(err, ErrInvalidFlowState) {
		return validationError("invalid_flow_state", "saved daily flow state failed integrity validation")
	}
	if errors.Is(err, ErrArtifactConflict) {
		return validationError("artifact_conflict", "confirmed worklog artifacts already exist for this output directory and date")
	}
	return internalError(err)
}
