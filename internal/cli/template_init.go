package cli

import (
	"fmt"
	"log/slog"

	"github.com/fiwon123/eznit/internal/domain/templates"
)

// fence is a markdown code fence.
//
// The starter content below is a raw string literal, and a raw string cannot
// hold a backtick — which a markdown code fence is made of. The fence is
// concatenated in instead.
const fence = "```"

// Template Init Cmd writes a starter structure into a folder.
//
// This is the path that needs no server and no login: the files are built
// into the binary, so `eznit template init` works on a machine that has
// never seen eznit.
type TemplateInitCmd struct {
	Kind   string `help:"the terminal the starter targets" enum:"opencode,codex,copilot,cursor,windsurf,generic" default:"generic"`
	Target string `help:"the folder to write into" default:"."`
	Force  bool   `help:"overwrite the files that are already there"`
}

// Run writes the starter structure.
func (cmd *TemplateInitCmd) Run(g *Globals) error {
	target := cmd.Target
	if target == "" {
		target = "."
	}

	files := starterTemplate(cmd.Kind)

	// The files are checked before the questions are asked: a user is not
	// asked what the starter wants to know when the starter is not going to
	// be written.
	if err := checkTemplateFiles(target, files, cmd.Force); err != nil {
		g.logger.Warn("the starter was not written. ", slog.String("error", err.Error()))
		return nil
	}

	values, ok := askTemplateValues(starterVariables())
	if !ok {
		return nil
	}

	for i := range files {
		files[i].Content = substitute(files[i].Content, values)
	}

	if err := writeTemplateFiles(target, files, cmd.Force); err != nil {
		g.logger.Warn("the starter was not written. ", slog.String("error", err.Error()))
		return nil
	}

	fmt.Println()
	fmt.Printf("wrote the %s starter into %s (%d files)\n", cmd.Kind, target, len(files))
	for _, file := range files {
		fmt.Printf("  %s\n", file.Path)
	}
	fmt.Println()

	return nil
}

// starterVariables are the questions every starter asks.
func starterVariables() []templates.TemplateVariableData {
	return []templates.TemplateVariableData{
		{
			Key:      "PROJECT_NAME",
			Prompt:   "the name of the project",
			Required: true,
		},
		{
			Key:    "PROJECT_DESCRIPTION",
			Prompt: "what the project does, in one sentence",
		},
		{
			Key:          "LANGUAGE",
			Prompt:       "the language the project is written in",
			DefaultValue: "Go",
		},
	}
}

// starterTemplate returns the files the starter writes for a terminal.
//
// The starter is a template like any other: the same substitution, the same
// rules about where a file may land. It just happens to be built into the
// binary instead of fetched from the catalogue.
func starterTemplate(kind string) []templateFile {
	files := []templateFile{
		{
			Path:    "AGENTS.md",
			Content: starterAgentsMd,
		},
	}

	switch kind {
	case templates.KindOpencode:
		files = append(files,
			templateFile{
				Path:    ".opencode/agents/build.md",
				Content: starterOpencodeBuild,
			},
			templateFile{
				Path:    ".opencode/agents/plan.md",
				Content: starterOpencodePlan,
			},
			templateFile{
				Path:    ".opencode/agents/reviewer.md",
				Content: starterOpencodeReviewer,
			},
		)
	case templates.KindCopilot:
		files = append(files,
			templateFile{
				Path:    ".github/copilot-instructions.md",
				Content: starterCopilotInstructions,
			},
		)
	case templates.KindCursor:
		files = append(files,
			templateFile{
				Path:    ".cursor/rules/project.mdc",
				Content: starterCursorRule,
			},
		)
	case templates.KindWindsurf:
		files = append(files,
			templateFile{
				Path:    ".windsurf/rules/project.md",
				Content: starterWindsurfRule,
			},
		)
	}

	return files
}

// starterAgentsMd is the AGENTS.md the starter writes. It is the file every
// terminal reads, so it carries the project identity.
const starterAgentsMd = `# {{PROJECT_NAME}}

{{PROJECT_DESCRIPTION}}

## Project Objective

What the project does, in one sentence, and the problem it solves.

## Main Technology

- Language: {{LANGUAGE}}
- Tests: the framework the project uses
- Storage: what the project keeps its data in

## Core Architecture

` + fence + `
cmd/            the entrypoints
internal/       the domain packages
pkg/            the shared helpers
` + fence + `

## Key Conventions

- Conventional commits: ` + "`feat:`" + `, ` + "`fix:`" + `, ` + "`refactor:`" + `, ` + "`docs:`" + `, ` + "`test:`" + `, ` + "`chore:`" + `, ` + "`ci:`" + `
- Layering is strict: a handler decodes and writes, a service holds the rules,
  a repository is the only place that speaks the storage
- Migrations are append-only: never edit a migration that has run

## Development Commands

` + fence + `bash
make check      # the local verification gate
make run        # run the project
` + fence + `

## Git Workflow

- Branch from ` + "`dev`" + `, never from ` + "`main`" + `
- Branch naming: ` + "`<type>/<issue-number>-<slug>`" + `
- Conventional commits
- Do not merge a pull request without an explicit instruction
`

