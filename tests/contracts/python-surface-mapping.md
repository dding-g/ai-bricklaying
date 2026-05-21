# Removed Python Surface Coverage Mapping

This document records the completed replacement coverage for the removed legacy Python package and Python regression tests. The removed Python import-level API is intentionally not preserved; the public contract is the npm launcher plus bundled Go CLI behavior.

| Removed legacy test | Former assertion | Current Go/contract coverage |
| --- | --- | --- |
| `tests/test_cli.py::test_non_interactive_flags_write_only_to_requested_dirs` | Non-interactive CLI writes summary, metadata, skill, config, delivery notes, and OpenCode restart hint only to requested directories. | Covered by `tests/cli-node.test.js::testNonInteractiveFlagsWriteOnlyToRequestedDirs`, `tests/contracts/semantic-contracts.js::testNonInteractiveArtifactContract`, and `internal/cli/cli_test.go::TestRunFileOnlyGeneratesSummaryMetadataConfigAndSkill`. |
| `tests/test_cli.py::test_non_interactive_rejects_missing_slack_webhook_url` | `slack-webhook` mode requires a webhook URL in non-interactive runs. | Covered by `tests/cli-node.test.js::testMissingSlackWebhookFails`, `tests/contracts/semantic-contracts.js::testValidationContracts`, and `internal/cli/cli_test.go::TestValidationErrorsExitTwoWithContractMessages`. |
| `tests/test_cli.py::test_skill_name_must_be_path_safe` | Unsafe skill names are rejected and do not create escaped paths. | Covered by `tests/cli-node.test.js::testSkillNameMustBePathSafe`, `tests/contracts/semantic-contracts.js::testValidationContracts`, and `internal/cli/cli_test.go::TestValidationErrorsExitTwoWithContractMessages`. |
| `tests/test_session_sources.py::test_collect_today_sessions_reads_jsonl` | Session source collection reads today's JSONL records through env-configured roots. | Covered by `internal/sources/sources_test.go::TestDiscoverReadsJSONLFromEnvOverrideDirectory`; launcher-level session ingestion and redaction are covered by `tests/contracts/semantic-contracts.js::testRedactionContract` and `tests/cli-node.test.js::testRedactsSecretsFromSessionSnippets`. |
| `tests/test_summary.py::test_summary_includes_source_and_template` | Summary includes date title, source/session count, and language instruction template. | Covered by `tests/contracts/semantic-contracts.js::testNonInteractiveArtifactContract`, `tests/cli-node.test.js::testNonInteractiveFlagsWriteOnlyToRequestedDirs`, and `internal/cli/cli_test.go::TestRunFileOnlyGeneratesSummaryMetadataConfigAndSkill`. |

## Retained Behavior Categories

- CLI flags and aliases: `--help`, `--version`, `--sources`/`--sessions`, `--output-modes`/`--delivery`, `--gmail-to` alias, and `--config-dir` are covered by Go unit tests, Node launcher tests, and semantic contracts.
- Artifact paths: dated Markdown summary, `ai-bricklaying-summary-skill.json`, optional `ai-bricklaying-slack-payload.json`, generated `SKILL.md`, and private config file are covered by Go unit tests and Node launcher tests.
- Safety: file output always enabled, missing Slack webhook error, unsafe skill name rejection, symlink overwrite refusal, config permission best effort, and no direct network delivery remain covered by Go/Node contracts.
- Redaction: Slack webhook URLs, bearer tokens, token/API/password fields, and absolute home paths are covered by `internal/safeio`, source discovery tests, Node launcher tests, and semantic contracts.
- Slack payload: saved Markdown is source of truth; payload exposes `text`, top-level `blocks`, `messages`, ordered top-level section coverage, split/split-ready behavior, and `verification.source === saved_markdown`.
- Empty sessions: no local records is still a successful run that writes artifacts and embeds the reflection template.
