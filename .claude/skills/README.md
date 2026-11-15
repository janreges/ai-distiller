# AI Distiller skills

Project skills for Claude Code. Each skill lives in `.claude/skills/<name>/SKILL.md`
and states only what is true at the commit where it is written.

Skills:

- aid-testing-debugging: running and updating tests, known gaps, finding a bug's layer, debug output
- aid-language-parsers: the 12 language processors, the parser each really uses, visibility, method attachment, IR conventions, adding a language
- aid-architecture: the language-agnostic core: data flow, CLI modes, flags to options, file discovery, the stripper, output naming, which packages are live
- aid-output-formatting: how IR becomes output: the per-language text formatters, the shared body and comment renderers, md/jsonl/json-structured/xml, the import filter, the summary
- aid-release-mcp: the tag as the only version, the release workflow and its re-runs, cgo cross-builds, the archive naming contract, npm Trusted Publishing, the installers, the MCP server and its tool-to-flag mapping

Conventions:

- Refer to code as `path/to/file.go#Symbol` (function, method, type, variable or
  constant), never by line number. For `Type.Method` use `file.go#Type.Method`.
- A change that makes a skill statement false fixes the skill in the same commit.
- SKILL CHECK: `sh scripts/check-skill-anchors.sh`
  (test: `sh scripts/tests/check_skill_anchors_test.sh`)