// starterOpencodeBuild is the build agent of the opencode starter.
const starterOpencodeBuild = `---
description: Primary agent for feature development, bug fixes, and refactoring
mode: primary
---

You are a senior developer responsible for implementing features, fixing bugs,
and refactoring code in this project.

## Project Context

Read ` + "`AGENTS.md`" + ` for project-specific information: tech stack,
architecture, conventions. ` + "`PROJECT_CONTEXT.md`" + ` holds the structured
project identity — problem, goal, stack, topology.

## Responsibilities

- Implement features from issues
- Fix bugs and resolve issues
- Refactor code while maintaining functionality
- Manage the full issue lifecycle: issues, pull requests, reviews

## Plan first

You are the agent that changes files, so you are the last checkpoint before
work happens. If the task has no agreed plan, do not open an editor and start
inferring what was meant.

Before the first edit, confirm all three:

1. An issue exists for this work.
2. A plan exists: goal, files, dependencies, risks, steps.
3. The user has seen it and not objected.

## Rules

- Follow the existing code patterns and conventions
- Run the project's check target before committing
- Never push directly to a release branch
- One feature per pull request
`

// starterOpencodePlan is the plan agent of the opencode starter.
const starterOpencodePlan = `---
description: Analysis and planning without making changes
mode: primary
---

You analyse and plan. You do not change files.

## What you do

- Read the codebase and the issue
- Work out the goal, the files to touch, the dependencies, the risks and the
  steps
- Write the plan down and show it to the user before anything is built

## Rules

- No edits: a plan that changes files is a change without a plan
- Every plan names the files it touches and the risks it carries
- Ask about what is ambiguous instead of guessing
`

// starterOpencodeReviewer is the reviewer agent of the opencode starter.
const starterOpencodeReviewer = `---
description: Reviews code for bugs, security issues, and regressions
mode: subagent
---

You review code. You do not change it.

## What you look for

- Bugs: the wrong condition, the missed error, the off-by-one
- Security: the input that is trusted because it should not be, the path that
  is built from untrusted input, the secret that lands in a log
- Regressions: the behaviour that changed without the diff saying so

## How you report

- One finding per note: where it is, why it is wrong, what it breaks
- Name the file and the line: a finding without a place is not a finding
- Say plainly when the code is fine — silence is not a review
`

// starterCopilotInstructions is the copilot starter: the same project
// identity, in the file GitHub Copilot reads.
const starterCopilotInstructions = `# {{PROJECT_NAME}}

{{PROJECT_DESCRIPTION}}

## Project Objective

What the project does, in one sentence, and the problem it solves.

## Main Technology

- Language: {{LANGUAGE}}

## Conventions

- Conventional commits: ` + "`feat:`" + `, ` + "`fix:`" + `, ` + "`refactor:`" + `, ` + "`docs:`" + `, ` + "`test:`" + `, ` + "`chore:`" + `, ` + "`ci:`" + `
- Follow the existing code patterns: read the surrounding code before writing
  new code
- Run the project's check target before committing

## Commands

` + fence + `bash
make check      # the local verification gate
make run        # run the project
` + fence + `
`

// starterCursorRule is the cursor starter: the project rules, in the file
// Cursor reads.
const starterCursorRule = `---
description: Project rules for the AI editor
globs: **/*
alwaysApply: true
---

# {{PROJECT_NAME}}

{{PROJECT_DESCRIPTION}}

## Main Technology

- Language: {{LANGUAGE}}

## Conventions

- Conventional commits: ` + "`feat:`" + `, ` + "`fix:`" + `, ` + "`refactor:`" + `, ` + "`docs:`" + `, ` + "`test:`" + `, ` + "`chore:`" + `, ` + "`ci:`" + `
- Read the surrounding code before writing new code
- Run the project's check target before committing
`

// starterWindsurfRule is the windsurf starter: the project rules, in the
// file Windsurf reads.
const starterWindsurfRule = `---
trigger: always_on
description: Project rules for the AI editor
---

# {{PROJECT_NAME}}

{{PROJECT_DESCRIPTION}}

## Main Technology

- Language: {{LANGUAGE}}

## Conventions

- Conventional commits: ` + "`feat:`" + `, ` + "`fix:`" + `, ` + "`refactor:`" + `, ` + "`docs:`" + `, ` + "`test:`" + `, ` + "`chore:`" + `, ` + "`ci:`" + `
- Read the surrounding code before writing new code
- Run the project's check target before committing
`
