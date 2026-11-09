# 🧩 Templates and skills

A **template** is a folder you keep once and write onto other machines: agent
instructions, editor rules, skills, scripts, project scaffolding. Publishing it
makes one copy the source of truth; applying it is the setup step.

You do not need a template to start: `eznit template init` writes a built-in
starter offline. Publishing and applying through the API need a running eznit
instance; an **offline bundle** needs neither (see
[Take it with you](#-take-it-with-you-no-server)).

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

The library round-trips through a crower pack
([CLI Reference](https://github.com/fiwon123/eznit/wiki/CLI-Reference#-commands-library)),
for moving the same commands to a workflow manager that already uses
[crower](https://github.com/fiwon123/crower).

## 📦 Take it with you (no server)

A template can leave the server as one file, and come back out on a machine that
has never seen eznit:

```sh
eznit template export <id> -o setup.eznit     # needs a session for a private template
eznit template apply --from-file setup.eznit --target ../another-project
```

The bundle is JSON (`format: 1`) holding the files — text and binary — and the
variables with their prompts and defaults. A command the template pulls in is
written at its target path, so the bundle is a snapshot to apply, not a publish
request. The same path rules and limits are checked when it is read; a bundle
cannot be a way around them. `apply --from-file` asks the variables locally, so
no answer leaves the machine.

`export` refuses to overwrite an existing file without `--force`.

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

- **Crower packs** — move a command library between eznit and
  [crower](https://github.com/fiwon123/crower)
  ([#185](https://github.com/fiwon123/eznit/issues/185)).
- **Device-to-device transfer** over cthrone. See
  [Direction](https://github.com/fiwon123/eznit/wiki/Direction).

An offline bundle is how sharing works today without a shared instance:
`template export` and `apply --from-file`, above.
