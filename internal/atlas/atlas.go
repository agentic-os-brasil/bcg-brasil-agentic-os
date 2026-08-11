// Package atlas owns the minimal human-readable, local atlas bootstrap. It
// does not compile a wiki, index memory, or grant cross-scope access.
package atlas

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

type Options struct {
	DataRoot      string
	WorkspacePath string
	WorkspaceID   string
	Now           func() time.Time
}

const dailyTemplate = "# Daily — YYYY-MM-DD\n\n> Human work log for this workspace. Raw entries are not memory input. An approved sanitization route may select daily signals to compose L1 alongside Claude/Codex session signals.\n\n## Related scope\n- **Projects:**\n- **Clients:**\n\n## Priorities\n1.\n2.\n3.\n\n## Notes\n-\n\n## Decisions surfaced\n- <link to or update the authoritative project decision>\n\n## Learning candidates\n- <owner-private learning pointer; do not copy private owner content here>\n\n## Carry forward\n-\n"

type Pointer struct {
	Path      string `json:"path"`
	Available bool   `json:"available"`
	State     string `json:"state"`
}

type Status struct {
	Managed   Pointer `json:"managed"`
	Owner     Pointer `json:"owner"`
	Workspace Pointer `json:"workspace"`
}

var ownerFiles = map[string]string{
	"index.md":             "# Owner atlas\n\nNavegacao humana para conhecimento profissional privado do owner. Este atlas nao inclui o SELF: use `owner/self/` para identidade, voz e preferencias.\n",
	"learnings/index.md":   "# Learnings\n\nAprendizados profissionais duraveis, corrigiveis e ligados a suas fontes quando aplicavel.\n",
	"development/index.md": "# Development\n\nObjetivos, retros e evidencias de desenvolvimento profissional. Trate este conteudo como sensivel.\n",
	"concepts/index.md":    "# Personal concepts\n\nMetodos e playbooks pessoais reutilizaveis. Promocao para conhecimento compartilhado requer governanca separada.\n",
}

var workspaceFiles = map[string]string{
	"index.md":                     "# Workspace atlas\n\nNavegacao humana para este workspace somente. Nao use este atlas como fonte para outro cliente ou workspace.\n",
	"clients/index.md":             "# Clients\n\nContexto de clientes autorizado para este workspace.\n",
	"clients/template-client.md":   "# Client: <name>\n\n> Crie uma pagina por cliente somente quando este workspace estiver autorizado a conter esse contexto.\n\n## Snapshot\n- **Organization / business unit:**\n- **Relationship context:**\n- **Sensitivity:** client_restricted\n- **Source / as of:**\n\n## Stakeholders\n- **Name / role / relevance:**\n\n## Current context\n-\n\n## Related\n- [Projects](../projects/index.md)\n- [Daily](../daily/index.md)\n",
	"projects/index.md":            "# Projects\n\nProjetos e workstreams deste workspace, com fontes e artefatos vinculados.\n",
	"projects/template-project.md": "# Project / workstream: <name>\n\n> Uma pagina por projeto ou workstream deste workspace. Cite fontes; nao replique fatos criticos em outras paginas.\n\n## Snapshot\n- **Client:**\n- **Owner / role:**\n- **Status:** on_track | at_risk | blocked\n- **Next milestone / date:**\n\n## Objective\n-\n\n## Current truth\n| Fact | Value | As of | Source |\n|---|---|---|---|\n| | | | |\n\n## Current state\n-\n\n## Workplan\n- [ ] <step> — <owner> — <due date>\n\n## Decisions\n> Record the durable decision, its source and review date. Do not treat generated memory as authority.\n\n### <decision-id> — YYYY-MM-DD — <title>\n- **Decision:**\n- **Context / source:**\n- **Review by:**\n- **Status:** active | superseded\n\n## Risks / blockers\n-\n\n## Key artifacts\n-\n\n## Related\n- [Clients](../clients/index.md)\n- [People](../people/index.md)\n- [Daily](../daily/index.md)\n",
	"people/index.md":              "# People\n\nPessoas relevantes neste workspace. Registre somente informacao profissional necessaria e autorizada.\n",
	"people/template-person.md":    "# Person: <name>\n\n> Professional context only. Keep the minimum necessary information, identify the source and correct or remove it when no longer justified.\n\n## Snapshot\n- **Role / organization:**\n- **Relationship to this workspace:**\n- **Sensitivity:** professional_restricted\n- **Source / as of:**\n\n## Working context\n- **Collaboration preferences observed:**\n- **Communication considerations:**\n\n## Workspace interactions\n- YYYY-MM-DD — <factual, necessary note>\n\n## Related\n- [Projects](../projects/index.md)\n- [Clients](../clients/index.md)\n",
	"daily/index.md":               "# Daily\n\nRegistros humanos deste workspace. Eles so podem alimentar memoria por uma rota de sanitizacao aprovada.\n",
	"daily/template-daily.md":      dailyTemplate,
}

func Initialize(options Options) (Status, error) {
	if strings.TrimSpace(options.DataRoot) == "" || strings.TrimSpace(options.WorkspacePath) == "" || strings.TrimSpace(options.WorkspaceID) == "" {
		return Status{}, errors.New("data root, workspace path and workspace id are required")
	}
	inspection, err := workspace.Inspect(options.WorkspacePath, options.DataRoot)
	if err != nil {
		return Status{}, err
	}
	if inspection.State == "uninitialized" || inspection.State == "invalid" || inspection.State == "incomplete" || inspection.WorkspaceID != options.WorkspaceID {
		return Status{}, errors.New("atlas bootstrap requires the registered workspace identity")
	}
	ownerRoot, err := scheduler.CanonicalPrivatePath(filepath.Join(options.DataRoot, "atlas", "owner"))
	if err != nil {
		return Status{}, err
	}
	workspaceRoot, err := scheduler.CanonicalPrivatePath(filepath.Join(options.WorkspacePath, "brain"))
	if err != nil {
		return Status{}, err
	}
	if err := createFiles(ownerRoot, ownerFiles); err != nil {
		return Status{}, err
	}
	if err := createFiles(workspaceRoot, workspaceFiles); err != nil {
		return Status{}, err
	}
	now := time.Now()
	if options.Now != nil {
		now = options.Now()
	}
	day := now.Format("2006-01-02")
	if err := createFiles(workspaceRoot, map[string]string{
		filepath.ToSlash(filepath.Join("daily", day+".md")): strings.Replace(dailyTemplate, "YYYY-MM-DD", day, 1),
	}); err != nil {
		return Status{}, err
	}
	return Inspect(options), nil
}

func Inspect(options Options) Status {
	ownerRoot := filepath.Join(options.DataRoot, "atlas", "owner")
	workspaceRoot := filepath.Join(options.WorkspacePath, "brain")
	return Status{
		Managed:   Pointer{State: "unavailable"},
		Owner:     pointer(ownerRoot),
		Workspace: pointer(workspaceRoot),
	}
}

func createFiles(root string, files map[string]string) error {
	if err := scheduler.EnsurePrivateDirectory(root); err != nil {
		return err
	}
	for relative, body := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := scheduler.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		err := scheduler.WriteNewPrivateFile(path, []byte(body))
		if errors.Is(err, os.ErrExist) {
			if _, validateErr := scheduler.ReadPrivateFile(path, 16<<20); validateErr != nil {
				return validateErr
			}
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func pointer(path string) Pointer {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return Pointer{Path: path, Available: true, State: "available"}
	}
	return Pointer{Path: path, State: "missing"}
}
