# Claude Instructions for DTF

Before working in this repository, read and follow [AGENTS.md](AGENTS.md) in full.
It is the shared baseline for all agents; this file does not replace or weaken it.

In particular, follow its [Code and Comments](AGENTS.md#code-and-comments) rules:

- Separate logical steps with blank lines while keeping operations and their
  immediate error checks together. Do not produce one dense script-like block.
- Use operation-specific error names where multiple scopes or operations make
  `err` ambiguous, and avoid shadowing an outer or named return error.
- Return useful context at meaningful boundaries with `fmt.Errorf` and `%w`,
  preserving the original cause without redundant wrappers or secret data.
- Review readability after formatting; passing lint alone is not sufficient.

The detailed rules and Go example live in `AGENTS.md`. Apply them to new or edited
code without introducing unrelated repository-wide formatting or renames.
