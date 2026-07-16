// Package worklog owns the versioned daily-worklog state and machine protocol.
package worklog

import "time"

const (
	ProtocolVersion = "1.0"
	SchemaVersion   = "1.0"

	StateDraft        = "draft"
	StateInterviewing = "interviewing"
	StateConfirmed    = "confirmed"

	InterviewTitleReview              = "title_review"
	InterviewReflectionResult         = "reflection_result"
	InterviewReflectionDifficultyFeel = "reflection_difficulty_feeling"
	InterviewReflectionLearningNext   = "reflection_learning_next"
	InterviewPreview                  = "preview"
	InterviewComplete                 = "complete"

	WorkItemCandidate = "candidate"
	WorkItemConfirmed = "confirmed"
	WorkItemExcluded  = "excluded"

	WorkItemOriginSession = "session_inference"
	WorkItemOriginUser    = "user_recall"
	WorkItemOriginBoth    = "session_and_user"
)

type SourceCoverage struct {
	SourceKey      string `json:"source_key"`
	SourceLabel    string `json:"source_label"`
	Status         string `json:"status"`
	CandidateFiles int    `json:"candidate_files"`
	UsedRecords    int    `json:"used_records"`
	Reason         string `json:"reason,omitempty"`
}

type Evidence struct {
	ID             string    `json:"id"`
	SourceKey      string    `json:"source_key"`
	SourceLabel    string    `json:"source_label"`
	ModifiedAt     time.Time `json:"modified_at"`
	TimestampBasis string    `json:"timestamp_basis"`
	Excerpt        string    `json:"excerpt"`
	Untrusted      bool      `json:"untrusted"`
}

type Consent struct {
	RemoteEvidence string     `json:"remote_evidence"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

type WorkItem struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	EvidenceSummary string   `json:"evidence_summary,omitempty"`
	Uncertainty     string   `json:"uncertainty,omitempty"`
	Performed       string   `json:"performed,omitempty"`
	Outcome         string   `json:"outcome,omitempty"`
	Verification    string   `json:"verification,omitempty"`
	Issues          string   `json:"issues,omitempty"`
	EvidenceIDs     []string `json:"evidence_ids,omitempty"`
	Status          string   `json:"status,omitempty"`
	Origin          string   `json:"origin,omitempty"`
}

type Reflection struct {
	MeaningfulResult string `json:"meaningful_result,omitempty"`
	Difficulty       string `json:"difficulty,omitempty"`
	Feeling          string `json:"feeling,omitempty"`
	Learning         string `json:"learning,omitempty"`
	NextAction       string `json:"next_action,omitempty"`
}

type Interview struct {
	Stage              string   `json:"stage,omitempty"`
	CompletedQuestions []string `json:"completed_questions,omitempty"`
	NextQuestion       string   `json:"next_question,omitempty"`
}

type IdempotencyRecord struct {
	Key         string    `json:"key"`
	Command     string    `json:"command"`
	RequestHash string    `json:"request_hash"`
	Revision    int       `json:"revision"`
	CreatedAt   time.Time `json:"created_at"`
}

type DailyFlow struct {
	SchemaVersion       string              `json:"schema_version"`
	FlowID              string              `json:"flow_id"`
	Kind                string              `json:"kind"`
	Date                string              `json:"date"`
	Timezone            string              `json:"timezone"`
	Language            string              `json:"language"`
	State               string              `json:"state"`
	Revision            int                 `json:"revision"`
	OutputDir           string              `json:"output_dir"`
	ConfigDir           string              `json:"config_dir"`
	StatePath           string              `json:"state_path"`
	WorklogMarkdownPath string              `json:"worklog_markdown_path"`
	WorklogJSONPath     string              `json:"worklog_json_path"`
	Consent             Consent             `json:"consent"`
	Coverage            []SourceCoverage    `json:"coverage"`
	Evidence            []Evidence          `json:"evidence,omitempty"`
	WorkItems           []WorkItem          `json:"work_items"`
	NoWorkConfirmed     bool                `json:"no_work_confirmed"`
	Reflection          Reflection          `json:"reflection"`
	Interview           Interview           `json:"interview"`
	Idempotency         []IdempotencyRecord `json:"idempotency,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	ConfirmedAt         *time.Time          `json:"confirmed_at,omitempty"`
}

