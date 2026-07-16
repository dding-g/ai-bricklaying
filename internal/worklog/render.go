package worklog

import (
	"fmt"
	"strings"
)

// RenderMarkdown deterministically renders a confirmed worklog without raw evidence.
func RenderMarkdown(worklog ConfirmedWorklog) string {
	if NormalizeLanguage(worklog.Language) == "Korean" {
		return renderKorean(worklog)
	}
	return renderEnglish(worklog)
}

// NormalizeLanguage keeps persisted worklog metadata aligned with the two
// deterministic renderers supported by the version 1.0 contract.
func NormalizeLanguage(language string) string {
	if isKorean(language) {
		return "Korean"
	}
	return "English"
}

func renderKorean(worklog ConfirmedWorklog) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "---\nschema_version: %q\nflow_id: %q\ndate: %q\ntimezone: %q\nstate: confirmed\nrevision: %d\n---\n\n", worklog.SchemaVersion, worklog.FlowID, worklog.Date, worklog.Timezone, worklog.Revision)
	fmt.Fprintf(&builder, "# 업무일지 - %s\n\n", worklog.Date)
	builder.WriteString("## 수집 범위\n\n")
	renderCoverage(&builder, worklog.Coverage, true)
	builder.WriteString("\n## 확인된 업무\n\n")
	renderWorkItems(&builder, worklog.WorkItems, true)
	builder.WriteString("\n## 오늘의 회고\n\n")
	writeField(&builder, "가장 의미 있었던 결과", worklog.Reflection.MeaningfulResult)
	writeField(&builder, "어려웠던 점", worklog.Reflection.Difficulty)
	writeField(&builder, "느낀 점", worklog.Reflection.Feeling)
	writeField(&builder, "배운 점", worklog.Reflection.Learning)
	builder.WriteString("\n## 다음 행동\n\n")
	writeValueOrPlaceholder(&builder, worklog.Reflection.NextAction, "정하지 않음")
	builder.WriteString("\n## 불확실성\n\n")
	renderUncertainty(&builder, worklog.Coverage, true)
	return builder.String()
}

func renderEnglish(worklog ConfirmedWorklog) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "---\nschema_version: %q\nflow_id: %q\ndate: %q\ntimezone: %q\nstate: confirmed\nrevision: %d\n---\n\n", worklog.SchemaVersion, worklog.FlowID, worklog.Date, worklog.Timezone, worklog.Revision)
	fmt.Fprintf(&builder, "# Worklog - %s\n\n", worklog.Date)
	builder.WriteString("## Collection Coverage\n\n")
	renderCoverage(&builder, worklog.Coverage, false)
	builder.WriteString("\n## Confirmed Work\n\n")
	renderWorkItems(&builder, worklog.WorkItems, false)
	builder.WriteString("\n## Daily Reflection\n\n")
	writeField(&builder, "Most meaningful result", worklog.Reflection.MeaningfulResult)
	writeField(&builder, "Difficulty", worklog.Reflection.Difficulty)
	writeField(&builder, "How the work felt", worklog.Reflection.Feeling)
	writeField(&builder, "Learning", worklog.Reflection.Learning)
	builder.WriteString("\n## Next Action\n\n")
	writeValueOrPlaceholder(&builder, worklog.Reflection.NextAction, "Not decided")
	builder.WriteString("\n## Uncertainty\n\n")
	renderUncertainty(&builder, worklog.Coverage, false)
	return builder.String()
}

func renderCoverage(builder *strings.Builder, coverage []SourceCoverage, korean bool) {
	if len(coverage) == 0 {
		if korean {
			builder.WriteString("- 기록된 소스 수집 범위 없음.\n")
		} else {
			builder.WriteString("- No source coverage was recorded.\n")
		}
		return
	}
	for _, item := range coverage {
		if korean {
			fmt.Fprintf(builder, "- %s: `%s` (후보 파일 %d개 중 %d개 사용)\n", singleLine(item.SourceLabel), item.Status, item.CandidateFiles, item.UsedRecords)
		} else {
			fmt.Fprintf(builder, "- %s: `%s` (used %d of %d candidate files)\n", singleLine(item.SourceLabel), item.Status, item.UsedRecords, item.CandidateFiles)
		}
	}
}

