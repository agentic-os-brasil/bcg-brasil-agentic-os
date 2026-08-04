// Package workspace owns the small, regenerable surface created inside a
// user-selected BCGOS work folder. Private runtime state stays outside it.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

var ErrSynchronizedWorkspace = errors.New("workspace appears to be inside a synchronized directory")

type Options struct {
	WorkspacePath         string
	DataRoot              string
	AllowSynchronizedRoot bool
}

type Result struct {
	State             string `json:"state"`
	WorkspacePath     string `json:"workspace_path"`
	WorkspaceID       string `json:"workspace_id"`
	DataRoot          string `json:"data_root"`
	Synchronized      bool   `json:"synchronized_workspace"`
	ExistingWorkspace bool   `json:"existing_workspace"`
}

type Inspection struct {
	State          string `json:"state"`
	WorkspacePath  string `json:"workspace_path"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	DataRoot       string `json:"data_root"`
	Synchronized   bool   `json:"synchronized_workspace"`
	BrainReadable  bool   `json:"brain_readable"`
	MetadataStatus string `json:"metadata_status"`
}

type manifest struct {
	SchemaVersion int    `json:"schema_version"`
	WorkspaceID   string `json:"workspace_id"`
}

const rootReadme = `# Maestro workspace

Este é o seu workspace profissional do Maestro. Ele começa com uma estrutura
local e vazia por design: nada de cliente, memória antiga ou contexto externo é
copiado para cá durante a instalação.

## Comece aqui

1. Abra este diretório no Claude Code ou Codex.
2. O Maestro verificará o onboarding e conduzirá a entrevista inicial se ela
   ainda estiver pendente.
3. Depois, preencha as áreas do brain conforme o trabalho acontece.

## Mapa

- brain/ — clientes, projetos, conhecimento, pessoas, decisões e tarefas.
- agents/ — stubs navegáveis dos papéis do Maestro.
- onboarding/ — contrato da entrevista inicial e próximos passos.
- .bcgos/ e .claude/ — configuração gerenciada do runtime; normalmente
  ficam ocultas no Finder.
`

const brainReadme = `# Meu BCGOS Brain

Este é o espaço navegável do seu segundo cérebro profissional.

Você pode abrir, ler e organizar arquivos Markdown aqui diretamente. O BCGOS
usa memória operacional privada separada para continuidade entre sessões; ela
não substitui suas fontes, decisões ou documentos neste workspace.

Sua preferência de interação fica fora deste brain e da memória. Use bcgos
profile show para consultá-la ou bcgos profile set standard|advanced|power
para ajustar a profundidade técnica das sugestões.

Estruture projetos, clientes e conhecimento reutilizável conforme seu trabalho
e os bundles instalados. Os diretórios iniciais são stubs vazios: use-os quando
o trabalho existir, sem criar contexto de cliente artificialmente.
`

var visibleSurface = []struct {
	path string
	body string
}{
	{"README.md", rootReadme},
	{"onboarding/README.md", "# Onboarding\n\nNa primeira sessão, Maestro verifica se o perfil do dono foi concluído. Se estiver pendente, ele conduz uma pergunta por vez e só trata o onboarding como completo depois da revisão e confirmação do dono.\n\nDepois da confirmação, o Maestro pergunta uma vez se o dono quer indicar pastas autorizadas do SharePoint deste projeto ou começar sem essa fonte. A escolha registra somente ponteiros locais revisados: não lê, copia, envia nem coleta documentos durante o onboarding.\n\nNenhuma memória anterior é lida, copiada ou enviada durante essa etapa.\n"},
	{"brain/clients/README.md", "# Clientes\n\nCrie uma pasta somente quando houver um cliente autorizado. Registre stakeholders, fatos e decisões no escopo correto.\n"},
	{"brain/projects/README.md", "# Projetos\n\nCrie um diretório por projeto ativo. Mantenha hipótese, entregáveis, plano de trabalho e evidências próximos ao projeto.\n"},
	{"brain/knowledge/README.md", "# Conhecimento\n\nGuarde conhecimento reutilizável que tenha fonte e escopo claros. Não copie material de cliente para esta área.\n"},
	{"brain/people/README.md", "# Pessoas\n\nUse para informações profissionais que sejam necessárias ao trabalho e tenham contexto e permissão apropriados.\n"},
	{"brain/decisions/README.md", "# Decisões\n\nRegistre decisões relevantes com racional, evidência, dono e data. A fonte humana continua sendo a autoridade.\n"},
	{"brain/tasks/README.md", "# Tarefas\n\nO Maestro mostra no início da sessão somente tarefas marcadas explicitamente como abertas no estado operacional do dono. Este diretório é para planos e artefatos de trabalho; não é um backlog inventado.\n"},
	{"agents/README.md", "# Agentes\n\nEstes são stubs de papéis. Maestro coordena; nomes, avatares e ownership de Client Account Agents e Case Agents são definidos pelo dono durante o onboarding.\n"},
	{"agents/maestro.md", "# Maestro 🎼\n\nHub do workspace: entende a intenção, decide profundidade e coordena os loops autorizados.\n"},
	{"agents/walter.md", "# Walter 🧭\n\nSenior advisor e proxy do self do dono para tarefas de maior leverage. Refina; não é um naysayer.\n"},
	{"agents/darwin.md", "# Darwin 🧬\n\nMeta-harness: observa saúde, housekeeping e caminhos de evolução do Agentic OS.\n"},
	{"agents/pa-experts.md", "# PA Experts 🧠\n\nEspecialistas funcionais e industriais consultivos. São versionados e evoluem ao longo do tempo; nesta instalação permanecem stubs.\n"},
	{"agents/client-accounts/README.md", "# Client Account Agents\n\nStubs para os agentes que exercem visão estratégica de conta e stakeholders. O dono define nome, avatar e ownership.\n"},
	{"agents/cases/README.md", "# Case Agents\n\nStubs para os agentes especializados na execução de cada projeto. O dono define nome, avatar e ownership.\n"},
}

// DefaultDataRoot resolves per-user application storage without placing BCGOS
// state below a workspace or a cloud-synchronized Documents folder.
func DefaultDataRoot(platform, home, localAppData, xdgStateHome string) (string, error) {
	switch platform {
	case "windows":
		if strings.TrimSpace(localAppData) == "" {
			return "", errors.New("LOCALAPPDATA is required on Windows")
		}
		return strings.TrimRight(localAppData, `\\/`) + `\BCGOS`, nil
	case "darwin":
		if strings.TrimSpace(home) == "" {
			return "", errors.New("home directory is required on macOS")
		}
		return pathpkg.Join(home, "Library", "Application Support", "BCGOS"), nil
	case "linux":
		if strings.TrimSpace(xdgStateHome) != "" {
			return pathpkg.Join(xdgStateHome, "bcgos"), nil
		}
		if strings.TrimSpace(home) == "" {
			return "", errors.New("home directory is required on Linux")
		}
		return pathpkg.Join(home, ".local", "share", "bcgos"), nil
	default:
		return "", fmt.Errorf("unsupported platform %q", platform)
	}
}

func Initialize(options Options) (Result, error) {
	workspacePath, dataRoot, err := normalize(options)
	if err != nil {
		return Result{}, err
	}
	synchronized := IsSynchronizedPath(workspacePath)
	if synchronized && !options.AllowSynchronizedRoot {
		return Result{}, ErrSynchronizedWorkspace
	}

	existing, err := pathExists(workspacePath)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(workspacePath, 0o700); err != nil {
		return Result{}, err
	}
	for _, directory := range []string{"config", "memory", "scheduler", "logs"} {
		if err := os.MkdirAll(filepath.Join(dataRoot, directory), 0o700); err != nil {
			return Result{}, err
		}
	}

	id := workspaceID(workspacePath)
	metadataRoot := filepath.Join(workspacePath, ".bcgos")
	if err := os.MkdirAll(metadataRoot, 0o700); err != nil {
		return Result{}, err
	}
	manifestPath := filepath.Join(metadataRoot, "workspace.json")
	if existingManifest, err := readManifest(manifestPath); err == nil {
		if existingManifest.WorkspaceID != id {
			return Result{}, errors.New("workspace metadata does not match this path")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := writeManifest(manifestPath, manifest{SchemaVersion: 1, WorkspaceID: id}); err != nil {
			return Result{}, err
		}
	} else {
		return Result{}, err
	}
	if err := ensureBrainReadme(workspacePath); err != nil {
		return Result{}, err
	}
	if err := ensureVisibleSurface(workspacePath); err != nil {
		return Result{}, err
	}

	return Result{State: "initialized", WorkspacePath: workspacePath, WorkspaceID: id, DataRoot: dataRoot, Synchronized: synchronized, ExistingWorkspace: existing}, nil
}

func Inspect(workspacePath, dataRoot string) (Inspection, error) {
	workspacePath, dataRoot, err := normalize(Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		return Inspection{}, err
	}
	inspection := Inspection{WorkspacePath: workspacePath, DataRoot: dataRoot, Synchronized: IsSynchronizedPath(workspacePath), MetadataStatus: "missing"}
	value, err := readManifest(filepath.Join(workspacePath, ".bcgos", "workspace.json"))
	if errors.Is(err, os.ErrNotExist) {
		inspection.State = "uninitialized"
		return inspection, nil
	}
	if err != nil {
		inspection.State = "invalid"
		inspection.MetadataStatus = "invalid"
		return inspection, nil
	}
	if value.WorkspaceID != workspaceID(workspacePath) {
		inspection.State = "invalid"
		inspection.MetadataStatus = "path_mismatch"
		return inspection, nil
	}
	inspection.WorkspaceID = value.WorkspaceID
	inspection.MetadataStatus = "valid"
	brainInfo, err := os.Stat(filepath.Join(workspacePath, "brain", "README.md"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Inspection{}, err
	}
	inspection.BrainReadable = err == nil && !brainInfo.IsDir()
	if inspection.Synchronized {
		inspection.State = "warning"
	} else if !inspection.BrainReadable {
		inspection.State = "incomplete"
	} else {
		inspection.State = "ready"
	}
	return inspection, nil
}

func IsSynchronizedPath(value string) bool {
	for _, segment := range strings.FieldsFunc(value, func(character rune) bool { return character == '/' || character == '\\' }) {
		normalized := strings.ToLower(strings.TrimSpace(segment))
		if normalized == "onedrive" || strings.HasPrefix(normalized, "onedrive -") || normalized == "dropbox" || normalized == "google drive" || normalized == "googledrive" {
			return true
		}
	}
	return false
}

func normalize(options Options) (string, string, error) {
	if strings.TrimSpace(options.WorkspacePath) == "" || strings.TrimSpace(options.DataRoot) == "" {
		return "", "", errors.New("workspace path and data root are required")
	}
	workspacePath, err := filepath.Abs(options.WorkspacePath)
	if err != nil {
		return "", "", err
	}
	dataRoot, err := filepath.Abs(options.DataRoot)
	if err != nil {
		return "", "", err
	}
	if IsSynchronizedPath(dataRoot) {
		return "", "", errors.New("BCGOS local data directory cannot be inside a synchronized root")
	}
	if sameOrNested(dataRoot, workspacePath) || sameOrNested(workspacePath, dataRoot) {
		return "", "", errors.New("workspace and BCGOS local data must be separate")
	}
	return workspacePath, dataRoot, nil
}

func ensureBrainReadme(workspacePath string) error {
	brainRoot := filepath.Join(workspacePath, "brain")
	if err := os.MkdirAll(brainRoot, 0o700); err != nil {
		return err
	}
	path := filepath.Join(brainRoot, "README.md")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := file.WriteString(brainReadme); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func ensureVisibleSurface(workspacePath string) error {
	for _, entry := range visibleSurface {
		if err := ensureFile(filepath.Join(workspacePath, filepath.FromSlash(entry.path)), entry.body); err != nil {
			return err
		}
	}
	return nil
}

func ensureFile(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func workspaceID(path string) string {
	digest := sha256.Sum256([]byte("bcgos-workspace-v1\x00" + filepath.Clean(path)))
	return hex.EncodeToString(digest[:16])
}

func sameOrNested(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func readManifest(path string) (manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return manifest{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var value manifest
	if err := decoder.Decode(&value); err != nil {
		return manifest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return manifest{}, errors.New("workspace metadata contains multiple JSON values")
		}
		return manifest{}, err
	}
	if value.SchemaVersion != 1 || value.WorkspaceID == "" {
		return manifest{}, errors.New("invalid workspace metadata")
	}
	return value, nil
}

func writeManifest(path string, value manifest) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