type MachineError struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	Retryable      bool   `json:"retryable"`
	LatestRevision int    `json:"latest_revision,omitempty"`
}

type Envelope struct {
	ProtocolVersion string           `json:"protocol_version"`
	Command         string           `json:"command"`
	OK              bool             `json:"ok"`
	FlowID          string           `json:"flow_id"`
	Revision        int              `json:"revision"`
	State           string           `json:"state"`
	NextAction      string           `json:"next_action"`
	Flow            *PublicDailyFlow `json:"flow,omitempty"`
	Error           *MachineError    `json:"error,omitempty"`
}

// PublicDailyFlow is the adapter-safe representation of persisted private
// state. It never exposes idempotency records, withholds evidence by default,
// and minimizes paths below the current user's home directory.
type PublicDailyFlow struct {
	SchemaVersion       string           `json:"schema_version"`
	FlowID              string           `json:"flow_id"`
	Kind                string           `json:"kind"`
	Date                string           `json:"date"`
	Timezone            string           `json:"timezone"`
	Language            string           `json:"language"`
	State               string           `json:"state"`
	Revision            int              `json:"revision"`
	OutputDir           string           `json:"output_dir"`
	ConfigDir           string           `json:"config_dir"`
	StatePath           string           `json:"state_path"`
	WorklogMarkdownPath string           `json:"worklog_markdown_path"`
	WorklogJSONPath     string           `json:"worklog_json_path"`
	Consent             Consent          `json:"consent"`
	Coverage            []SourceCoverage `json:"coverage"`
	Evidence            []Evidence       `json:"evidence,omitempty"`
	WorkItems           []WorkItem       `json:"work_items"`
	NoWorkConfirmed     bool             `json:"no_work_confirmed"`
	Reflection          Reflection       `json:"reflection"`
	Interview           Interview        `json:"interview"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
	ConfirmedAt         *time.Time       `json:"confirmed_at,omitempty"`
}

type RequestMeta struct {
	ProtocolVersion string `json:"protocol_version"`
}

type PrepareRequest struct {
	RequestMeta
	Date      string   `json:"date,omitempty"`
	Timezone  string   `json:"timezone,omitempty"`
	Language  string   `json:"language,omitempty"`
	Sources   []string `json:"sources,omitempty"`
	OutputDir string   `json:"output_dir,omitempty"`
	ConfigDir string   `json:"config_dir,omitempty"`
}

type StatusRequest struct {
	RequestMeta
	Date      string `json:"date,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	ConfigDir string `json:"config_dir,omitempty"`
}

type MutationIdentity struct {
	RequestMeta
	FlowID           string `json:"flow_id"`
	Date             string `json:"date"`
	ConfigDir        string `json:"config_dir,omitempty"`
	ExpectedRevision int    `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type DiscloseRequest struct {
	MutationIdentity
	Consent *bool `json:"consent"`
}

type CheckpointRequest struct {
	MutationIdentity
	WorkItems       []WorkItem  `json:"work_items"`
	NoWorkConfirmed *bool       `json:"no_work_confirmed"`
	Reflection      *Reflection `json:"reflection"`
	Interview       *Interview  `json:"interview"`
}

type FinalizeRequest struct {
	MutationIdentity
	UserConfirmed bool `json:"user_confirmed"`
}

type ConfirmedWorklog struct {
	SchemaVersion   string           `json:"schema_version"`
	FlowID          string           `json:"flow_id"`
	Date            string           `json:"date"`
	Timezone        string           `json:"timezone"`
	Language        string           `json:"language"`
	State           string           `json:"state"`
	Revision        int              `json:"revision"`
	Coverage        []SourceCoverage `json:"coverage"`
	WorkItems       []WorkItem       `json:"work_items"`
	NoWorkConfirmed bool             `json:"no_work_confirmed"`
	Reflection      Reflection       `json:"reflection"`
	ConfirmedAt     time.Time        `json:"confirmed_at"`
}
