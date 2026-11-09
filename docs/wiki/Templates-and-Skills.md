# 🧩 Templates and skills

A **template** is a folder you keep once and write onto other machines: agent
instructions, editor rules, skills, scripts, project scaffolding. Publishing it
makes one copy the source of truth; applying it is the setup step.

You do not need a template to start: `eznit template init` writes a built-in
starter offline. You do need a running eznit instance to publish one or to apply
someone else's.

## 🗂️ What goes in a template

Any file, in any folder shape. These are the ones people actually keep:

| What | Example paths |
| --- | --- |
| Agent instructions | `AGENTS.md`, `CLAUDE.md` |
| opencode agents and skills | `.opencode/agents/build.md`, `.opencode/skills/deploy/SKILL.md` |
| Editor rules | `.cursor/rules/project.mdc`, `.windsurf/rules/project.md`, `.github/copilot-instructions.md` |
| Shell configuration | `shell/zshrc.sh`, `shell/profile.ps1` |
| Project files | `Makefile`, `.editorconfig`, `.gitignore`, `docker-compose.yml` |
| Binary helpers | an image, a compiled script, a font — anything non-text |

A worked folder:

```text
my-setup/
├── AGENTS.md
├── .opencode/
│   ├── agents/build.md
│   └── skills/deploy/SKILL.md
├── .cursor/rules/project.mdc
└── shell/zshrc.sh
```

Files are written relative to the target folder; `..` and absolute paths are
refused. A file marked executable is written with mode `0755` when applied.

## 📤 Publish, list, show, apply

```sh
eznit template publish ./my-setup      # prompts name, kind, description, public?
eznit template publish ./my-setup --public --commands <id>=shell/zshrc.sh
eznit template list                    # public catalogue + your own
eznit template list --mine --kind opencode
eznit template show <id>               # the files and the variables
eznit template apply <id> --target ../other-project [--force]
```

`publish` reads every file in the folder, skipping symlinks and files over the
limits. `--force` on `apply` overwrites files that already exist; without it, an
existing file stops the apply before anything is written.

Only a public template can be applied without a session. Your own private
templates need `eznit login`.

## 🔤 Variables

Write a placeholder in any text file and it becomes a variable:

```markdown
# {{PROJECT_NAME}}

{{PROJECT_DESCRIPTION}}
```

On `publish`, each distinct placeholder is asked once for a **default value**.
An empty default makes the variable required: `apply` will ask for it.

On `apply`, the values are asked on the machine that writes the files and
substituted into text files. A binary file is copied byte for byte. A
placeholder the template does not declare is left alone and reported, so you can
see it.

Variable names are upper case (`PROJECT_NAME`), up to 50 per template.

## ⚙️ Commands pulled in by id

A command is a script kept once in your library and referenced by templates, so
updating it updates every template that pulls it in.

```sh
eznit command add scripts/fmt.sh --name fmt      # keep it once
eznit command list
eznit template publish . --commands <command-id>=bin/fmt
eznit command remove fmt                          # refused while a template uses it
```

A command name is unique per user, not globally. `command remove` answers
`409` while a template still references it — a published template does not
quietly lose a script.

## 🧮 Limits

Checked by the CLI before the upload and by the server on the request:

| Thing | Limit |
| --- | --- |
| Files per template | 200 |
| Text file | 256 KiB |
| Text across the template | 4 MiB |
| Binary file | 10 MiB |
| Binary across the template | 32 MiB |
| Variables | 50 |
| Commands per template | 50 |

## 🔒 What does not travel

- **Variable values.** They are asked where the files are written, never stored
  on the server.
- **Your git token.** It stays in `~/.eznit/config.json` on your machine.

## 🚧 Not available yet

- **Offline bundles** — export a template to one file and apply it with no
  server ([#184](https://github.com/fiwon123/eznit/issues/184)).
- **Crower packs** — move a command library between eznit and
  [crower](https://github.com/fiwon123/crower)
  ([#185](https://github.com/fiwon123/eznit/issues/185)).
- **Device-to-device transfer** over cthrone. See
  [Direction](https://github.com/fiwon123/eznit/wiki/Direction).

Until then, sharing means a running instance: `publish --public` and anyone with
the id can `apply` it.
