# Skill Registry

This registry lists all available non-internal skills detected for GolemUI.

| Skill Name | Scope | Trigger / Description | Path |
|---|---|---|---|
| branch-pr | user | Create Gentle AI pull requests with issue-first checks. Trigger: creating, opening, or preparing PRs for review. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/branch-pr/SKILL.md) |
| chained-pr | user | Trigger: PRs over 400 lines, stacked PRs, review slices. Split oversized changes into chained PRs that protect review focus. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/chained-pr/SKILL.md) |
| cognitive-doc-design | user | Design docs that reduce cognitive load. Trigger: writing guides, READMEs, RFCs, onboarding, architecture, or review-facing docs. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/cognitive-doc-design/SKILL.md) |
| comment-writer | user | Write warm, direct collaboration comments. Trigger: PR feedback, issue replies, reviews, Slack messages, or GitHub comments. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/comment-writer/SKILL.md) |
| go-testing | user | Trigger: Go tests, go test coverage, Bubbletea teatest, golden files. Apply focused Go testing patterns. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/go-testing/SKILL.md) |
| issue-creation | user | Create Gentle AI issues with issue-first checks. Trigger: creating GitHub issues, bug reports, or feature requests. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/issue-creation/SKILL.md) |
| judgment-day | user | Trigger: judgment day, dual review, adversarial review, juzgar. Run blind dual review, fix confirmed issues, then re-judge. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/judgment-day/SKILL.md) |
| skill-creator | user | Trigger: new skills, agent instructions, documenting AI usage patterns. Create LLM-first skills with valid frontmatter. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/skill-creator/SKILL.md) |
| skill-improver | user | Trigger: improve skills, audit skills, refactor skills, skill quality. Audit and upgrade existing LLM-first skills. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/skill-improver/SKILL.md) |
| work-unit-commits | user | Plan commits as reviewable work units. Trigger: implementation, commit splitting, chained PRs, or keeping tests and docs with code. | [SKILL.md](file:///home/dsasco/.config/opencode/skills/work-unit-commits/SKILL.md) |

## Project Conventions and Manuals

These files govern code style, architecture, and behavior rules for AI agents in GolemUI.

| File | Type | Purpose / Reference |
|---|---|---|
| [AGENTS.md](file:///src/GolemUI/AGENTS.md) | Convention File | Main instruction file for AI agents in GolemUI |
| [GEMINI.md](file:///src/GolemUI/GEMINI.md) | Convention File | Main instruction file for AI agents in GolemUI |
