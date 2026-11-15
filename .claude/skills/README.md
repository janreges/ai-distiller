# AI Distiller skills

Project skills for Claude Code. Each skill lives in `.claude/skills/<name>/SKILL.md`
and states only what is true at the commit where it is written.

Skills (a line is replaced when its skill lands):

- aid-testing-debugging: not yet written
- aid-language-parsers: not yet written
- aid-architecture: not yet written
- aid-output-formatting: not yet written
- aid-release-mcp: not yet written

Conventions:

- Refer to code as `path/to/file.go#Symbol` (function, method, type, variable or
  constant), never by line number. For `Type.Method` use `file.go#Type.Method`.
- A change that makes a skill statement false fixes the skill in the same commit.
- SKILL CHECK: `sh scripts/check-skill-anchors.sh`
  (test: `sh scripts/tests/check_skill_anchors_test.sh`)