func renderWorkItems(builder *strings.Builder, items []WorkItem, korean bool) {
	written := 0
	for _, item := range items {
		if strings.EqualFold(item.Status, "excluded") {
			continue
		}
		written++
		fmt.Fprintf(builder, "### %s\n\n", singleLine(item.Title))
		if korean {
			writeField(builder, "한 줄 근거", item.EvidenceSummary)
			writeUncertainty(builder, "불확실성", item.Uncertainty, "없음")
			writeField(builder, "수행 내용", item.Performed)
			writeField(builder, "결과", item.Outcome)
			writeField(builder, "검증", item.Verification)
			writeField(builder, "남은 이슈", item.Issues)
			if len(item.EvidenceIDs) > 0 {
				fmt.Fprintf(builder, "- 근거 ID: %s\n", strings.Join(item.EvidenceIDs, ", "))
			}
		} else {
			writeField(builder, "Evidence summary", item.EvidenceSummary)
			writeUncertainty(builder, "Uncertainty", item.Uncertainty, "None known")
			writeField(builder, "Work performed", item.Performed)
			writeField(builder, "Outcome", item.Outcome)
			writeField(builder, "Verification", item.Verification)
			writeField(builder, "Remaining issues", item.Issues)
			if len(item.EvidenceIDs) > 0 {
				fmt.Fprintf(builder, "- Evidence IDs: %s\n", strings.Join(item.EvidenceIDs, ", "))
			}
		}
		builder.WriteString("\n")
	}
	if written == 0 {
		if korean {
			builder.WriteString("확인된 업무 없음.\n")
		} else {
			builder.WriteString("No confirmed work items.\n")
		}
	}
}

func writeUncertainty(builder *strings.Builder, label, value, none string) {
	fmt.Fprintf(builder, "- %s: ", label)
	writeValueOrPlaceholder(builder, value, none)
}

func renderUncertainty(builder *strings.Builder, coverage []SourceCoverage, korean bool) {
	written := false
	for _, item := range coverage {
		if item.Status == "complete" {
			continue
		}
		written = true
		if item.Reason != "" {
			fmt.Fprintf(builder, "- %s: %s (`%s`)\n", singleLine(item.SourceLabel), markdownValue(localizedCoverageReason(item.Reason, korean)), item.Status)
		} else {
			fmt.Fprintf(builder, "- %s: `%s`\n", singleLine(item.SourceLabel), item.Status)
		}
	}
	if !written {
		if korean {
			builder.WriteString("- 알려진 수집 불확실성 없음.\n")
		} else {
			builder.WriteString("- No known collection uncertainty.\n")
		}
	}
}

func localizedCoverageReason(reason string, korean bool) string {
	if !korean {
		return reason
	}
	switch reason {
	case "configured source root could not be read":
		return "설정한 소스 루트를 읽을 수 없음"
	case "configured source root was not found":
		return "설정한 소스 루트를 찾을 수 없음"
	case "one or more JSON session artifacts could not be parsed":
		return "하나 이상의 JSON 세션 기록을 해석할 수 없음"
	case "one or more configured source paths could not be read":
		return "하나 이상의 설정된 소스 경로를 읽을 수 없음"
	case "source exceeded the per-run record or excerpt limit":
		return "소스가 실행당 기록 또는 발췌 한도를 초과함"
	case "source exceeded the per-run file or excerpt limit":
		return "소스가 실행당 파일 또는 발췌 한도를 초과함"
	case "worklog evidence exceeded the private excerpt limit":
		return "업무일지 근거가 비공개 발췌 한도를 초과함"
	case "no readable activity was found for the requested date":
		return "요청한 날짜에 읽을 수 있는 활동을 찾지 못함"
	default:
		return "수집 상태 코드에 세부 사유가 기록됨"
	}
}

func writeField(builder *strings.Builder, label, value string) {
	placeholder := "기록하지 않음"
	if !containsKorean(label) {
		placeholder = "Not recorded"
	}
	fmt.Fprintf(builder, "- %s: ", label)
	writeValueOrPlaceholder(builder, value, placeholder)
}

func writeValueOrPlaceholder(builder *strings.Builder, value, placeholder string) {
	if strings.TrimSpace(value) == "" {
		builder.WriteString(placeholder + "\n")
		return
	}
	builder.WriteString(markdownValue(value) + "\n")
}

func markdownValue(value string) string {
	value = strings.TrimSpace(value)
	return strings.ReplaceAll(value, "\n", "\n  ")
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func isKorean(language string) bool {
	normalized := strings.ToLower(strings.TrimSpace(language))
	return normalized == "ko" || strings.HasPrefix(normalized, "ko-") || strings.Contains(normalized, "korean") || strings.Contains(language, "한국")
}

func containsKorean(value string) bool {
	for _, char := range value {
		if char >= '\uac00' && char <= '\ud7a3' {
			return true
		}
	}
	return false
}
