# Spec 056 — Complete engineering methods in the Maestro skillset

Status: accepted for implementation. The product owner requested the stable
Pocock methods as distributed Maestro skills and cohesive PRs that deliver
usable functionality. This is one installed-method workflow: understand a
need, specify it, implement with TDD, review and document the change.

## Product outcome

An owner selecting the existing Tech Core track receives all 27 methods listed
by the upstream 1.3.1 plugin manifest, with their references, templates and
license. Methods use `pocock-` IDs to coexist with Maestro's existing methods.
Experimental or miscellaneous upstream directories are outside that published
manifest and do not become product methods implicitly.

Pinned upstream commit and per-file source/distributed hashes identify the
import. The upstream MIT notice ships with the methods. Adaptations cover
product IDs, invocation pointers, canonical interaction profile and runtime
metadata; the method does not expand workspace or external-action authority.

## Discoverable and executable package

The canonical Tech Core catalog and release allowlist include each method and
its resources. The ZIP carries native discovery copies for Claude and Codex,
plus canonical source for the existing router. All copied method resources
remain byte-identical to the governed product sources. Package resources are
distinct from project artifacts such as a consumer repository's glossary or
issue-tracker configuration.

The experimental Code plugin also carries these 27 methods and their canonical
interaction-profile dependency. Markdown invocation references are projected
into the selected plugin namespace; frontmatter and non-Markdown resources keep
their canonical bytes. Its provenance explicitly labels canonical hashes before
this projection. The MCPB and its three synthetic tools retain their scope.

Runtime projection installs, updates, inspects and uninstalls the same full
method packages. It accounts for every resource in integrity checks, conflict
preflight, transaction snapshots and rollback. User edits or unowned collisions
block mutation before files are changed. A forged resource path cannot grant
overwrite/delete authority. Legacy projections upgrade only when new resource
paths do not collide with unowned files.

Legacy manifests without resource ownership require an explicit update before
inspection/routing can attest a complete package. Even a byte-identical file
at a previously unowned resource path is preserved as a conflict. A local
manifest alone never grants deletion authority over unknown resources.
Adding resources at free paths and reinstalling identical resources are supported.
Changing or retiring a previously managed resource fails closed: a future package
must carry reviewed prior-version migration authority before that transition can
be supported. This release does not infer such authority from local hashes.
Workspace migration reuses the same managed-path inventory, including resources,
with a finite 256-file snapshot cap and unchanged 512 KiB/file, 4 MiB total caps.

`disable-model-invocation: true` remains explicit-only in Claude metadata,
Codex metadata, the compact catalog and both product routers. An explicit
method ID can select it; an ordinary lexical match cannot. Helpers/templates
are inert until an authorized workflow uses them. Installation invokes no
models, creates no issue-tracker labels and changes no consumer repository
configuration through the upstream setup method.

## Observable acceptance

- The full catalog contains 53 base and 39 Tech Core methods: the prior 12 plus
  all 27 pinned upstream methods. IDs are distinct and sorted.
- Installed `pocock-tdd` resolves `tests.md` and `mocking.md`; domain modeling,
  prototype, setup, writing and wizard retain their referenced local material.
- The specification → implementation → review → PR method chain resolves to
  installed IDs, including explicit-only invocation rules.
- Projection `Inspect` detects changed/missing resources, and update/uninstall
  preserve customized or unowned files. No state or credentials enter packages.
- A real built ZIP contains every catalogued method/resource in its canonical
  and native discovery surfaces; the extracted router resolves a Pocock method
  to an existing file and does not auto-select an explicit-only method.
- Focused local tests and four review lenses cover the exact source tree.
  Artifact/CLI discovery, actual model invocation, native platform qualification
  and production release/signing retain separate evidence labels.

This delivery includes its content, callers, packaging, documentation and
functional tests. Full Maestro MCP domain parity and owner-state migration are
independent product capabilities; this method package is usable without them.
