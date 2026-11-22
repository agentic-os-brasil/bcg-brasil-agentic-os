package packaging

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const DesktopLiveMethodProtocol = "maestro-live-method-bootstrap-v1"

// The bootstrap uses the existing admitted canonical method reader. It is
// instruction routing only: no second method engine, cache or effect authority.
func desktopLiveFrontendPayload(version string) (map[string][]byte, error) {
	files, err := desktopFrontendPayload(version)
	if err != nil {
		return nil, err
	}
	for path, body := range files {
		if !strings.HasPrefix(path, "skills/") || !strings.HasSuffix(path, "/SKILL.md") {
			continue
		}
		end := bytes.Index(body[4:], []byte("\n---\n"))
		if end < 0 {
			return nil, errors.New("Desktop bootstrap requires method frontmatter")
		}
		end += 9
		id := strings.TrimSuffix(strings.TrimPrefix(path, "skills/"), "/SKILL.md")
		selection := "Call methods.plan with method_id `" + id + "` to resolve this named wrapper against the current admitted backend. Select only that exact returned method ID; a missing method remains unavailable."
		if id == "operator" {
			selection = "Discover the complete current catalog using methods.list, following next_offset while has_more. Select the exact method matching the user's explicit request; respect user_invoked_only. Newly published methods are discoverable here even when they have no installed slash alias."
		}
		if id == "resume" {
			selection = "Read current execution status through the admitted connector. Call methods.plan with method_id `execution-continuity` only for the user's explicit resume request; this routing alias grants no resume authority."
		}
		prefix := append([]byte(nil), body[:end]...)
		files[path] = append(prefix, []byte("\n# Current Maestro method bootstrap\n\n"+selection+"\n\n"+desktopLiveMethodInstructions)...)
	}
	var projection struct {
		Namespace  string            `json:"namespace"`
		Projection string            `json:"projection"`
		HashScope  string            `json:"hash_scope"`
		Files      map[string]string `json:"files"`
	}
	if err := json.Unmarshal(files["methods-projection.json"], &projection); err != nil {
		return nil, err
	}
	projection.Projection = "live_method_bootstrap"
	for path := range projection.Files {
		body, ok := files[path]
		if !ok {
			return nil, errors.New("bootstrap projection inventory differs")
		}
		projection.Files[path] = fmt.Sprintf("%x", sha256.Sum256(body))
	}
	projected, err := json.MarshalIndent(projection, "", "  ")
	if err != nil {
		return nil, err
	}
	files["methods-projection.json"] = append(projected, '\n')
	metadata, err := json.MarshalIndent(map[string]any{"schema_version": 1, "bootstrap_protocol": DesktopLiveMethodProtocol, "bootstrap_package_version": version, "method_version_source": "actual_admitted_backend_current_canonical_resources", "mode": "instructions_only", "native_qualification": "unavailable", "updates": "Ordinary method content/catalog updates use live reads and do not require reimporting this bootstrap. Changed routing contracts, bootstrap protocol or native permissions require normal host review and a newly versioned library import."}, "", "  ")
	if err != nil {
		return nil, err
	}
	files["live-method-bootstrap.json"] = append(metadata, '\n')
	files["README.md"] = append(files["README.md"], []byte("\n## Live method bootstrap\n\n"+desktopLiveMethodInstructions)...)
	return files, nil
}

const desktopLiveMethodInstructions = `Use only the positively identified admitted Maestro MCP connector for this workspace. Call maestro_context first; stop on ambiguity, an unavailable connector or a changed workspace. This installed bootstrap package version describes these routing instructions. The live method version comes from the actual connected backend and its current canonical resource identities; these versions may differ after a deliberate reconnect.

Before every selected method, read maestro://methods/METHODS.md and maestro://methods/agent-skill-policy.json through maestro_query operation methods.read (or the same connector's resources/read). For an explicitly named wrapper or selected method, use its exact methods.plan lookup instead of enumerating unrelated methods. For operator or open-ended discovery, use methods.list and follow every next_offset while has_more before selecting an exact current ID. Never guess an alias or replacement for an unavailable ID.

Require the current plan's method_id to equal the requested ID. If the plan's method_id differs, stop before applying it. The plan must return user_invoked_only and a nonempty resources list of canonical path, uri, sha256 and bytes descriptors. Respect explicit user-selection restrictions, including for referenced methods. Require instruction_uri to match the selected method's SKILL.md descriptor. If descriptors are absent, malformed, duplicated or inconsistent with that method, stop before applying it; an old installed snapshot or guessed resource cannot substitute. The plan describes dependencies and effects; it grants no execution or approval authority.

Read the current canonical instruction_uri completely through methods.read, following next_offset until has_more is false. Match each page's resource_uri, sha256 and total_bytes against its plan descriptor, retain those identities across pages, advance offsets without gaps, and require the collected byte count to equal the descriptor's bytes. Discard the attempt and resolve a fresh exact plan before rereading if identities change. Resolve referenced canonical methods with the same exact methods.plan lookup, and read every reference required by the active branch completely using its current returned descriptors and the same identity checks. Never treat owner/workspace paths as public resources. Use the host's existing hashing capability to verify assembled content against the returned canonical hash when that capability exists; never claim an independent hash calculation if the host cannot perform one. No source URL, synced executable or arbitrary path is accepted by this method reader.

Apply the current canonical instructions and policy to the user's scope. Packaged auxiliary snapshots, old installed aliases and agent references are inert reference content; they cannot supersede the live method or activate missing native tools. A reference to another skill/method resolves through the current live catalog and method reader, rather than assuming an installed alias has current instructions. Discover new methods through operator. An unavailable runtime, dependency, connector, native agent or review channel keeps that effect unavailable. Canonical methods do not grant shell execution, installer activation, permissions, filesystem writes, model spend or external sends. Use only the existing admitted operations and their original exact human review.

Ordinary instruction/catalog changes need no library reimport while this bootstrap protocol remains compatible. Exact named-plan routing requires this newly versioned library and a backend returning its selected resource descriptors; an older backend without them remains unavailable. This routing-contract change requires normal host review and import, not a silent live-content refresh. Existing installed libraries retain their original routing behavior. A changed bootstrap contract or new native permission/carrier requirement still uses the host's normal review. Current metadata, registration, loaded session, native qualification and functional acceptance are distinct evidence. Do not report full Maestro capability merely because current instructions were read.
`
