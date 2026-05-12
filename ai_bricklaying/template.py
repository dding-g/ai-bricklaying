DEFAULT_SUMMARY_TEMPLATE = """# Daily AI Bricklaying Summary

Write the summary in {language}.

## Executive Brief
Summarize the day in 3-5 sentences. Focus on the user's actual progress, decisions, and remaining leverage points.

## Work Completed
List the meaningful work streams. For each item, include the agent source, the concrete outcome, and the artifact or decision produced.

## Lessons Learned
Capture reusable lessons: implementation patterns, failed assumptions, debugging insights, workflow improvements, and tool limitations.

## Results And Evidence
Describe what changed, what was verified, what remains unverified, and where the evidence came from. Prefer specific files, commands, issues, commits, or session references when available.

## Compound Engineering Notes
Explain how today's work can compound: reusable prompts, skills, docs, tests, automation, architectural patterns, or guardrails that reduce future effort.

## Improvement Backlog
Prioritize the next 3-7 improvements. Each item should include why it matters, the smallest useful next step, and which agent or workflow should handle it.

## Follow-Up Prompt
Write a concise prompt the user can paste into an AI coding agent tomorrow to continue from today's context.
"""


def render_template(language: str) -> str:
    return DEFAULT_SUMMARY_TEMPLATE.format(language=language)
