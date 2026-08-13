package releasepack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
)

type ScriptPortableOptions struct {
	Root      string
	Output    string
	Version   string
	TargetOS  string
	Allowlist string
}

type ScriptPortableResult struct {
	Output     string `json:"output"`
	SHA256     string `json:"sha256"`
	Status     string `json:"status"`
	Provenance string `json:"provenance"`
	Checksum   string `json:"checksum"`
}

type scriptPortableProvenance struct {
	SchemaVersion       int      `json:"schema_version"`
	Product             string   `json:"product"`
	Version             string   `json:"version"`
	TargetOS            string   `json:"target_os"`
	DistributionProfile string   `json:"distribution_profile"`
	Runtime             string   `json:"runtime"`
	Status              string   `json:"status"`
	Retained            []string `json:"retained"`
	Degraded            []string `json:"degraded"`
	Unavailable         []string `json:"unavailable"`
	ProjectedSkills     []string `json:"projected_skills"`
	NativeOnlySkills    []string `json:"native_only_skills"`
}

func BuildScriptPortable(options ScriptPortableOptions) (ScriptPortableResult, error) {
	if !portableVersionPattern.MatchString(options.Version) {
		return ScriptPortableResult{}, errors.New("script portable version must be MAJOR.MINOR.PATCH")
	}
	if !filepath.IsAbs(options.Root) || !filepath.IsAbs(options.Output) {
		return ScriptPortableResult{}, errors.New("script portable root and output must be absolute")
	}
	profile, runtimeName := scriptPortableProfile(options.TargetOS)
	if profile == "" {
		return ScriptPortableResult{}, errors.New("script portable target must be macos or windows")
	}
	expected := "Maestro-Portable-" + options.Version + "-" + profile + ".zip"
	if filepath.Base(options.Output) != expected {
		return ScriptPortableResult{}, fmt.Errorf("script portable output must be named %s", expected)
	}
	checksumPath := options.Output + ".sha256"
	provenancePath := options.Output + ".provenance.json"
	for _, output := range []string{options.Output, checksumPath, provenancePath} {
		if _, err := os.Lstat(output); err == nil {
			return ScriptPortableResult{}, fmt.Errorf("script portable output already exists: %s", output)
		} else if !errors.Is(err, os.ErrNotExist) {
			return ScriptPortableResult{}, err
		}
	}
	if options.Allowlist == "" {
		options.Allowlist = filepath.Join(options.Root, "bundles", "base", "distribution.json")
	} else if !filepath.IsAbs(options.Allowlist) {
		return ScriptPortableResult{}, errors.New("script portable allowlist must be absolute")
	}
	allowlist, err := LoadAllowlist(options.Allowlist)
	if err != nil {
		return ScriptPortableResult{}, fmt.Errorf("load script portable allowlist: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(options.Output), 0o755); err != nil {
		return ScriptPortableResult{}, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(options.Output), ".maestro-script-portable-")
	if err != nil {
		return ScriptPortableResult{}, err
	}
	defer os.RemoveAll(staging)
	rootName := strings.TrimSuffix(expected, ".zip")
	packageRoot := filepath.Join(staging, rootName)
	if err := projectScriptPayload(options.Root, packageRoot, allowlist); err != nil {
		return ScriptPortableResult{}, err
	}
	projectedSkills, nativeOnlySkills, err := prepareScriptSkillProjection(packageRoot)
	if err != nil {
		return ScriptPortableResult{}, err
	}
	if err := prepareScriptAgentProjection(packageRoot); err != nil {
		return ScriptPortableResult{}, err
	}
	if err := pruneScriptProjectionSources(packageRoot); err != nil {
		return ScriptPortableResult{}, err
	}
	provenance := scriptPortableProvenance{
		SchemaVersion: 1, Product: "maestro", Version: options.Version,
		TargetOS: options.TargetOS, DistributionProfile: profile, Runtime: runtimeName,
		Status:           "script-only-controlled-beta",
		Retained:         []string{"managed_skills", "managed_claude_agents", "managed_agent_route_completion_assurance", "orientation", "onboarding", "atlas", "managed_policies", "claude_script_hooks", "reviewed_continuity", "reviewed_session_profile", "content_update", "content_rollback"},
		Degraded:         []string{"claude_hooks_best_effort", "agent_route_lite_v1", "continuity_lite_v1", "session_profile_lite_v1", "workspace_projection_recoverable_not_atomic", "integrity_without_publisher_authentication"},
		Unavailable:      []string{"native_cli", "codex_runtime_adapter", "signed_release_verification", "secure_provider_auth", "authenticated_native_hook_receipts", "external_mutation_challenge", "native_subagent_route_enforcement", "native_scheduler", "background_maintenance", "native_ingestion", "cli_ledgers"},
		ProjectedSkills:  projectedSkills,
		NativeOnlySkills: nativeOnlySkills,
	}
	provenanceBody, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return ScriptPortableResult{}, err
	}
	provenanceBody = append(provenanceBody, '\n')
	capabilitiesBody, err := json.MarshalIndent(map[string]any{
		"schema_version": 1, "product": "maestro", "version": options.Version,
		"distribution_profile": profile, "runtime": runtimeName,
		"retained": provenance.Retained, "degraded": provenance.Degraded, "unavailable": provenance.Unavailable,
		"projected_skills": provenance.ProjectedSkills, "native_only_skills": provenance.NativeOnlySkills,
		"claim": "managed_projection_with_script_hooks",
	}, "", "  ")
	if err != nil {
		return ScriptPortableResult{}, err
	}
	capabilitiesBody = append(capabilitiesBody, '\n')
	common := map[string]struct {
		body []byte
		mode os.FileMode
	}{
		"COMECE-AQUI.txt":          {scriptPortableStartHere(options.TargetOS), 0o600},
		"README.md":                {scriptPortableReadme(options.Version, options.TargetOS), 0o600},
		"capabilities.json":        {capabilitiesBody, 0o600},
		"portable-provenance.json": {provenanceBody, 0o600},
		"maestro-os/CLAUDE.md":     {scriptPortableClaude(options.TargetOS), 0o600},
		"maestro-os/README.md":     {[]byte("# Maestro OS\n\nAbra esta pasta no Claude Code e diga que quer comecar.\n"), 0o600},
	}
	if options.TargetOS == "macos" {
		common["install.sh"] = struct {
			body []byte
			mode os.FileMode
		}{scriptPortablePOSIXInstaller(options.Version), 0o700}
		common["Start Maestro.command"] = struct {
			body []byte
			mode os.FileMode
		}{scriptPortablePOSIXStart(), 0o700}
		common["maestro-hook.sh"] = struct {
			body []byte
			mode os.FileMode
		}{scriptPortablePOSIXHook(), 0o700}
		common["projection/settings.local.json"] = struct {
			body []byte
			mode os.FileMode
		}{scriptPortableHookSettings("macos"), 0o600}
	} else {
		common["Install-Maestro.ps1"] = struct {
			body []byte
			mode os.FileMode
		}{scriptPortablePowerShellInstaller(options.Version), 0o600}
		common["Start Maestro.cmd"] = struct {
			body []byte
			mode os.FileMode
		}{scriptPortableCMDStart(), 0o600}
		common["Maestro-Hook.ps1"] = struct {
			body []byte
			mode os.FileMode
		}{scriptPortablePowerShellHook(), 0o600}
		common["projection/settings.local.json"] = struct {
			body []byte
			mode os.FileMode
		}{scriptPortableHookSettings("windows"), 0o600}
	}
	for name, entry := range common {
		if err := copyRegularBytes(entry.body, filepath.Join(packageRoot, filepath.FromSlash(name)), entry.mode); err != nil {
			return ScriptPortableResult{}, err
		}
	}
	if err := writeScriptRuntimeInventory(packageRoot, options.TargetOS); err != nil {
		return ScriptPortableResult{}, err
	}
	if err := writeScriptInventory(packageRoot); err != nil {
		return ScriptPortableResult{}, err
	}
	if err := verifyScriptPortableTree(packageRoot); err != nil {
		return ScriptPortableResult{}, err
	}
	if err := writeDeterministicZip(staging, rootName, options.Output); err != nil {
		return ScriptPortableResult{}, err
	}
	_, digest, err := digestBoundedRegular(options.Output, 512<<20)
	if err != nil {
		return ScriptPortableResult{}, err
	}
	checksumBody := []byte(digest + "  " + filepath.Base(options.Output) + "\n")
	if err := writeExclusive(checksumPath, checksumBody, 0o600); err != nil {
		_ = os.Remove(options.Output)
		return ScriptPortableResult{}, err
	}
	if err := writeExclusive(provenancePath, provenanceBody, 0o600); err != nil {
		_ = os.Remove(options.Output)
		_ = os.Remove(checksumPath)
		return ScriptPortableResult{}, err
	}
	return ScriptPortableResult{
		Output: options.Output, SHA256: digest, Status: provenance.Status,
		Provenance: provenancePath, Checksum: checksumPath,
	}, nil
}

func prepareScriptAgentProjection(packageRoot string) error {
	files, err := claudeagents.ProjectionFiles()
	if err != nil {
		return fmt.Errorf("render script agent projection: %w", err)
	}
	for name, body := range files {
		if err := copyRegularBytes(body, filepath.Join(packageRoot, "projection", "agents", name), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func pruneScriptProjectionSources(packageRoot string) error {
	for _, relative := range []string{
		filepath.Join("payload", "agents"),
		filepath.Join("payload", "skills"),
		filepath.Join("payload", "bundles", "tech-core", "skills"),
	} {
		if err := os.RemoveAll(filepath.Join(packageRoot, relative)); err != nil {
			return fmt.Errorf("remove redundant script projection source %s: %w", relative, err)
		}
	}
	return nil
}

func writeScriptRuntimeInventory(root, targetOS string) error {
	installer := "install.sh"
	hook := "maestro-hook.sh"
	if targetOS == "windows" {
		installer = "Install-Maestro.ps1"
		hook = "Maestro-Hook.ps1"
	}
	names := []string{"capabilities.json", installer, hook}
	for _, directory := range []string{"payload", "projection"} {
		base := filepath.Join(root, directory)
		if err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == base || entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("script runtime inventory encountered an unsafe entry: %s", path)
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(relative))
			return nil
		}); err != nil {
			return err
		}
	}
	sort.Strings(names)
	var output strings.Builder
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		output.WriteString(SHA256(body) + "  " + name + "\n")
	}
	return copyRegularBytes([]byte(output.String()), filepath.Join(root, "runtime-inventory.sha256"), 0o600)
}

func prepareScriptSkillProjection(packageRoot string) ([]string, []string, error) {
	projectionSkills := filepath.Join(packageRoot, "projection", "skills")
	skillRoots := []string{
		filepath.Join(packageRoot, "payload", "skills"),
		filepath.Join(packageRoot, "payload", "bundles", "tech-core", "skills"),
	}
	overlays := map[string][]byte{
		"maestro-onboarding":        scriptOnlyOnboardingSkill(),
		"bcgos-operator":            scriptOnlyOperatorSkill(),
		"maestro-setup-update":      scriptOnlyUpdateSkill(),
		"execution-continuity":      scriptOnlyContinuitySkill(),
		"interaction-profile":       scriptOnlyInteractionProfileSkill(),
		"agent-identity-setup":      scriptOnlyAgentIdentitySkill(),
		"maestro-environment-setup": scriptOnlyCheckupSkill("maestro-environment-setup", "Prepare o workspace do perfil script-only e confirme sua projecao de arquivos."),
		"maestro-runtime-checkup":   scriptOnlyCheckupSkill("maestro-runtime-checkup", "Inspecione o estado do perfil script-only e os hooks de texto sem procurar CLI nativo."),
	}
	nativeOnlyByContract := map[string]bool{
		"dream-memory":    true,
		"find-prior-work": true,
		"ingest-content":  true,
		"retro":           true,
	}
	var projected, nativeOnly []string
	seen := map[string]bool{}
	for _, skillRoot := range skillRoots {
		entries, err := os.ReadDir(skillRoot)
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if seen[entry.Name()] {
				return nil, nil, fmt.Errorf("duplicate script skill identity: %s", entry.Name())
			}
			seen[entry.Name()] = true
			skillFile := filepath.Join(skillRoot, entry.Name(), "SKILL.md")
			if _, err := os.ReadFile(skillFile); err != nil {
				return nil, nil, fmt.Errorf("read script skill %s: %w", entry.Name(), err)
			}
			if nativeOnlyByContract[entry.Name()] {
				nativeOnly = append(nativeOnly, entry.Name())
				continue
			}
			if overlay, ok := overlays[entry.Name()]; ok {
				if err := copyRegularBytes(overlay, filepath.Join(projectionSkills, entry.Name(), "SKILL.md"), 0o600); err != nil {
					return nil, nil, err
				}
				projected = append(projected, entry.Name())
				continue
			}
			if err := copyScriptDirectory(filepath.Join(skillRoot, entry.Name()), filepath.Join(projectionSkills, entry.Name())); err != nil {
				return nil, nil, err
			}
			projected = append(projected, entry.Name())
		}
	}
	sort.Strings(projected)
	sort.Strings(nativeOnly)
	return projected, nativeOnly, nil
}

func copyScriptDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("script projection contains a symlink: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(destination, relative), 0o700)
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 8<<20 {
			return fmt.Errorf("script projection contains an unsafe file: %s", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return copyRegularBytes(body, filepath.Join(destination, relative), 0o600)
	})
}

func scriptPortableProfile(target string) (string, string) {
	switch target {
	case "macos":
		return "macos-shell-local-beta", "posix_sh"
	case "windows":
		return "windows-powershell-local-beta", "windows_powershell"
	default:
		return "", ""
	}
}

func projectScriptPayload(root, packageRoot string, allowlist Allowlist) error {
	for _, entry := range allowlist.Files {
		source := filepath.Join(root, filepath.FromSlash(entry.Source))
		if symlinked, err := hasSymlinkComponent(root, entry.Source); err != nil {
			return err
		} else if symlinked {
			return fmt.Errorf("script payload source is symlinked: %s", entry.Source)
		}
		info, err := os.Lstat(source)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 {
			return fmt.Errorf("script payload source is not a bounded regular file: %s", entry.Source)
		}
		body, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if forbiddenScriptPayload(entry.Path, body) {
			return fmt.Errorf("script payload contains executable or compiler content: %s", entry.Path)
		}
		if err := copyRegularBytes(body, filepath.Join(packageRoot, "payload", filepath.FromSlash(entry.Path)), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func forbiddenScriptPayload(name string, body []byte) bool {
	lower := strings.ToLower(name)
	allowedText := false
	for _, suffix := range []string{".md", ".json", ".yaml", ".yml", ".tmpl", ".txt", ".sha256", ".sh", ".command", ".ps1", ".cmd"} {
		if strings.HasSuffix(lower, suffix) {
			allowedText = true
			break
		}
	}
	if !allowedText || bytes.IndexByte(body, 0) >= 0 {
		return true
	}
	return bytes.HasPrefix(body, []byte("MZ")) || bytes.HasPrefix(body, []byte{0x7f, 'E', 'L', 'F'}) ||
		bytes.HasPrefix(body, []byte{0xcf, 0xfa, 0xed, 0xfe}) || bytes.HasPrefix(body, []byte{0xfe, 0xed, 0xfa, 0xcf}) ||
		bytes.HasPrefix(body, []byte{0xce, 0xfa, 0xed, 0xfe}) || bytes.HasPrefix(body, []byte{0xfe, 0xed, 0xfa, 0xce}) ||
		bytes.HasPrefix(body, []byte{0xca, 0xfe, 0xba, 0xbe}) || bytes.HasPrefix(body, []byte{0xbe, 0xba, 0xfe, 0xca}) ||
		bytes.HasPrefix(body, []byte{0xca, 0xfe, 0xba, 0xbf}) || bytes.HasPrefix(body, []byte{0xbf, 0xba, 0xfe, 0xca})
}

func writeScriptInventory(root string) error {
	var names []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() || filepath.Base(path) == "inventory.sha256" {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("script portable inventory encountered an unsafe entry")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(names)
	var output strings.Builder
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		output.WriteString(SHA256(body) + "  " + name + "\n")
	}
	return copyRegularBytes([]byte(output.String()), filepath.Join(root, "inventory.sha256"), 0o600)
}

func verifyScriptPortableTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("script portable contains unsafe entry: %s", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		if forbiddenScriptPayload(relative, body) {
			return fmt.Errorf("script portable contains forbidden executable content: %s", relative)
		}
		return nil
	})
}

func scriptPortableStartHere(target string) []byte {
	launcher := "Start Maestro.command"
	if target == "windows" {
		launcher = "Start Maestro.cmd"
	}
	return []byte("MAESTRO SEM GO\n\nCAMINHO RAPIDO\n\n1. Extraia a pasta completa.\n2. De duplo clique em " + launcher + ".\n3. Pressione S para confirmar.\n4. Abra no Claude Code a pasta maestro-os revelada pelo Finder/Explorer e diga: Quero comecar.\n\nSe o duplo clique nao abrir ou for bloqueado pela empresa, nao tente contornar: abra a pasta maestro-os deste pacote no Claude Code, diga Quero comecar e confirme. O proprio Claude Code pode mostrar uma permissao de ferramenta; confirme somente se o comando apontar para este pacote. Ao terminar, abra a pasta permanente mostrada. Depois disso, o ZIP pode ser apagado. Se os dois caminhos forem bloqueados, envie a mensagem de erro para quem forneceu o pacote. O instalador nao altera politicas de seguranca. Plataforma: " + target + ".\n")
}

func scriptPortableReadme(version, target string) []byte {
	return []byte("# Maestro script-only " + version + "\n\nEste pacote usa somente scripts de texto e conteudo gerenciado; Go, compilador e executaveis Maestro nao sao necessarios. Use o arquivo Start Maestro da sua plataforma; ele prepara e revela a pasta permanente `maestro-os`, que deve ser aberta no Claude Code.\n\nEste perfil preserva os sete hooks do Claude por handlers shell/PowerShell, cinco especialistas Claude operacionais e uma garantia reduzida de sequencia entre eles, skills file-driven, orientacao, onboarding proprio, atlas, continuidade revisada e perfil profissional consentido entre sessoes, alem de update/rollback de conteudo. O `doctor` comprova que essa projecao esta integra no disco; a primeira sessao na pasta permanente ainda e necessaria para o Claude carregar os hooks. Autoridades que dependem do CLI nativo estao marcadas como indisponiveis em `capabilities.json`. O pacote nao remove quarentena/MOTW nem muda Gatekeeper, PowerShell ou AppLocker.\n\nOs scripts, skills e conteudo deste ZIP sao legiveis pelo destinatario. O repositorio e a implementacao Go nao sao enviados, mas esta nao e uma distribuicao com zero logica de produto exposta. Plataforma: " + target + ".\n")
}

func scriptPortableClaude(target string) []byte {
	command := `/bin/sh ../install.sh install`
	if target == "windows" {
		command = `powershell.exe -NoLogo -NoProfile -File ..\Install-Maestro.ps1 -Action install`
	}
	return []byte(`# Maestro script-only - primeiro uso

Conduza a pessoa em portugues simples. Este pacote nao usa Go nem executaveis Maestro.

1. Leia ../capabilities.json antes de descrever capacidades. Nunca afirme que itens em unavailable estao ativos.
2. Se .maestro-script/active-version existir, continue o pedido normal ou use maestro-onboarding quando ainda incompleto.
3. Caso contrario, explique que voce pode instalar a projecao gerenciada nesta pasta e peca uma confirmacao curta: "Posso preparar o Maestro agora?"
4. Somente apos resposta afirmativa, execute internamente: ` + command + `
5. Nao instrua a pessoa a digitar comandos, instalar Go, elevar privilegios ou mudar politicas de seguranca.
6. Depois do sucesso, leia o caminho exibido por MAESTRO-SCRIPT-WORKSPACE. Explique que a pasta permanente foi criada e peça que a pessoa a abra uma vez no Claude Code; os hooks e agentes só são garantidos em uma nova sessão nessa pasta. Não continue onboarding dentro da pasta temporária do ZIP.
7. Se a politica bloquear o script, pare. Confirme que nenhum trabalho foi apagado e oriente a procurar o responsavel pelo pacote.
`)
}

func scriptOnlyOnboardingSkill() []byte {
	return []byte(strings.ReplaceAll(`---
name: maestro-onboarding
description: Conduza o onboarding conversacional do perfil Maestro script-only, sem CLI nativo e sem afirmar persistencia governada.
---

# Maestro Onboarding — script-only

Leia @@BT@@.maestro-script/capabilities.json@@BT@@ antes de começar. Este perfil tem
hooks de texto do Claude, mas não tem o CLI nativo, cofre de credenciais,
recibos de hook autenticados nem ledger determinístico. Nunca tente
chamar @@BT@@bcgos@@BT@@, nunca simule recibos e nunca diga que uma capacidade marcada
como indisponível está ativa.

Conduza a conversa em português brasileiro, uma pergunta por vez:

1. explique em uma frase que este é um baseline profissional local e revisável;
2. ofereça uma trilha curta (nome, papel, comunicação, forma de trabalhar e
   qualidade) ou completa (acrescenta motivações, decisões, limites e voz);
3. reflita cada resposta de forma breve e peça correção antes de prosseguir;
4. não peça nomes de clientes, credenciais, dados pessoais desnecessários nem
   material confidencial;
5. ao final, mostre uma síntese completa e pergunte separadamente se o owner
   quer salvá-la em @@BT@@.maestro-script/local-profile.md@@BT@@ neste workspace;
6. só escreva após aprovação explícita da síntese e do destino. Preserve o
   arquivo em updates e diga claramente que ele é local, não um registro
   autenticado pelo core nativo;
7. depois de salvar, pergunte separadamente se o owner autoriza usar um ponteiro
   para esse perfil em novas sessões e explique que o hook mostra apenas nível,
   ponteiro e revisão, mas o Claude poderá abrir somente os trechos relevantes do
   Markdown quando precisar adaptar a interação. Se sim, siga
   @@BT@@interaction-profile@@BT@@ para criar o estado bounded; se não, remova um
   @@BT@@session-profile.json@@BT@@ anterior, se existir, sem apagar
   @@BT@@local-profile.md@@BT@@.

Se a pessoa não quiser persistir, continue usando apenas o contexto da conversa.
`, "@@BT@@", "`"))
}

func scriptOnlyAgentIdentitySkill() []byte {
	return []byte(strings.ReplaceAll(`---
name: agent-identity-setup
description: Personalize os nomes e avatares dos agentes Claude operacionais do perfil script-only com revisão e confirmação explícitas.
---

# Agent identity — script-only

Este perfil já projeta Client Account Agent, Case Agent, Walter, Darwin e PA
Expert como agentes Claude operacionais. A personalização muda somente a forma
como Maestro os apresenta; nunca muda tools, escopo, permissões ou autoridade.

1. Leia @@BT@@.maestro-script/capabilities.json@@BT@@ e o perfil existente em
   @@BT@@.maestro-script/agent-profile.md@@BT@@, se houver.
2. Pergunte uma escolha por vez: nome de exibição e um emoji para Maestro,
   Walter e Darwin; ofereça depois Client Account e Case quando existirem.
3. Não infira personalidade, nome ou emoji de cargo, cliente, histórico ou
   material de trabalho. Walter, Darwin e os defaults continuam válidos.
4. Mostre a proposta consolidada com os papéis canônicos imutáveis e peça uma
   confirmação explícita separada.
5. Somente após confirmação, escreva um Markdown curto e legível em
   @@BT@@.maestro-script/agent-profile.md@@BT@@. Preserve esse arquivo em update e
   rollback. Nunca edite os arquivos gerenciados em @@BT@@.claude/agents@@BT@@.

O arquivo é owner-local e revisável, não um recibo autenticado. Não inclua
client names, prompts, transcrições, credenciais ou material confidencial.
`, "@@BT@@", "`"))
}

func scriptOnlyOperatorSkill() []byte {
	return []byte(strings.ReplaceAll(`---
name: bcgos-operator
description: Inspecione o perfil Maestro script-only e opere status, doctor ou rollback pelo script instalado, sem CLI nativo.
---

# Maestro script-only operator

Leia @@BT@@.maestro-script/capabilities.json@@BT@@ e o bloco gerenciado de @@BT@@CLAUDE.md@@BT@@.
Este perfil não inclui o executável @@BT@@bcgos@@BT@@. Nunca procure, baixe ou reconstrua
esse binário.

- Para status ou doctor, invoque internamente o script instalado apontado no
  bloco gerenciado, usando a ação correspondente.
- Para rollback, explique a versão que será reprojetada e obtenha confirmação
  antes de chamar a ação @@BT@@rollback@@BT@@.
- Para update, peça que a pessoa extraia o novo ZIP oficial e abra seu seed
  @@BT@@maestro-os@@BT@@. O instalador preserva e mostra o workspace permanente;
  depois do sucesso, peça que a pessoa volte a essa pasta numa nova sessão.
- Traduza erros para linguagem simples, confirme que arquivos locais não foram
  apagados e não recomende mudanças em quarentena ou políticas de segurança.
`, "@@BT@@", "`"))
}

func scriptOnlyUpdateSkill() []byte {
	return []byte(strings.ReplaceAll(`---
name: maestro-setup-update
description: Atualize ou reverta somente o conteúdo do perfil Maestro script-only, usando pacotes completos e inventariados.
---

# Maestro script-only update

Este fluxo atualiza apenas conteúdo e projeções gerenciadas. Não há update de
binário, assinatura de release ou download automático.

1. Leia @@BT@@.maestro-script/capabilities.json@@BT@@ e o status pelo script instalado.
2. Para instalar uma versão nova, use somente um novo ZIP completo recebido
   pelo canal oficial; abra o seed @@BT@@maestro-os@@BT@@ dele e execute seu preparo após
   uma confirmação curta. O pacote valida seu inventário antes de mudar o
   runtime e mostra o workspace permanente ao final. Reabra essa pasta, não
   continue na pasta temporária da nova versão.
3. Para rollback, informe a versão anterior e peça confirmação; então invoque
   @@BT@@rollback@@BT@@ no script instalado.
4. Nunca use @@BT@@ExecutionPolicy Bypass@@BT@@, remova quarentena/MOTW, eleve privilégios,
   baixe ferramentas ou afirme que capacidades nativas foram atualizadas.
`, "@@BT@@", "`"))
}

func scriptOnlyContinuitySkill() []byte {
	return []byte(strings.ReplaceAll(`---
name: execution-continuity
description: Registre, pause e retome trabalho profissional revisado entre sessões no perfil script-only.
---

# Execution continuity — continuity-lite-v1

Este perfil preserva continuidade por artefatos legíveis, sem simular o
Execution Ledger nativo. O corpo do trabalho fica em Markdown revisado sob
@@BT@@brain/tasks/@@BT@@; o hook SessionStart recebe somente um ponteiro lógico bounded.

## Registrar ou atualizar o handoff

1. Confirme o objetivo, o próximo passo e um critério de conclusão pequeno.
2. Escreva ou atualize exatamente um arquivo regular
   @@BT@@brain/tasks/<nome-seguro>.md@@BT@@. Use apenas letras, números, ponto,
   sublinhado e hífen no nome. Mantenha objetivo, próximo passo, owner, critério
   e referências lógicas; nunca grave prompt, transcript, credencial, caminho
   absoluto ou corpo de cliente.
3. Para pausar, acrescente um @@BT@@## Checkpoint@@BT@@ curto e revisado com resumo,
   próximo passo e blocker. Não sintetize checkpoint do histórico da conversa.
4. Depois que a pessoa revisar o artefato, escreva atomicamente
   @@BT@@.maestro-script/continuity-state.json@@BT@@ com no máximo 2 KiB e exatamente:

@@BT@@@@BT@@@@BT@@json
{"schema_version":1,"state":"active","revision":1,"task":"brain/tasks/<nome-seguro>.md","checkpoint_present":false}
@@BT@@@@BT@@@@BT@@

   Use @@BT@@active@@BT@@ enquanto o trabalho segue e @@BT@@paused@@BT@@ quando houver
   checkpoint. Incremente @@BT@@revision@@BT@@ a cada handoff revisado. Grave em um
   temporário na mesma pasta e renomeie; não siga links.

## Retomar

SessionStart pode informar apenas estado, ponteiro e presença de checkpoint.
Leia o Markdown apontado somente quando o owner quiser continuar, confirme o
escopo e o próximo passo e então prossiga. Um estado inválido requer reparo; não
invente a tarefa. Update e rollback preservam esse arquivo owner-local.

Isto é continuidade degradada e revisável. Não alegue CAS, attempt fencing,
recibo autenticado, completion proof, memória automática ou ledger nativo.
`, "@@BT@@", "`"))
}

func scriptOnlyInteractionProfileSkill() []byte {
	return []byte(strings.ReplaceAll(`---
name: interaction-profile
description: Ajuste linguagem, ritmo e nivel de detalhe no perfil script-only sem criar autoridade ou inferir preferencias privadas.
---

# Interaction profile — script-only

Use primeiro preferências explícitas da conversa atual. Se existir e a pessoa
tiver consentido em salvá-lo, leia apenas o trecho relevante de
@@BT@@.maestro-script/local-profile.md@@BT@@. Na ausência de preferência, use português
brasileiro claro, detalhe moderado e uma pergunta por vez.

Para reutilização em novas sessões, obtenha uma segunda confirmação explícita.
Mostre o nível proposto — @@BT@@standard@@BT@@, @@BT@@advanced@@BT@@ ou
@@BT@@power@@BT@@ — e explique duas coisas separadamente: o hook verá somente nível,
ponteiro e revisão, nunca o corpo; depois disso o Claude poderá abrir somente o
trecho relevante do Markdown quando precisar adaptar a interação. Após a
confirmação:

1. confirme que o perfil é arquivo regular, não link e tem no máximo 1 MiB;
2. calcule seu SHA-256 localmente;
3. escreva primeiro um temporário e depois renomeie atomicamente para
   @@BT@@.maestro-script/session-profile.json@@BT@@, com no máximo 2 KiB:
   @@BT@@{"schema_version":1,"interaction_profile":"standard|advanced|power","revision":1,"local_profile":".maestro-script/local-profile.md","profile_sha256":"<sha256>","session_use_confirmed":true}@@BT@@;
4. aumente a revisão positiva em cada alteração revisada.

Não grave corpo, nome, cliente, caminho absoluto ou digest em output de hook.
Estado ausente significa @@BT@@standard@@BT@@; estado inválido requer reparo e
também cai para @@BT@@standard@@BT@@.

Para revogar o uso entre sessões, confirme a revogação e remova somente
@@BT@@.maestro-script/session-profile.json@@BT@@. Preserve
@@BT@@.maestro-script/local-profile.md@@BT@@ para revisão ou uso manual, a menos que
o owner peça separadamente para apagá-lo.

Esse perfil só muda apresentação. Ele nunca concede acesso, dispensa revisão,
ativa ferramentas, substitui confirmação ou prova estado do core nativo. Não
crie arquivos de perfil sem mostrar o texto e obter consentimento explícito.
`, "@@BT@@", "`"))
}

func scriptOnlyCheckupSkill(name, description string) []byte {
	template := `---
name: @@NAME@@
description: @@DESCRIPTION@@
---

# Maestro workspace — script-only

Leia @@BT@@.maestro-script/capabilities.json@@BT@@. Confirme que o bloco
@@BT@@MAESTRO SCRIPT MANAGED@@BT@@ existe em @@BT@@CLAUDE.md@@BT@@ e que as skills
gerenciadas estão sob @@BT@@.claude/skills@@BT@@. Para uma verificação adicional,
use internamente a ação @@BT@@doctor@@BT@@ do script instalado apontado naquele
bloco.

Se esses arquivos estiverem presentes, diga que a projeção de conteúdo está
pronta. Confirme os sete bindings em @@BT@@.claude/settings.local.json@@BT@@ e o
handler em @@BT@@.maestro-script/hooks@@BT@@, mas não procure scheduler,
executável ou recibos autenticados do core nativo. Uma falha deve preservar os
arquivos locais e encaminhar para o novo ZIP oficial ou para quem forneceu o
pacote, sem mudar políticas de segurança.
`
	template = strings.ReplaceAll(template, "@@NAME@@", name)
	template = strings.ReplaceAll(template, "@@DESCRIPTION@@", description)
	return []byte(strings.ReplaceAll(template, "@@BT@@", "`"))
}

func scriptPortablePOSIXStart() []byte {
	return []byte(`#!/bin/sh
set -u
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
printf '%s\n' 'O Maestro instalara somente scripts e conteudo gerenciado no seu perfil, sem Go ou administrador.'
printf 'Continuar? [s/N] '
IFS= read -r answer
case "$answer" in s|S|sim|SIM|Sim) ;; *) printf '%s\n' 'Preparacao cancelada; nada foi alterado.'; exit 0;; esac
if /bin/sh "$ROOT/install.sh" install --reveal-workspace; then
  printf '%s\n' 'Maestro preparado. O Finder mostrou a pasta permanente; abra maestro-os no Claude Code e diga: Quero comecar.'
else
  status=$?
  printf '%s\n' 'Nao foi possivel preparar o Maestro. Nada do seu trabalho foi apagado.'
  exit "$status"
fi
`)
}

func scriptPortableHookSettings(target string) []byte {
	prefix := `/bin/sh "$CLAUDE_PROJECT_DIR/.maestro-script/hooks/maestro-hook.sh"`
	if target == "windows" {
		prefix = `powershell.exe -NoLogo -NoProfile -NonInteractive -File "%CLAUDE_PROJECT_DIR%\.maestro-script\hooks\Maestro-Hook.ps1" -Event`
	}
	events := []struct {
		name   string
		action string
		async  bool
	}{
		{name: "SessionStart", action: "session-start"},
		{name: "UserPromptSubmit", action: "context-injection"},
		{name: "PreToolUse", action: "pre-action-guard"},
		{name: "PostToolUse", action: "post-action-receipt", async: true},
		{name: "Stop", action: "stop-finalization"},
		{name: "SubagentStart", action: "subagent-start"},
		{name: "SubagentStop", action: "subagent-stop"},
	}
	hooks := map[string]any{}
	for _, event := range events {
		command := prefix + " " + event.action
		if target == "macos" {
			command += ` "$CLAUDE_PROJECT_DIR"`
		} else {
			command += ` -Workspace "%CLAUDE_PROJECT_DIR%"`
		}
		entry := map[string]any{"type": "command", "command": command, "timeout": 10}
		if event.async {
			entry["async"] = true
		}
		hooks[event.name] = []any{map[string]any{"hooks": []any{entry}}}
	}
	body, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(body, '\n')
}

func scriptPortablePOSIXHook() []byte {
	return []byte(`#!/bin/sh
set -eu
umask 077
EVENT=${1:-}
WORKSPACE=${2:-${CLAUDE_PROJECT_DIR:-}}

deny() {
  printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Maestro script hook could not verify this action safely. Nothing was changed. Review the target and retry with a narrower action."}}'
}

ask_external() {
  printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"This appears to publish or mutate an external system. Ask the owner for confirmation before continuing. This is Claude permission handling, not a native Maestro challenge receipt."}}'
}

record_event() {
  [ -n "$WORKSPACE" ] && [ -d "$WORKSPACE/.maestro-script" ] || return 0
  log="$WORKSPACE/.maestro-script/hook-events.jsonl"
  [ ! -L "$WORKSPACE/.maestro-script" ] && [ ! -L "$log" ] || return 0
  printf '{"schema_version":1,"profile":"script-only","event":"%s","state":"observed"}\n' "$1" >> "$log"
  size=$(wc -c < "$log" | tr -d ' ')
  if [ "$size" -gt 65536 ]; then
    tail -n 200 "$log" > "$log.tmp.$$" && mv "$log.tmp.$$" "$log"
  fi
}

continuity_context() (
  state_root="$WORKSPACE/.maestro-script"
  continuity="$state_root/continuity-state.json"
  [ -e "$continuity" ] || return 0
  invalid='CONTINUITY LITE REPAIR REQUIRED: the owner-local pointer is invalid; use the execution-continuity skill without inventing a task.'
  [ -d "$state_root" ] && [ ! -L "$state_root" ] || { printf '%s' "$invalid"; return 0; }
  [ -f "$continuity" ] && [ ! -L "$continuity" ] || { printf '%s' "$invalid"; return 0; }
  bytes=$(wc -c < "$continuity" | tr -d ' ')
  [ "$bytes" -gt 0 ] && [ "$bytes" -le 2048 ] && command -v plutil >/dev/null 2>&1 && plutil -convert xml1 -o /dev/null "$continuity" >/dev/null 2>&1 || { printf '%s' "$invalid"; return 0; }
  state_xml=$(mktemp "${TMPDIR:-/tmp}/maestro-continuity.XXXXXX") || { printf '%s' "$invalid"; return 0; }
  trap 'rm -f "$state_xml"' EXIT HUP INT TERM
  plutil -convert xml1 -o "$state_xml" "$continuity" >/dev/null 2>&1 || { printf '%s' "$invalid"; return 0; }
  plist_type_is() {
    awk -v key="$1" -v kind="$2" '
      index($0,"<key>" key "</key>") {
        count++
        if (getline <= 0) next
        value=$0
        sub(/^[[:space:]]*/,"",value); sub(/[[:space:]]*$/,"",value)
        if (kind=="integer" && value ~ /^<integer>[0-9]+<\/integer>$/) valid++
        if (kind=="string" && value ~ /^<string>.*<\/string>$/) valid++
        if (kind=="boolean" && (value=="<true/>" || value=="<false/>")) valid++
      }
      END { exit !(count==1 && valid==1) }
    ' "$state_xml"
  }
  plist_type_is schema_version integer &&
    plist_type_is state string &&
    plist_type_is revision integer &&
    plist_type_is task string &&
    plist_type_is checkpoint_present boolean || { printf '%s' "$invalid"; return 0; }
  schema=$(plutil -extract schema_version raw -o - "$continuity" 2>/dev/null || true)
  state=$(plutil -extract state raw -o - "$continuity" 2>/dev/null || true)
  revision=$(plutil -extract revision raw -o - "$continuity" 2>/dev/null || true)
  task=$(plutil -extract task raw -o - "$continuity" 2>/dev/null || true)
  checkpoint=$(plutil -extract checkpoint_present raw -o - "$continuity" 2>/dev/null || true)
  [ "$schema" = 1 ] || { printf '%s' "$invalid"; return 0; }
  case "$state" in active|paused) ;; *) printf '%s' "$invalid"; return 0;; esac
  case "$revision" in ''|0|*[!0-9]*) printf '%s' "$invalid"; return 0;; esac
  [ "${#revision}" -le 10 ] && awk -v value="$revision" 'BEGIN { exit !(value <= 2147483647) }' || { printf '%s' "$invalid"; return 0; }
  case "$task" in brain/tasks/*.md) ;; *) printf '%s' "$invalid"; return 0;; esac
  task_name=${task#brain/tasks/}
  case "$task_name" in ''|.*|*/*|*\\*|*..*|*[!A-Za-z0-9._-]*) printf '%s' "$invalid"; return 0;; esac
  case "$checkpoint" in true|false) ;; *) printf '%s' "$invalid"; return 0;; esac
  target="$WORKSPACE/$task"
  [ -f "$target" ] && [ ! -L "$target" ] || { printf '%s' "$invalid"; return 0; }
  target_bytes=$(wc -c < "$target" | tr -d ' ')
  [ "$target_bytes" -le 1048576 ] || { printf '%s' "$invalid"; return 0; }
  printf 'CONTINUITY LITE: state: %s; reviewed task pointer: %s; checkpoint present: %s; revision: %s. Do not inject the task body; read it only when the owner asks to continue.' "$state" "$task" "$checkpoint" "$revision"
)

session_profile_context() (
  state_root="$WORKSPACE/.maestro-script"
  state_file="$state_root/session-profile.json"
  [ -e "$state_file" ] || { printf 'SESSION PROFILE: interaction profile: standard; no reviewed session profile is registered.'; return 0; }
  invalid='SESSION PROFILE REPAIR REQUIRED: use interaction profile standard until the reviewed owner-local pointer is repaired.'
  [ -d "$state_root" ] && [ ! -L "$state_root" ] || { printf '%s' "$invalid"; return 0; }
  [ -f "$state_file" ] && [ ! -L "$state_file" ] || { printf '%s' "$invalid"; return 0; }
  bytes=$(wc -c < "$state_file" | tr -d ' ')
  [ "$bytes" -gt 0 ] && [ "$bytes" -le 2048 ] && command -v plutil >/dev/null 2>&1 && plutil -convert xml1 -o /dev/null "$state_file" >/dev/null 2>&1 || { printf '%s' "$invalid"; return 0; }
  state_xml=$(mktemp "${TMPDIR:-/tmp}/maestro-session-profile.XXXXXX") || { printf '%s' "$invalid"; return 0; }
  trap 'rm -f "$state_xml"' EXIT HUP INT TERM
  plutil -convert xml1 -o "$state_xml" "$state_file" >/dev/null 2>&1 || { printf '%s' "$invalid"; return 0; }
  plist_type_is() {
    awk -v key="$1" -v kind="$2" '
      index($0,"<key>" key "</key>") {
        count++
        if (getline <= 0) next
        value=$0
        sub(/^[[:space:]]*/,"",value); sub(/[[:space:]]*$/,"",value)
        if (kind=="integer" && value ~ /^<integer>[0-9]+<\/integer>$/) valid++
        if (kind=="string" && value ~ /^<string>.*<\/string>$/) valid++
        if (kind=="true" && value=="<true/>") valid++
      }
      END { exit !(count==1 && valid==1) }
    ' "$state_xml"
  }
  plist_type_is schema_version integer &&
    plist_type_is interaction_profile string &&
    plist_type_is revision integer &&
    plist_type_is local_profile string &&
    plist_type_is profile_sha256 string &&
    plist_type_is session_use_confirmed true || { printf '%s' "$invalid"; return 0; }
  schema=$(plutil -extract schema_version raw -o - "$state_file" 2>/dev/null || true)
  profile=$(plutil -extract interaction_profile raw -o - "$state_file" 2>/dev/null || true)
  revision=$(plutil -extract revision raw -o - "$state_file" 2>/dev/null || true)
  pointer=$(plutil -extract local_profile raw -o - "$state_file" 2>/dev/null || true)
  expected_digest=$(plutil -extract profile_sha256 raw -o - "$state_file" 2>/dev/null || true)
  confirmed=$(plutil -extract session_use_confirmed raw -o - "$state_file" 2>/dev/null || true)
  [ "$schema" = 1 ] || { printf '%s' "$invalid"; return 0; }
  case "$profile" in standard|advanced|power) ;; *) printf '%s' "$invalid"; return 0;; esac
  case "$revision" in ''|0|*[!0-9]*) printf '%s' "$invalid"; return 0;; esac
  [ "${#revision}" -le 10 ] && awk -v value="$revision" 'BEGIN { exit !(value <= 2147483647) }' || { printf '%s' "$invalid"; return 0; }
  [ "$pointer" = '.maestro-script/local-profile.md' ] && [ "$confirmed" = true ] || { printf '%s' "$invalid"; return 0; }
  case "$expected_digest" in *[!a-f0-9]*|'') printf '%s' "$invalid"; return 0;; esac
  [ "${#expected_digest}" -eq 64 ] || { printf '%s' "$invalid"; return 0; }
  target="$WORKSPACE/$pointer"
  [ -f "$target" ] && [ ! -L "$target" ] || { printf '%s' "$invalid"; return 0; }
  profile_bytes=$(wc -c < "$target" | tr -d ' ')
  [ "$profile_bytes" -le 1048576 ] || { printf '%s' "$invalid"; return 0; }
  actual_digest=$(shasum -a 256 "$target" | awk '{print $1}')
  [ "$actual_digest" = "$expected_digest" ] || { printf '%s' "$invalid"; return 0; }
  printf 'SESSION PROFILE: interaction profile: %s; reviewed profile pointer: %s; revision: %s. Never inject the profile body; read only the relevant section when needed.' "$profile" "$pointer" "$revision"
)

agent_route_lite() (
  mode=$1
  state_root="$WORKSPACE/.maestro-script"
  route_root="$state_root/agent-route-lite"
  command -v /usr/bin/head >/dev/null 2>&1 && command -v /usr/bin/base64 >/dev/null 2>&1 || return 2
  route_capture=$(/usr/bin/head -c 65537 | /usr/bin/base64; printf '\001') || return 2
  route_payload_base64=${route_capture%?}
  route_input() { printf '%s' "$route_payload_base64" | /usr/bin/base64 -D; }
  payload_bytes=$(route_input | wc -c | tr -d ' ')
  [ "$payload_bytes" -gt 0 ] && [ "$payload_bytes" -le 65536 ] || return 2
  state_xml=''
  route_lock=''
  route_lock_owned=0
  trap '[ -z "$state_xml" ] || rm -f "$state_xml"; [ "$route_lock_owned" -eq 0 ] || rmdir "$route_lock" 2>/dev/null || true' EXIT HUP INT TERM
  command -v plutil >/dev/null 2>&1 || return 2
  json_key_xml() { route_input | plutil -extract "$1" xml1 -o - - 2>/dev/null; }
  json_key_raw() { route_input | plutil -extract "$1" raw -o - - 2>/dev/null; }
  json_key_has_type() {
    key=$1 kind=$2
    value=$(json_key_xml "$key") || return 1
    case "$kind" in
      integer) printf '%s\n' "$value" | grep -Eq '^[[:space:]]*<integer>[0-9]+</integer>[[:space:]]*$' ;;
      string) printf '%s\n' "$value" | grep -Eq '^[[:space:]]*<string>.*</string>[[:space:]]*$' ;;
      true) printf '%s\n' "$value" | grep -Eq '^[[:space:]]*<true/>[[:space:]]*$' ;;
      false) printf '%s\n' "$value" | grep -Eq '^[[:space:]]*<false/>[[:space:]]*$' ;;
      *) return 1 ;;
    esac
  }
  state_xml_key_has_type() {
    xml_file=$1 key=$2 kind=$3
    awk -v key="$key" -v kind="$kind" '
      index($0,"<key>" key "</key>") { found=1; next }
      found {
        value=$0; sub(/^[[:space:]]*/,"",value); sub(/[[:space:]]*$/,"",value)
        if (kind=="integer" && value ~ /^<integer>[0-9]+<\/integer>$/) exit 0
        if (kind=="string" && value ~ /^<string>.*<\/string>$/) exit 0
        exit 1
      }
      END { if (!found) exit 1 }
    ' "$xml_file"
  }
  json_key_present() { json_key_xml "$1" >/dev/null 2>&1; }
  json_key_has_type session_id string || return 2
  session_id=$(json_key_raw session_id || true)
  [ -n "$session_id" ] && [ "${#session_id}" -le 256 ] || return 2
  session_digest=$(printf '%s' "$session_id" | shasum -a 256 | awk '{print $1}')
  [ -d "$state_root" ] && [ ! -L "$state_root" ] || { echo 'MAESTRO-ROUTE-LITE: state root is unsafe' >&2; return 1; }
  if [ -e "$route_root" ]; then
    [ -d "$route_root" ] && [ ! -L "$route_root" ] || { echo 'MAESTRO-ROUTE-LITE: route state is unsafe' >&2; return 1; }
  else
    mkdir "$route_root" || return 1
  fi
  state_file="$route_root/$session_digest.json"
  route_lock="$state_file.lock"
  mkdir "$route_lock" 2>/dev/null || { echo 'MAESTRO-ROUTE-LITE: route state is busy; retry once' >&2; return 1; }
  route_lock_owned=1

  stage=idle
  transition_count=0
  active_agent_digest=''
  active_agent_type=''
  last_event=''
  last_agent_digest=''
  last_agent_type=''
  if [ -e "$state_file" ]; then
    [ -f "$state_file" ] && [ ! -L "$state_file" ] || { echo 'MAESTRO-ROUTE-LITE: route state is unsafe' >&2; return 1; }
    state_bytes=$(wc -c < "$state_file" | tr -d ' ')
    state_xml=$(mktemp "${TMPDIR:-/tmp}/maestro-route-state.XXXXXX") || return 1
    [ "$state_bytes" -gt 0 ] && [ "$state_bytes" -le 2048 ] && plutil -convert xml1 -o "$state_xml" "$state_file" >/dev/null 2>&1 || { echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1; }
    state_xml_key_has_type "$state_xml" schema_version integer &&
      state_xml_key_has_type "$state_xml" session_digest string &&
      state_xml_key_has_type "$state_xml" stage string &&
      state_xml_key_has_type "$state_xml" transition_count integer &&
      state_xml_key_has_type "$state_xml" active_agent_digest string &&
      state_xml_key_has_type "$state_xml" active_agent_type string &&
      state_xml_key_has_type "$state_xml" last_event string &&
      state_xml_key_has_type "$state_xml" last_agent_digest string &&
      state_xml_key_has_type "$state_xml" last_agent_type string || { echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1; }
    schema=$(plutil -extract schema_version raw -o - "$state_file" 2>/dev/null || true)
    stored_session=$(plutil -extract session_digest raw -o - "$state_file" 2>/dev/null || true)
    stage=$(plutil -extract stage raw -o - "$state_file" 2>/dev/null || true)
    transition_count=$(plutil -extract transition_count raw -o - "$state_file" 2>/dev/null || true)
    active_agent_digest=$(plutil -extract active_agent_digest raw -o - "$state_file" 2>/dev/null || true)
    active_agent_type=$(plutil -extract active_agent_type raw -o - "$state_file" 2>/dev/null || true)
    last_event=$(plutil -extract last_event raw -o - "$state_file" 2>/dev/null || true)
    last_agent_digest=$(plutil -extract last_agent_digest raw -o - "$state_file" 2>/dev/null || true)
    last_agent_type=$(plutil -extract last_agent_type raw -o - "$state_file" 2>/dev/null || true)
    [ "$schema" = 1 ] && [ "$stored_session" = "$session_digest" ] || { echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1; }
    case "$stage" in idle|account_active|account_framed|case_active_direct|case_active_strategic|case_complete_direct|case_complete_strategic|account_validation_active|complete|darwin_active|darwin_complete|pa_active|pa_complete|walter_active|walter_complete) ;; *) echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1;; esac
    case "$transition_count" in ''|*[!0-9]*) echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1;; esac
    [ "$transition_count" -le 32 ] || { echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1; }
    for digest_value in "$active_agent_digest" "$last_agent_digest"; do
      case "$digest_value" in '') continue ;; *[!a-f0-9]*) echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1;; esac
      [ "${#digest_value}" -eq 64 ] || { echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1; }
    done
    for type_value in "$active_agent_type" "$last_agent_type"; do case "$type_value" in ''|client-account-agent|case-agent|walter|darwin|pa-expert) ;; *) echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1;; esac; done
    case "$last_event" in ''|start|stop) ;; *) echo 'MAESTRO-ROUTE-LITE: route state requires repair' >&2; return 1;; esac
  fi

  save_state() {
    temporary="$route_root/.route-$session_digest-$$.tmp"
    printf '{"schema_version":1,"session_digest":"%s","stage":"%s","transition_count":%s,"active_agent_digest":"%s","active_agent_type":"%s","last_event":"%s","last_agent_digest":"%s","last_agent_type":"%s"}\n' \
      "$session_digest" "$stage" "$transition_count" "$active_agent_digest" "$active_agent_type" "$last_event" "$last_agent_digest" "$last_agent_type" > "$temporary"
    [ "$(wc -c < "$temporary" | tr -d ' ')" -le 2048 ] || { rm -f "$temporary"; return 1; }
    mv "$temporary" "$state_file"
  }

  case "$mode" in
    begin)
      case "$stage" in *_active) echo 'MAESTRO-ROUTE-LITE: cannot begin a new turn while a specialist is active' >&2; return 1;; esac
      stage=idle; transition_count=0; active_agent_digest=''; active_agent_type=''; last_event=''; last_agent_digest=''; last_agent_type=''; save_state
      ;;
    start|stop)
      json_key_has_type agent_id string && json_key_has_type agent_type string || { echo 'MAESTRO-ROUTE-LITE: managed agent identity is malformed' >&2; return 1; }
      agent_id=$(json_key_raw agent_id || true)
      agent_type=$(json_key_raw agent_type || true)
      [ -n "$agent_id" ] && [ "${#agent_id}" -le 256 ] || { echo 'MAESTRO-ROUTE-LITE: managed agent identity is missing' >&2; return 1; }
      case "$agent_type" in client-account-agent|case-agent|walter|darwin|pa-expert) ;; *) return 2;; esac
      agent_digest=$(printf '%s' "$agent_id" | shasum -a 256 | awk '{print $1}')
      if [ "$mode" = start ]; then
        if [ -n "$active_agent_digest" ]; then
          [ "$active_agent_digest" = "$agent_digest" ] && [ "$active_agent_type" = "$agent_type" ] && [ "$last_event" = start ] && return 0
          echo 'MAESTRO-ROUTE-LITE: one managed specialist is still active' >&2; return 1
        fi
        [ "$transition_count" -lt 32 ] || { echo 'MAESTRO-ROUTE-LITE: transition limit reached' >&2; return 1; }
        case "$stage:$agent_type" in
          idle:client-account-agent) stage=account_active ;;
          idle:case-agent) stage=case_active_direct ;;
          idle:darwin) stage=darwin_active ;;
          idle:pa-expert) stage=pa_active ;;
          idle:walter) stage=walter_active ;;
          account_framed:case-agent) stage=case_active_strategic ;;
          case_complete_strategic:client-account-agent) stage=account_validation_active ;;
          case_complete_direct:walter|complete:walter) stage=walter_active ;;
          darwin_complete:*|*:darwin) echo 'MAESTRO-ROUTE-LITE: Darwin system-health work cannot be mixed with client execution in the same turn' >&2; return 1 ;;
          *) echo 'MAESTRO-ROUTE-LITE: specialist order is invalid for the selected route' >&2; return 1 ;;
        esac
        active_agent_digest=$agent_digest; active_agent_type=$agent_type; last_event=start; last_agent_digest=$agent_digest; last_agent_type=$agent_type; transition_count=$((transition_count + 1)); save_state
      else
        if [ -z "$active_agent_digest" ] && [ "$last_event" = stop ] && [ "$last_agent_digest" = "$agent_digest" ] && [ "$last_agent_type" = "$agent_type" ]; then return 0; fi
        [ "$active_agent_digest" = "$agent_digest" ] && [ "$active_agent_type" = "$agent_type" ] || { echo 'MAESTRO-ROUTE-LITE: specialist stop does not match the active specialist' >&2; return 1; }
        case "$stage:$agent_type" in
          account_active:client-account-agent) stage=account_framed ;;
          case_active_direct:case-agent) stage=case_complete_direct ;;
          case_active_strategic:case-agent) stage=case_complete_strategic ;;
          account_validation_active:client-account-agent) stage=complete ;;
          darwin_active:darwin) stage=darwin_complete ;;
          pa_active:pa-expert) stage=pa_complete ;;
          walter_active:walter) stage=walter_complete ;;
          *) echo 'MAESTRO-ROUTE-LITE: specialist stop is invalid for the selected route' >&2; return 1 ;;
        esac
        active_agent_digest=''; active_agent_type=''; last_event=stop; last_agent_digest=$agent_digest; last_agent_type=$agent_type; transition_count=$((transition_count + 1)); save_state
      fi
      ;;
    finalize)
      if json_key_present stop_hook_active; then
        if json_key_has_type stop_hook_active true; then printf 'allow'; return 0; fi
        json_key_has_type stop_hook_active false || { echo 'MAESTRO-ROUTE-LITE: stop reentrancy flag is malformed' >&2; return 1; }
      fi
      case "$stage" in
        *_active) printf 'block|Maestro cannot finish while a managed specialist is still active' ;;
        account_framed) printf 'block|Maestro selected the strategic route; call Case Agent after Client Account Agent framing before finishing' ;;
        case_complete_strategic) printf 'block|Maestro selected the strategic route; return the Case result to Client Account Agent before finishing' ;;
        *) printf 'allow' ;;
      esac
      ;;
    *) return 2 ;;
  esac
)

case "$EVENT" in
  session-start)
    continuity=$(continuity_context)
    session_profile=$(session_profile_context)
    context='MAESTRO SCRIPT HOOKS ARE ACTIVE. Read .maestro-script/capabilities.json before capability claims, honor .maestro-script/agent-profile.md when present, load the smallest relevant managed skill, preserve client/local data, and never claim native CLI authority or authenticated receipts.'
    [ -z "$continuity" ] || context="$context $continuity"
    [ -z "$session_profile" ] || context="$context $session_profile"
    printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$context"
    ;;
  context-injection)
    if agent_route_lite begin; then
      printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"Use the managed Maestro projection and its capability matrix for this turn. agent-route-lite enforces only the bounded recognized specialist sequence; native signed route authority remains unavailable."}}'
    else
      status=$?
      [ "$status" -eq 2 ] && printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"Use the managed Maestro projection and its capability matrix for this turn. Native route authority remains unavailable."}}' || exit "$status"
    fi
    ;;
  pre-action-guard)
    input=$(mktemp "${TMPDIR:-/tmp}/maestro-script-hook.XXXXXX") || { deny; exit 0; }
    trap 'rm -f "$input"' EXIT HUP INT TERM
    /usr/bin/head -c 65537 > "$input" || { deny; exit 0; }
    bytes=$(wc -c < "$input" | tr -d ' ')
    if [ "$bytes" -eq 0 ] || [ "$bytes" -gt 65536 ] || ! command -v plutil >/dev/null 2>&1 || ! plutil -convert xml1 -o /dev/null "$input" >/dev/null 2>&1; then
      deny
      exit 0
    fi
    command_text=$(plutil -extract tool_input.command raw -o - "$input" 2>/dev/null || true)
    file_path=$(plutil -extract tool_input.file_path raw -o - "$input" 2>/dev/null || true)
    normalized_path=${file_path#./}
    case "$normalized_path" in
      "$WORKSPACE"/*) normalized_path=${normalized_path#"$WORKSPACE"/} ;;
    esac
    case "$normalized_path" in
      .claude/settings.local.json|.maestro-script/hooks/*|.maestro-script/agent-route-lite/*|.maestro-script/capabilities.json|.maestro-script/active-version|.maestro-script/managed-skills|.maestro-script/managed-agents|.claude/agents/client-account-agent.md|.claude/agents/case-agent.md|.claude/agents/walter.md|.claude/agents/darwin.md|.claude/agents/pa-expert.md) deny; exit 0 ;;
      .claude/skills/*)
        skill_name=${normalized_path#.claude/skills/}; skill_name=${skill_name%%/*}
        if [ -f "$WORKSPACE/.maestro-script/managed-skills" ] && grep -Fx "$skill_name" "$WORKSPACE/.maestro-script/managed-skills" >/dev/null; then deny; exit 0; fi
        ;;
    esac
    if printf '%s\n' "$command_text" | grep -Eiq '(^|[;&|][[:space:]]*)(/bin/|/usr/bin/)?rm[[:space:]]+(-[^[:space:]]*[rR][^[:space:]]*[fF]|-[^[:space:]]*[fF][^[:space:]]*[rR])[[:space:]]+(--[[:space:]]+)?(/|/System|/Library|/Users)([[:space:]]|$)'; then
      deny
      exit 0
    fi
    if printf '%s\n' "$command_text" | grep -Eiq '(^|[;&|][[:space:]]*)(/bin/|/usr/bin/)?rm[[:space:]]+(-[^[:space:]]*[rR][^[:space:]]*[fF]|-[^[:space:]]*[fF][^[:space:]]*[rR]).*(^|[[:space:]"])(\./)?(\.claude|\.maestro-script)(/|[[:space:]"]|$)'; then
      deny
      exit 0
    fi
    if printf '%s\n' "$command_text" | grep -Eiq '(^|[;&|][[:space:]]*)(git[[:space:]]+push|gh[[:space:]]+pr[[:space:]]+merge|curl[[:space:]].*(-X|--request)[[:space:]]*(POST|PUT|PATCH|DELETE))([[:space:]]|$)'; then
      ask_external
    fi
    ;;
  post-action-receipt) record_event PostToolUse ;;
  stop-finalization)
    record_event Stop
    if route_result=$(agent_route_lite finalize 2>/dev/null); then
      case "$route_result" in
        block\|*) reason=${route_result#block|}; printf '{"decision":"block","reason":"%s"}\n' "$reason" ;;
        allow) printf '%s\n' '{"continue":true}' ;;
        *) printf '%s\n' '{"decision":"block","reason":"Maestro route state requires repair before finishing"}' ;;
      esac
    else
      status=$?
      if [ "$status" -eq 2 ]; then printf '%s\n' '{"continue":true}'; else printf '%s\n' '{"decision":"block","reason":"Maestro route state is busy or requires repair before finishing"}'; fi
    fi
    ;;
  subagent-start)
    record_event SubagentStart
    if agent_route_lite start; then :; else status=$?; [ "$status" -eq 2 ] || exit "$status"; fi
    printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"SubagentStart","additionalContext":"You are a Maestro specialist in the script-only profile. Stay inside the delegated scope, do not delegate further, and return the result to Maestro. agent-route-lite applies bounded metadata-only sequence assurance; native signed route authority remains unavailable."}}'
    ;;
  subagent-stop)
    record_event SubagentStop
    if agent_route_lite stop; then :; else status=$?; [ "$status" -eq 2 ] || exit "$status"; fi
    ;;
  *) exit 2 ;;
esac
`)
}

func scriptPortablePowerShellHook() []byte {
	return []byte(`param(
  [Parameter(Mandatory=$true)][ValidateSet('session-start','context-injection','pre-action-guard','post-action-receipt','stop-finalization','subagent-start','subagent-stop')][string]$Event,
  [string]$Workspace = $env:CLAUDE_PROJECT_DIR
)
$ErrorActionPreference = 'Stop'

function Write-HookJson([object]$Value) {
  [Console]::Out.WriteLine(($Value | ConvertTo-Json -Compress -Depth 8))
}

function Read-BoundedHookInput() {
  [byte[]]$buffer = [Array]::CreateInstance([byte], 65537)
  [int]$count = 0
  $stream = [Console]::OpenStandardInput()
  try {
    while ($count -lt $buffer.Length) {
      [int]$remaining = $buffer.Length - $count
      [int]$read = $stream.Read($buffer, $count, $remaining)
      if ($read -eq 0) { break }
      $count += $read
    }
  } finally {
    $stream.Dispose()
  }
  if ($count -gt 65536) { throw 'MAESTRO-HOOK-INPUT: input exceeds 64 KiB' }
  if ($count -eq 0) { return '' }
  try {
    $utf8 = New-Object Text.UTF8Encoding($false, $true)
    return $utf8.GetString($buffer, 0, $count)
  } catch [Text.DecoderFallbackException] {
    throw 'MAESTRO-HOOK-INPUT: input is not valid UTF-8'
  }
}

function Write-Denial {
  Write-HookJson @{ hookSpecificOutput = @{ hookEventName = 'PreToolUse'; permissionDecision = 'deny'; permissionDecisionReason = 'Maestro script hook could not verify this action safely. Nothing was changed. Review the target and retry with a narrower action.' } }
}

function Write-Event([string]$Name) {
  if (-not $Workspace) { return }
  $state = Join-Path $Workspace '.maestro-script'
  if (-not (Test-Path -LiteralPath $state -PathType Container)) { return }
  $log = Join-Path $state 'hook-events.jsonl'
  if (((Get-Item -LiteralPath $state -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -or ((Test-Path -LiteralPath $log) -and ((Get-Item -LiteralPath $log -Force).Attributes -band [IO.FileAttributes]::ReparsePoint))) { return }
  Add-Content -LiteralPath $log -Value (@{ schema_version = 1; profile = 'script-only'; event = $Name; state = 'observed' } | ConvertTo-Json -Compress)
  if ((Get-Item -LiteralPath $log).Length -gt 65536) {
    $tail = @(Get-Content -LiteralPath $log | Select-Object -Last 200)
    [IO.File]::WriteAllLines("$log.$PID.tmp", $tail)
    Move-Item -LiteralPath "$log.$PID.tmp" -Destination $log -Force
  }
}

function Get-ContinuityContext {
  if (-not $Workspace) { return '' }
  $stateRoot = Join-Path $Workspace '.maestro-script'
  $path = Join-Path $stateRoot 'continuity-state.json'
  if (-not (Test-Path -LiteralPath $path)) { return '' }
  $invalid = 'CONTINUITY LITE REPAIR REQUIRED: the owner-local pointer is invalid; use the execution-continuity skill without inventing a task.'
  try {
    $stateItem = Get-Item -LiteralPath $stateRoot -Force
    if (-not $stateItem.PSIsContainer -or ($stateItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) { return $invalid }
    $item = Get-Item -LiteralPath $path -Force
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $item.Length -le 0 -or $item.Length -gt 2048) { return $invalid }
    $value = [IO.File]::ReadAllText($path) | ConvertFrom-Json
    if (($value.schema_version -isnot [int] -and $value.schema_version -isnot [long]) -or [int64]$value.schema_version -ne 1 -or
        $value.state -isnot [string] -or $value.state -notin @('active','paused') -or
        ($value.revision -isnot [int] -and $value.revision -isnot [long]) -or [int64]$value.revision -lt 1 -or [int64]$value.revision -gt 2147483647 -or
        $value.task -isnot [string] -or $value.checkpoint_present -isnot [bool]) { return $invalid }
    $task = [string]$value.task
    if ($task -notmatch '^brain/tasks/[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.md$' -or $task.Contains('..')) { return $invalid }
    $target = Join-Path $Workspace ($task.Replace('/', [IO.Path]::DirectorySeparatorChar))
    $targetItem = Get-Item -LiteralPath $target -Force
    if ($targetItem.PSIsContainer -or ($targetItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $targetItem.Length -gt 1048576) { return $invalid }
    $checkpoint = ([bool]$value.checkpoint_present).ToString().ToLowerInvariant()
    return "CONTINUITY LITE: state: $($value.state); reviewed task pointer: $task; checkpoint present: $checkpoint; revision: $($value.revision). Do not inject the task body; read it only when the owner asks to continue."
  } catch { return $invalid }
}

function Get-SessionProfileContext {
  if (-not $Workspace) { return '' }
  $stateRoot = Join-Path $Workspace '.maestro-script'
  $path = Join-Path $stateRoot 'session-profile.json'
  if (-not (Test-Path -LiteralPath $path)) { return 'SESSION PROFILE: interaction profile: standard; no reviewed session profile is registered.' }
  $invalid = 'SESSION PROFILE REPAIR REQUIRED: use interaction profile standard until the reviewed owner-local pointer is repaired.'
  try {
    $stateItem = Get-Item -LiteralPath $stateRoot -Force
    if (-not $stateItem.PSIsContainer -or ($stateItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) { return $invalid }
    $item = Get-Item -LiteralPath $path -Force
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $item.Length -le 0 -or $item.Length -gt 2048) { return $invalid }
    $value = [IO.File]::ReadAllText($path) | ConvertFrom-Json
    if (($value.schema_version -isnot [int] -and $value.schema_version -isnot [long]) -or [int64]$value.schema_version -ne 1 -or
        $value.interaction_profile -isnot [string] -or $value.local_profile -isnot [string] -or $value.profile_sha256 -isnot [string] -or
        ($value.revision -isnot [int] -and $value.revision -isnot [long]) -or [int64]$value.revision -lt 1 -or [int64]$value.revision -gt 2147483647 -or
        $value.session_use_confirmed -isnot [bool] -or -not [bool]$value.session_use_confirmed) { return $invalid }
    $profile = [string]$value.interaction_profile
    $pointer = [string]$value.local_profile
    $digest = [string]$value.profile_sha256
    if ($profile -notin @('standard','advanced','power') -or $pointer -ne '.maestro-script/local-profile.md' -or $digest -notmatch '^[a-f0-9]{64}$') { return $invalid }
    $target = Join-Path $Workspace '.maestro-script\local-profile.md'
    $targetItem = Get-Item -LiteralPath $target -Force
    if ($targetItem.PSIsContainer -or ($targetItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $targetItem.Length -gt 1048576) { return $invalid }
    $actual = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $digest) { return $invalid }
    return "SESSION PROFILE: interaction profile: $profile; reviewed profile pointer: $pointer; revision: $($value.revision). Never inject the profile body; read only the relevant section when needed."
  } catch { return $invalid }
}

function Get-RouteDigest([string]$Value) {
  $sha = [Security.Cryptography.SHA256]::Create()
  try { return ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Value)))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
}

function Invoke-AgentRouteLite([string]$Mode, [string]$Raw) {
  if (-not $Raw -or [Text.Encoding]::UTF8.GetByteCount($Raw) -gt 65536) { return @{ managed = $false; result = '' } }
  try { $inputValue = $Raw | ConvertFrom-Json } catch { return @{ managed = $false; result = '' } }
  if ($inputValue.session_id -isnot [string] -or -not $inputValue.session_id -or $inputValue.session_id.Length -gt 256) { return @{ managed = $false; result = '' } }
  $sessionDigest = Get-RouteDigest $inputValue.session_id
  $stateRoot = Join-Path $Workspace '.maestro-script'
  $routeRoot = Join-Path $stateRoot 'agent-route-lite'
  $stateRootItem = Get-Item -LiteralPath $stateRoot -Force
  if (-not $stateRootItem.PSIsContainer -or ($stateRootItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-ROUTE-LITE: state root is unsafe' }
  if (Test-Path -LiteralPath $routeRoot) {
    $routeRootItem = Get-Item -LiteralPath $routeRoot -Force
    if (-not $routeRootItem.PSIsContainer -or ($routeRootItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-ROUTE-LITE: route state is unsafe' }
  } else { New-Item -ItemType Directory -Path $routeRoot | Out-Null }
  $statePath = Join-Path $routeRoot "$sessionDigest.json"
  $lockPath = "$statePath.lock"
  try {
    $lock = [IO.File]::Open($lockPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    $lock.Dispose()
  } catch { throw 'MAESTRO-ROUTE-LITE: route state is busy; retry once' }
  try {
    $state = [ordered]@{ schema_version = 1; session_digest = $sessionDigest; stage = 'idle'; transition_count = 0; active_agent_digest = ''; active_agent_type = ''; last_event = ''; last_agent_digest = ''; last_agent_type = '' }
    if (Test-Path -LiteralPath $statePath) {
      $item = Get-Item -LiteralPath $statePath -Force
      if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $item.Length -le 0 -or $item.Length -gt 2048) { throw 'MAESTRO-ROUTE-LITE: route state requires repair' }
      try { $stored = [IO.File]::ReadAllText($statePath) | ConvertFrom-Json } catch { throw 'MAESTRO-ROUTE-LITE: route state requires repair' }
      if (($stored.schema_version -isnot [int] -and $stored.schema_version -isnot [long]) -or [int64]$stored.schema_version -ne 1 -or
          $stored.session_digest -isnot [string] -or $stored.session_digest -ne $sessionDigest -or $stored.stage -isnot [string] -or
          $stored.stage -notin @('idle','account_active','account_framed','case_active_direct','case_active_strategic','case_complete_direct','case_complete_strategic','account_validation_active','complete','darwin_active','darwin_complete','pa_active','pa_complete','walter_active','walter_complete') -or
          ($stored.transition_count -isnot [int] -and $stored.transition_count -isnot [long]) -or [int64]$stored.transition_count -lt 0 -or [int64]$stored.transition_count -gt 32) { throw 'MAESTRO-ROUTE-LITE: route state requires repair' }
      foreach ($name in @('active_agent_digest','last_agent_digest')) { $value = [string]$stored.$name; if ($value -and $value -notmatch '^[a-f0-9]{64}$') { throw 'MAESTRO-ROUTE-LITE: route state requires repair' } }
      foreach ($name in @('active_agent_type','last_agent_type')) { $value = [string]$stored.$name; if ($value -and $value -notin @('client-account-agent','case-agent','walter','darwin','pa-expert')) { throw 'MAESTRO-ROUTE-LITE: route state requires repair' } }
      if ([string]$stored.last_event -notin @('','start','stop')) { throw 'MAESTRO-ROUTE-LITE: route state requires repair' }
      foreach ($name in $state.Keys) { $state[$name] = $stored.$name }
    }
    $save = {
      $body = $state | ConvertTo-Json -Compress
      if ([Text.Encoding]::UTF8.GetByteCount($body) -gt 2048) { throw 'MAESTRO-ROUTE-LITE: route state exceeds its bound' }
      $temporary = Join-Path $routeRoot ".route-$sessionDigest-$PID.tmp"
      [IO.File]::WriteAllText($temporary, $body, (New-Object Text.UTF8Encoding($false)))
      Move-Item -LiteralPath $temporary -Destination $statePath -Force
    }
    if ($Mode -eq 'begin') {
      if ([string]$state.stage -like '*_active') { throw 'MAESTRO-ROUTE-LITE: cannot begin a new turn while a specialist is active' }
      $state.stage = 'idle'; $state.transition_count = 0; $state.active_agent_digest = ''; $state.active_agent_type = ''; $state.last_event = ''; $state.last_agent_digest = ''; $state.last_agent_type = ''
      & $save
      return @{ managed = $true; result = 'begun' }
    }
    if ($Mode -eq 'finalize') {
      if ($inputValue.stop_hook_active -is [bool] -and [bool]$inputValue.stop_hook_active) { return @{ managed = $true; result = 'allow' } }
      $result = switch -Wildcard ([string]$state.stage) {
        '*_active' { 'block|Maestro cannot finish while a managed specialist is still active'; break }
        'account_framed' { 'block|Maestro selected the strategic route; call Case Agent after Client Account Agent framing before finishing'; break }
        'case_complete_strategic' { 'block|Maestro selected the strategic route; return the Case result to Client Account Agent before finishing'; break }
        default { 'allow' }
      }
      return @{ managed = $true; result = $result }
    }
    if ($inputValue.agent_id -isnot [string] -or -not $inputValue.agent_id -or $inputValue.agent_id.Length -gt 256 -or $inputValue.agent_type -isnot [string]) { throw 'MAESTRO-ROUTE-LITE: managed agent identity is missing' }
    $agentType = [string]$inputValue.agent_type
    if ($agentType -notin @('client-account-agent','case-agent','walter','darwin','pa-expert')) { return @{ managed = $false; result = '' } }
    $agentDigest = Get-RouteDigest $inputValue.agent_id
    if ($Mode -eq 'start') {
      if ($state.active_agent_digest) {
        if ($state.active_agent_digest -eq $agentDigest -and $state.active_agent_type -eq $agentType -and $state.last_event -eq 'start') { return @{ managed = $true; result = 'idempotent' } }
        throw 'MAESTRO-ROUTE-LITE: one managed specialist is still active'
      }
      if ([int64]$state.transition_count -ge 32) { throw 'MAESTRO-ROUTE-LITE: transition limit reached' }
      $transition = "$($state.stage):$agentType"
      $next = switch ($transition) {
        'idle:client-account-agent' { 'account_active' }
        'idle:case-agent' { 'case_active_direct' }
        'idle:darwin' { 'darwin_active' }
        'idle:pa-expert' { 'pa_active' }
        'idle:walter' { 'walter_active' }
        'account_framed:case-agent' { 'case_active_strategic' }
        'case_complete_strategic:client-account-agent' { 'account_validation_active' }
        'case_complete_direct:walter' { 'walter_active' }
        'complete:walter' { 'walter_active' }
        default { '' }
      }
      if (-not $next) {
        if ($state.stage -eq 'darwin_complete' -or $agentType -eq 'darwin') { throw 'MAESTRO-ROUTE-LITE: Darwin system-health work cannot be mixed with client execution in the same turn' }
        throw 'MAESTRO-ROUTE-LITE: specialist order is invalid for the selected route'
      }
      $state.stage = $next; $state.active_agent_digest = $agentDigest; $state.active_agent_type = $agentType; $state.last_event = 'start'; $state.last_agent_digest = $agentDigest; $state.last_agent_type = $agentType; $state.transition_count = [int64]$state.transition_count + 1
      & $save
      return @{ managed = $true; result = 'started' }
    }
    if ($Mode -eq 'stop') {
      if (-not $state.active_agent_digest -and $state.last_event -eq 'stop' -and $state.last_agent_digest -eq $agentDigest -and $state.last_agent_type -eq $agentType) { return @{ managed = $true; result = 'idempotent' } }
      if ($state.active_agent_digest -ne $agentDigest -or $state.active_agent_type -ne $agentType) { throw 'MAESTRO-ROUTE-LITE: specialist stop does not match the active specialist' }
      $transition = "$($state.stage):$agentType"
      $next = switch ($transition) {
        'account_active:client-account-agent' { 'account_framed' }
        'case_active_direct:case-agent' { 'case_complete_direct' }
        'case_active_strategic:case-agent' { 'case_complete_strategic' }
        'account_validation_active:client-account-agent' { 'complete' }
        'darwin_active:darwin' { 'darwin_complete' }
        'pa_active:pa-expert' { 'pa_complete' }
        'walter_active:walter' { 'walter_complete' }
        default { '' }
      }
      if (-not $next) { throw 'MAESTRO-ROUTE-LITE: specialist stop is invalid for the selected route' }
      $state.stage = $next; $state.active_agent_digest = ''; $state.active_agent_type = ''; $state.last_event = 'stop'; $state.last_agent_digest = $agentDigest; $state.last_agent_type = $agentType; $state.transition_count = [int64]$state.transition_count + 1
      & $save
      return @{ managed = $true; result = 'stopped' }
    }
    return @{ managed = $false; result = '' }
  } finally { Remove-Item -LiteralPath $lockPath -Force -ErrorAction SilentlyContinue }
}

switch ($Event) {
  'session-start' {
    $context = 'MAESTRO SCRIPT HOOKS ARE ACTIVE. Read .maestro-script/capabilities.json before capability claims, honor .maestro-script/agent-profile.md when present, load the smallest relevant managed skill, preserve client/local data, and never claim native CLI authority or authenticated receipts.'
    $continuity = Get-ContinuityContext
    if ($continuity) { $context += ' ' + $continuity }
    $sessionProfile = Get-SessionProfileContext
    if ($sessionProfile) { $context += ' ' + $sessionProfile }
    Write-HookJson @{ hookSpecificOutput = @{ hookEventName = 'SessionStart'; additionalContext = $context } }
  }
  'context-injection' {
    try { $route = Invoke-AgentRouteLite 'begin' (Read-BoundedHookInput) } catch { $route = @{ managed = $false; result = '' } }
    $context = if ($route.managed) { 'Use the managed Maestro projection and its capability matrix for this turn. agent-route-lite enforces only the bounded recognized specialist sequence; native signed route authority remains unavailable.' } else { 'Use the managed Maestro projection and its capability matrix for this turn. Native route authority remains unavailable.' }
    Write-HookJson @{ hookSpecificOutput = @{ hookEventName = 'UserPromptSubmit'; additionalContext = $context } }
  }
  'pre-action-guard' {
    try { $raw = Read-BoundedHookInput } catch { Write-Denial; exit 0 }
    if (-not $raw -or [Text.Encoding]::UTF8.GetByteCount($raw) -gt 65536) { Write-Denial; exit 0 }
    try { $inputObject = $raw | ConvertFrom-Json } catch { Write-Denial; exit 0 }
    $command = [string]$inputObject.tool_input.command
    $filePath = [string]$inputObject.tool_input.file_path
    $normalizedPath = $filePath.Replace('\','/').TrimStart('./')
    $workspacePrefix = $Workspace.Replace('\','/').TrimEnd('/') + '/'
    if ($normalizedPath.StartsWith($workspacePrefix, [StringComparison]::OrdinalIgnoreCase)) { $normalizedPath = $normalizedPath.Substring($workspacePrefix.Length) }
    if ($normalizedPath -match '^(?i)(\.claude/settings\.local\.json|\.maestro-script/(hooks/|agent-route-lite/|capabilities\.json$|active-version$|managed-skills$|managed-agents$)|\.claude/agents/(client-account-agent|case-agent|walter|darwin|pa-expert)\.md$)') { Write-Denial; exit 0 }
    if ($normalizedPath -match '^(?i)\.claude/skills/([^/]+)(/|$)') {
      $managedSkills = Join-Path $Workspace '.maestro-script\managed-skills'
      if ((Test-Path -LiteralPath $managedSkills -PathType Leaf) -and ((Get-Content -LiteralPath $managedSkills) -contains $Matches[1])) { Write-Denial; exit 0 }
    }
    if ($command -match '(?i)(^|[;&|]\s*)(rd|rmdir|del|erase)\s+.*(/s|-recurse).*(/q|-force).*([A-Z]:\\|%USERPROFILE%|\\Windows|\\Program Files)' -or
        $command -match '(?i)(^|[;&|]\s*)Remove-Item\s+.*-Recurse.*-Force.*([A-Z]:\\|\$env:USERPROFILE|\\Windows|\\Program Files)') { Write-Denial; exit 0 }
    if ($command -match '(?i)(Remove-Item|rd|rmdir)\b.*(-Recurse|/s).*(\.claude|\.maestro-script)') { Write-Denial; exit 0 }
    if ($command -match '(?i)(^|[;&|]\s*)(git\s+push|gh\s+pr\s+merge|Invoke-RestMethod\b.*-(Method)\s+(Post|Put|Patch|Delete))') {
      Write-HookJson @{ hookSpecificOutput = @{ hookEventName = 'PreToolUse'; permissionDecision = 'ask'; permissionDecisionReason = 'This appears to publish or mutate an external system. Ask the owner for confirmation before continuing. This is Claude permission handling, not a native Maestro challenge receipt.' } }
    }
  }
  'post-action-receipt' { Write-Event 'PostToolUse' }
  'stop-finalization' {
    Write-Event 'Stop'
    try {
      $route = Invoke-AgentRouteLite 'finalize' (Read-BoundedHookInput)
      if ($route.managed -and $route.result.StartsWith('block|')) { Write-HookJson @{ decision = 'block'; reason = $route.result.Substring(6) } else { Write-HookJson @{ continue = $true } }
    } catch {
      Write-HookJson @{ decision = 'block'; reason = 'Maestro route state is busy or requires repair before finishing' }
    }
  }
  'subagent-start' {
    Write-Event 'SubagentStart'
    $route = Invoke-AgentRouteLite 'start' (Read-BoundedHookInput)
    Write-HookJson @{ hookSpecificOutput = @{ hookEventName = 'SubagentStart'; additionalContext = 'You are a Maestro specialist in the script-only profile. Stay inside the delegated scope, do not delegate further, and return the result to Maestro. agent-route-lite applies bounded metadata-only sequence assurance; native signed route authority remains unavailable.' } }
  }
  'subagent-stop' { Write-Event 'SubagentStop'; $null = Invoke-AgentRouteLite 'stop' (Read-BoundedHookInput) }
}
`)
}

func scriptPortablePOSIXInstaller(version string) []byte {
	return []byte(strings.ReplaceAll(`#!/bin/sh
set -eu
VERSION='@@VERSION@@'
PROFILE='macos-shell-local-beta'
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
ACTION=${1:-install}
if [ "$#" -gt 0 ]; then shift; fi
WORKSPACE=${MAESTRO_WORKSPACE_HOME:-"$HOME/Maestro"}/maestro-os
WORKSPACE_EXPLICIT=0
REVEAL_WORKSPACE=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --workspace) [ "$#" -ge 2 ] || { echo 'MAESTRO-SCRIPT-ARGS: workspace ausente' >&2; exit 2; }; WORKSPACE=$2; WORKSPACE_EXPLICIT=1; shift 2 ;;
    --reveal-workspace) REVEAL_WORKSPACE=1; shift ;;
    *) echo "MAESTRO-SCRIPT-ARGS: argumento desconhecido: $1" >&2; exit 2 ;;
  esac
done
case "$(uname -s 2>/dev/null || true)" in Darwin) ;; *) echo 'MAESTRO-SCRIPT-OS: este pacote requer macOS' >&2; exit 3;; esac
[ "$(id -u)" -ne 0 ] || { echo 'MAESTRO-SCRIPT-ADMIN: execute como usuario normal, nunca como root' >&2; exit 4; }
RUNTIME_ROOT=${MAESTRO_SCRIPT_HOME:-"$HOME/Library/Application Support/Maestro/script-runtime"}
STATE="$RUNTIME_ROOT/state"
if [ "$WORKSPACE_EXPLICIT" -eq 0 ] && [ -f "$STATE/workspace" ]; then
  WORKSPACE=$(cat "$STATE/workspace")
fi

prepare_workspace() {
  if [ ! -e "$WORKSPACE" ]; then
    mkdir -p "$WORKSPACE"
    cp "$ROOT/maestro-os/README.md" "$WORKSPACE/README.md"
  fi
  [ -d "$WORKSPACE" ] && [ ! -L "$WORKSPACE" ] || { echo 'MAESTRO-SCRIPT-WORKSPACE: destino permanente inseguro ou invalido' >&2; return 1; }
}

reveal_workspace() {
  [ "$REVEAL_WORKSPACE" -eq 1 ] || return 0
  if ! /usr/bin/open "$WORKSPACE"; then
    echo "MAESTRO-SCRIPT-REVEAL: nao consegui mostrar a pasta; abra manualmente: $WORKSPACE" >&2
  fi
}

verify_package() {
  [ -f "$ROOT/inventory.sha256" ] || { echo 'MAESTRO-SCRIPT-INTEGRITY: inventario ausente' >&2; return 1; }
  if find "$ROOT" -type l -print | grep -q .; then echo 'MAESTRO-SCRIPT-INTEGRITY: link simbolico no pacote' >&2; return 1; fi
  while IFS= read -r line; do
    digest=$(printf '%s' "$line" | cut -c1-64)
    relative=$(printf '%s' "$line" | cut -c67-)
    case "$relative" in ''|/*|*'..'*|*\\*) echo 'MAESTRO-SCRIPT-INTEGRITY: caminho invalido' >&2; return 1;; esac
    [ -f "$ROOT/$relative" ] && [ ! -L "$ROOT/$relative" ] || { echo "MAESTRO-SCRIPT-INTEGRITY: arquivo ausente ou inseguro: $relative" >&2; return 1; }
    actual=$(shasum -a 256 "$ROOT/$relative" | awk '{print $1}')
    [ "$actual" = "$digest" ] || { echo "MAESTRO-SCRIPT-INTEGRITY: arquivo alterado: $relative" >&2; return 1; }
  done < "$ROOT/inventory.sha256"
  find "$ROOT" -type f -print | while IFS= read -r file; do
    relative=${file#"$ROOT/"}
    [ "$relative" = inventory.sha256 ] && continue
    awk -v path="$relative" 'substr($0,67)==path { found=1 } END { exit !found }' "$ROOT/inventory.sha256" || { echo "MAESTRO-SCRIPT-INTEGRITY: arquivo nao declarado: $relative" >&2; exit 1; }
  done
}

verify_runtime() {
  runtime=$1
  [ -f "$runtime/runtime-inventory.sha256" ] || { echo 'MAESTRO-SCRIPT-RUNTIME: inventario ausente' >&2; return 1; }
  if find "$runtime" -type l -print | grep -q .; then echo 'MAESTRO-SCRIPT-RUNTIME: link simbolico no runtime' >&2; return 1; fi
  while IFS= read -r line; do
    digest=$(printf '%s' "$line" | cut -c1-64)
    relative=$(printf '%s' "$line" | cut -c67-)
    case "$relative" in ''|/*|*'..'*|*\\*) echo 'MAESTRO-SCRIPT-RUNTIME: caminho invalido' >&2; return 1;; esac
    [ -f "$runtime/$relative" ] && [ ! -L "$runtime/$relative" ] || { echo "MAESTRO-SCRIPT-RUNTIME: arquivo ausente ou inseguro: $relative" >&2; return 1; }
    actual=$(shasum -a 256 "$runtime/$relative" | awk '{print $1}')
    [ "$actual" = "$digest" ] || { echo "MAESTRO-SCRIPT-RUNTIME: arquivo alterado: $relative" >&2; return 1; }
  done < "$runtime/runtime-inventory.sha256"
  find "$runtime" -type f -print | while IFS= read -r file; do
    relative=${file#"$runtime/"}
    [ "$relative" = runtime-inventory.sha256 ] && continue
    awk -v path="$relative" 'substr($0,67)==path { found=1 } END { exit !found }' "$runtime/runtime-inventory.sha256" || { echo "MAESTRO-SCRIPT-RUNTIME: arquivo nao declarado: $relative" >&2; exit 1; }
  done
}

version_greater_than() {
  awk -v candidate="$1" -v current="$2" 'BEGIN {
    split(candidate,n,"."); split(current,c,".")
    for (i=1;i<=3;i++) { n[i]+=0; c[i]+=0; if (n[i]>c[i]) exit 0; if (n[i]<c[i]) exit 1 }
    exit 1
  }'
}

failure_point() {
  :
}

projection_lock_acquire() {
  lock="$STATE/projection-lock"
  reclaim="$STATE/projection-lock-reclaim"
  PROJECTION_LOCK_TOKEN="$$:$(date +%s)"
  if (set -C; umask 077; printf '%s\n' "$PROJECTION_LOCK_TOKEN" > "$lock") 2>/dev/null; then
    return 0
  fi
  [ -f "$lock" ] && [ ! -L "$lock" ] || { echo 'MAESTRO-SCRIPT-BUSY: lock de projecao incompleto; nada foi alterado' >&2; return 1; }
  observed=$(cat "$lock" 2>/dev/null || true)
  owner=${observed%%:*}
  case "$observed" in *:*) ;; *) echo 'MAESTRO-SCRIPT-BUSY: lock de projecao invalido; nada foi alterado' >&2; return 1;; esac
  case "$owner" in ''|*[!0-9]*) echo 'MAESTRO-SCRIPT-BUSY: lock de projecao invalido; nada foi alterado' >&2; return 1;; esac
  if kill -0 "$owner" 2>/dev/null; then echo 'MAESTRO-SCRIPT-BUSY: outra projecao esta em andamento; nada foi alterado' >&2; return 1; fi
  mkdir "$reclaim" 2>/dev/null || { echo 'MAESTRO-SCRIPT-BUSY: recuperacao de lock em andamento; nada foi alterado' >&2; return 1; }
  current=$(cat "$lock" 2>/dev/null || true)
  if [ "$current" != "$observed" ]; then rmdir "$reclaim" 2>/dev/null || true; echo 'MAESTRO-SCRIPT-BUSY: lock mudou durante recuperacao; nada foi alterado' >&2; return 1; fi
  rm -f "$lock"
  if ! (set -C; umask 077; printf '%s\n' "$PROJECTION_LOCK_TOKEN" > "$lock") 2>/dev/null; then
    rmdir "$reclaim" 2>/dev/null || true
    echo 'MAESTRO-SCRIPT-BUSY: outra projecao assumiu o lock; nada foi alterado' >&2
    return 1
  fi
  rmdir "$reclaim" 2>/dev/null || true
}

projection_lock_release() {
  lock="$STATE/projection-lock"
  [ -f "$lock" ] && [ ! -L "$lock" ] || return 0
  current=$(cat "$lock" 2>/dev/null || true)
  [ -n "${PROJECTION_LOCK_TOKEN:-}" ] && [ "$current" = "$PROJECTION_LOCK_TOKEN" ] || return 0
  rm -f "$lock"
  PROJECTION_LOCK_TOKEN=''
}

copy_projection_backup() {
  from_release=$1
  backup=$2
  mkdir -p "$backup/skills" "$backup/agents" "$backup/fixed"
  if [ -n "$from_release" ] && [ -d "$from_release/projection/skills" ]; then
    for source in "$from_release/projection/skills"/*; do
      [ -d "$source" ] || continue
      name=$(basename "$source")
      [ ! -e "$WORKSPACE/.claude/skills/$name" ] || cp -R "$WORKSPACE/.claude/skills/$name" "$backup/skills/$name"
    done
  fi
  if [ -n "$from_release" ] && [ -d "$from_release/projection/agents" ]; then
    for source in "$from_release/projection/agents"/*.md; do
      [ -f "$source" ] || continue
      name=$(basename "$source")
      [ ! -e "$WORKSPACE/.claude/agents/$name" ] || cp "$WORKSPACE/.claude/agents/$name" "$backup/agents/$name"
    done
  fi
  for item in settings.local.json; do
    source="$WORKSPACE/.claude/$item"
    [ ! -e "$source" ] || cp "$source" "$backup/fixed/claude-$item"
  done
  for item in hooks capabilities.json active-version managed-skills managed-agents settings-local.sha256 projection-receipt.json; do
    source="$WORKSPACE/.maestro-script/$item"
    [ ! -e "$source" ] || cp -R "$source" "$backup/fixed/state-$item"
  done
  if [ -f "$WORKSPACE/CLAUDE.md" ]; then
    : > "$backup/claude-original-present"
    shasum -a 256 "$WORKSPACE/CLAUDE.md" | awk '{print $1}' > "$backup/claude-original.sha256"
    wc -c < "$WORKSPACE/CLAUDE.md" | tr -d ' ' > "$backup/claude-original.size"
    perl -0777 -e '
      use strict; use warnings;
      local $/; my $body=<>;
      my $begin="<!-- MAESTRO SCRIPT MANAGED BEGIN -->";
      my $end="<!-- MAESTRO SCRIPT MANAGED END -->";
      my $start=index($body,$begin);
      if ($start>=0) { my $finish=index($body,$end,$start); print substr($body,$start,$finish+length($end)-$start) if $finish>=0; }
    ' "$WORKSPACE/CLAUDE.md" > "$backup/managed-claude-block"
    [ -s "$backup/managed-claude-block" ] || rm -f "$backup/managed-claude-block"
  fi
  [ ! -f "$STATE/active-version" ] || cp "$STATE/active-version" "$backup/global-active-version"
  [ ! -f "$STATE/previous-version" ] || cp "$STATE/previous-version" "$backup/global-previous-version"
  [ ! -f "$STATE/workspace" ] || cp "$STATE/workspace" "$backup/global-workspace"
}

remove_release_projection_paths() {
  release=$1
  [ -n "$release" ] && [ -d "$release" ] || return 0
  for source in "$release/projection/skills"/*; do [ ! -d "$source" ] || rm -rf "$WORKSPACE/.claude/skills/$(basename "$source")"; done
  for source in "$release/projection/agents"/*.md; do [ ! -f "$source" ] || rm -f "$WORKSPACE/.claude/agents/$(basename "$source")"; done
}

restore_fixed_projection_path() {
  backup=$1
  destination=$2
  rm -rf "$destination"
  [ ! -e "$backup" ] || cp -R "$backup" "$destination"
}

projection_tree_is_known() {
  expected=$1
  actual=$2
  [ -d "$expected" ] && [ -d "$actual" ] && [ ! -L "$actual" ] || return 1
  differences=$(diff -qr "$expected" "$actual" 2>/dev/null || true)
  [ -z "$differences" ] || printf '%s\n' "$differences" | awk -v prefix="Only in $expected" 'index($0,prefix)!=1{exit 1}'
}

projection_file_is_known() {
  actual=$1
  first=$2
  second=$3
  [ ! -e "$actual" ] && return 0
  [ -f "$actual" ] && [ ! -L "$actual" ] || return 1
  [ -n "$first" ] && [ -f "$first" ] && cmp -s "$first" "$actual" && return 0
  [ -n "$second" ] && [ -f "$second" ] && cmp -s "$second" "$actual" && return 0
  return 1
}

validate_recovery_live_projection() {
  from_release=$1
  to_release=$2
  for release in "$from_release" "$to_release"; do
    [ -n "$release" ] && [ -d "$release" ] || continue
    for source in "$release/projection/skills"/*; do
      [ -d "$source" ] || continue
      name=$(basename "$source")
      actual="$WORKSPACE/.claude/skills/$name"
      [ ! -e "$actual" ] && continue
      first=''; second=''
      [ -z "$from_release" ] || first="$from_release/projection/skills/$name"
      second="$to_release/projection/skills/$name"
      projection_tree_is_known "$first" "$actual" || projection_tree_is_known "$second" "$actual" || { echo "MAESTRO-SCRIPT-CONFLICT: skill mudou durante recuperacao: $name; nada foi sobrescrito" >&2; return 1; }
    done
    for source in "$release/projection/agents"/*.md; do
      [ -f "$source" ] || continue
      name=$(basename "$source")
      first=''; second=''
      [ -z "$from_release" ] || first="$from_release/projection/agents/$name"
      second="$to_release/projection/agents/$name"
      projection_file_is_known "$WORKSPACE/.claude/agents/$name" "$first" "$second" || { echo "MAESTRO-SCRIPT-CONFLICT: agente mudou durante recuperacao: $name; nada foi sobrescrito" >&2; return 1; }
    done
  done
  from_settings=''; from_hook=''; from_capabilities=''
  if [ -n "$from_release" ]; then from_settings="$from_release/projection/settings.local.json"; from_hook="$from_release/maestro-hook.sh"; from_capabilities="$from_release/capabilities.json"; fi
  projection_file_is_known "$WORKSPACE/.claude/settings.local.json" "$from_settings" "$to_release/projection/settings.local.json" || { echo 'MAESTRO-SCRIPT-CONFLICT: settings mudou durante recuperacao; nada foi sobrescrito' >&2; return 1; }
  projection_file_is_known "$WORKSPACE/.maestro-script/capabilities.json" "$from_capabilities" "$to_release/capabilities.json" || { echo 'MAESTRO-SCRIPT-CONFLICT: capacidades mudaram durante recuperacao; nada foi sobrescrito' >&2; return 1; }
  hooks_root="$WORKSPACE/.maestro-script/hooks"
  if [ -e "$hooks_root" ]; then
    [ -d "$hooks_root" ] && [ ! -L "$hooks_root" ] || { echo 'MAESTRO-SCRIPT-CONFLICT: hooks mudaram durante recuperacao; nada foi sobrescrito' >&2; return 1; }
    hook_count=$(find "$hooks_root" -mindepth 1 -maxdepth 1 -print | awk 'END{print NR}')
    [ "$hook_count" -eq 1 ] && [ -f "$hooks_root/maestro-hook.sh" ] && [ ! -L "$hooks_root/maestro-hook.sh" ] || { echo 'MAESTRO-SCRIPT-CONFLICT: hooks mudaram durante recuperacao; nada foi sobrescrito' >&2; return 1; }
    projection_file_is_known "$hooks_root/maestro-hook.sh" "$from_hook" "$to_release/maestro-hook.sh" || { echo 'MAESTRO-SCRIPT-CONFLICT: hook mudou durante recuperacao; nada foi sobrescrito' >&2; return 1; }
  fi
}

restore_projection_transaction() {
  tx="$STATE/projection-transaction"
  [ -f "$tx/prepared" ] || { rm -rf "$tx"; return 0; }
  from=$(cat "$tx/from-version")
  to=$(cat "$tx/to-version")
  tx_workspace=$(cat "$tx/workspace")
  [ "$tx_workspace" = "$WORKSPACE" ] || { echo 'MAESTRO-SCRIPT-CONFLICT: journal pertence a outro workspace; nada foi sobrescrito' >&2; return 1; }
  from_release=''
  [ -z "$from" ] || from_release="$RUNTIME_ROOT/releases/$from"
  to_release="$RUNTIME_ROOT/releases/$to"
  verify_runtime "$to_release"
  [ -z "$from_release" ] || verify_runtime "$from_release"
  validate_recovery_live_projection "$from_release" "$to_release"
  base="$WORKSPACE/CLAUDE.md"
  original_hash=''
  [ ! -f "$tx/backup/claude-original.sha256" ] || original_hash=$(cat "$tx/backup/claude-original.sha256")
  target_hash=$(cat "$tx/claude-target.sha256")
  current_hash='absent'
  [ ! -f "$base" ] || current_hash=$(shasum -a 256 "$base" | awk '{print $1}')
  if [ "$current_hash" != "$target_hash" ] && { [ -z "$original_hash" ] || [ "$current_hash" != "$original_hash" ]; }; then
    [ "$current_hash" = absent ] && [ ! -f "$tx/backup/claude-original-present" ] || { echo 'MAESTRO-SCRIPT-CONFLICT: CLAUDE.md mudou durante recuperacao; nada foi sobrescrito' >&2; return 1; }
  fi
  remove_release_projection_paths "$from_release"
  remove_release_projection_paths "$to_release"
  for source in "$tx/backup/skills"/*; do [ ! -d "$source" ] || cp -R "$source" "$WORKSPACE/.claude/skills/$(basename "$source")"; done
  for source in "$tx/backup/agents"/*.md; do [ ! -f "$source" ] || cp "$source" "$WORKSPACE/.claude/agents/$(basename "$source")"; done
  restore_fixed_projection_path "$tx/backup/fixed/claude-settings.local.json" "$WORKSPACE/.claude/settings.local.json"
  for item in hooks capabilities.json active-version managed-skills managed-agents settings-local.sha256 projection-receipt.json; do
    restore_fixed_projection_path "$tx/backup/fixed/state-$item" "$WORKSPACE/.maestro-script/$item"
  done
  if [ "$current_hash" = "$target_hash" ]; then
    temp="$WORKSPACE/.maestro-script/CLAUDE.md.recovery.tmp"
    if [ ! -f "$tx/backup/claude-original-present" ]; then
      rm -f "$base"
    elif [ -f "$tx/backup/managed-claude-block" ]; then
      render_projected_claude "$base" "$tx/backup/managed-claude-block" > "$temp"
      restored_hash=$(shasum -a 256 "$temp" | awk '{print $1}')
      [ "$restored_hash" = "$original_hash" ] || { rm -f "$temp"; echo 'MAESTRO-SCRIPT-CONFLICT: CLAUDE.md nao pode ser restaurado sem perda; nada foi sobrescrito' >&2; return 1; }
      mv "$temp" "$base"
    else
      original_size=$(cat "$tx/backup/claude-original.size")
      dd if="$base" of="$temp" bs=1 count="$original_size" 2>/dev/null
      restored_hash=$(shasum -a 256 "$temp" | awk '{print $1}')
      [ "$restored_hash" = "$original_hash" ] || { rm -f "$temp"; echo 'MAESTRO-SCRIPT-CONFLICT: CLAUDE.md nao pode ser restaurado sem perda; nada foi sobrescrito' >&2; return 1; }
      mv "$temp" "$base"
    fi
  fi
  restore_fixed_projection_path "$tx/backup/global-active-version" "$STATE/active-version"
  restore_fixed_projection_path "$tx/backup/global-previous-version" "$STATE/previous-version"
  restore_fixed_projection_path "$tx/backup/global-workspace" "$STATE/workspace"
  rm -rf "$tx"
}

recover_projection_transaction() {
  [ -d "$STATE/projection-transaction" ] || return 0
  projection_lock_acquire
  restore_projection_transaction
  projection_lock_release
}

begin_projection_transaction() {
  release_root=$1
  desired_previous=$2
  projection_lock_acquire
  tx_stage="$STATE/.projection-transaction-$$"
  rm -rf "$tx_stage"
  mkdir -p "$tx_stage"
  from=''
  [ ! -f "$STATE/active-version" ] || from=$(cat "$STATE/active-version")
  from_release=''
  [ -z "$from" ] || from_release="$RUNTIME_ROOT/releases/$from"
  copy_projection_backup "$from_release" "$tx_stage/backup"
  write_managed_claude_block "$release_root" "$tx_stage/claude-target-block"
  render_projected_claude "$WORKSPACE/CLAUDE.md" "$tx_stage/claude-target-block" | shasum -a 256 | awk '{print $1}' > "$tx_stage/claude-target.sha256"
  printf '%s\n' "$from" > "$tx_stage/from-version"
  printf '%s\n' "$(basename "$release_root")" > "$tx_stage/to-version"
  printf '%s\n' "$desired_previous" > "$tx_stage/desired-previous-version"
  printf '%s\n' "$WORKSPACE" > "$tx_stage/workspace"
  : > "$tx_stage/prepared"
  mv "$tx_stage" "$STATE/projection-transaction"
  printf '%s\n' "$WORKSPACE" > "$STATE/workspace.tmp"
  mv "$STATE/workspace.tmp" "$STATE/workspace"
  failure_point after_prepared
}

commit_projection_transaction() {
  tx="$STATE/projection-transaction"
  desired_previous=$(cat "$tx/desired-previous-version")
  desired_active=$(cat "$tx/to-version")
  if [ -n "$desired_previous" ]; then printf '%s\n' "$desired_previous" > "$STATE/previous-version.tmp"; mv "$STATE/previous-version.tmp" "$STATE/previous-version"; else rm -f "$STATE/previous-version"; fi
  failure_point after_previous_pointer
  printf '%s\n' "$desired_active" > "$STATE/active-version.tmp"
  mv "$STATE/active-version.tmp" "$STATE/active-version"
  failure_point after_active_pointer
  rm -rf "$tx"
  projection_lock_release
}

write_managed_claude_block() (
  release_root=$1
  output=$2
  cat > "$output" <<EOF
<!-- MAESTRO SCRIPT MANAGED BEGIN -->
## Maestro managed projection

Active managed-content version: $(basename "$release_root"). Read .maestro-script/capabilities.json before capability claims. Use the installed skills under .claude/skills and the five managed specialists under .claude/agents. Managed Atlas starts at $release_root/payload/atlas/managed/index.md; managed memory/profile policies remain under $release_root/payload. Claude script hooks are active through .claude/settings.local.json. agent-route-lite provides bounded metadata-only sequence assurance; authenticated native receipts, external-mutation challenges and native signed route authority remain unavailable. For status, doctor or rollback, Claude may invoke this installed script internally: $release_root/install.sh.
<!-- MAESTRO SCRIPT MANAGED END -->
EOF
)

render_projected_claude() (
  base=$1
  block=$2
  perl -0777 -e '
    use strict; use warnings;
    my ($base_path,$block_path)=@ARGV;
    local $/;
    my $body="";
    if (-f $base_path) { open my $base,"<",$base_path or die $!; binmode $base; $body=<$base>; close $base; }
    open my $block,"<",$block_path or die $!; binmode $block; my $managed=<$block>; close $block;
    $managed =~ s/\r?\n\z//;
    my $begin="<!-- MAESTRO SCRIPT MANAGED BEGIN -->";
    my $end="<!-- MAESTRO SCRIPT MANAGED END -->";
    my $start=index($body,$begin);
    if ($start>=0) {
      my $finish=index($body,$end,$start);
      die "managed block end missing" if $finish<0;
      substr($body,$start,$finish+length($end)-$start,$managed);
    } else {
      $body .= "\n" if length($body) && substr($body,-1) ne "\n";
      $body .= $managed . "\n";
    }
    binmode STDOUT; print $body;
  ' "$base" "$block"
)

write_projection_receipt() {
  release_root=$1
  destination=$2
  runtime_digest=$(shasum -a 256 "$release_root/runtime-inventory.sha256" | awk '{print $1}')
  settings_digest=$(shasum -a 256 "$WORKSPACE/.claude/settings.local.json" | awk '{print $1}')
  block_receipt="$WORKSPACE/.maestro-script/claude-block-receipt.$$"
  write_managed_claude_block "$release_root" "$block_receipt"
  perl -pi -e 'chomp if eof' "$block_receipt"
  block_digest=$(shasum -a 256 "$block_receipt" | awk '{print $1}')
  rm -f "$block_receipt"
  printf '{"schema_version":1,"profile":"%s","version":"%s","runtime_inventory_sha256":"%s","hook_settings_sha256":"%s","managed_claude_block_sha256":"%s","state":"configured_on_disk"}\n' \
    "$PROFILE" "$(basename "$release_root")" "$runtime_digest" "$settings_digest" "$block_digest" > "$destination"
}

write_legacy_managed_claude_block() (
  release_root=$1
  output=$2
  cat > "$output" <<EOF
<!-- MAESTRO SCRIPT MANAGED BEGIN -->
## Maestro managed projection

Active managed-content version: $(basename "$release_root"). Read .maestro-script/capabilities.json before capability claims. Use the installed skills under .claude/skills and the five managed specialists under .claude/agents. Managed Atlas starts at $release_root/payload/atlas/managed/index.md; managed memory/profile policies remain under $release_root/payload. Claude script hooks are active through .claude/settings.local.json. Authenticated native receipts, external-mutation challenges and deterministic specialist-route enforcement remain unavailable. For status, doctor or rollback, Claude may invoke this installed script internally: $release_root/install.sh.
<!-- MAESTRO SCRIPT MANAGED END -->
EOF
)

managed_skill_is_known() {
  target_release=$1
  skill_name=$2
  installed="$WORKSPACE/.claude/skills/$skill_name"
  [ -d "$installed" ] && [ ! -L "$installed" ] || return 1
  target_skill="$target_release/projection/skills/$skill_name"
  differences=$(diff -qr "$target_skill" "$installed" 2>/dev/null || true)
  if [ -z "$differences" ] || printf '%s\n' "$differences" | awk -v prefix="Only in $target_skill" 'index($0,prefix)!=1{exit 1}'; then return 0; fi
  if [ -f "$STATE/active-version" ]; then
    current=$(cat "$STATE/active-version")
    current_skill="$RUNTIME_ROOT/releases/$current/projection/skills/$skill_name"
    if [ -d "$current_skill" ]; then
      differences=$(diff -qr "$current_skill" "$installed" 2>/dev/null || true)
      if [ -z "$differences" ] || printf '%s\n' "$differences" | awk -v prefix="Only in $current_skill" 'index($0,prefix)!=1{exit 1}'; then return 0; fi
    fi
  fi
  return 1
}

managed_agent_is_known() {
  target_release=$1
  agent_name=$2
  installed="$WORKSPACE/.claude/agents/$agent_name"
  [ -f "$installed" ] && [ ! -L "$installed" ] || return 1
  cmp -s "$target_release/projection/agents/$agent_name" "$installed" && return 0
  if [ -f "$STATE/active-version" ]; then
    current=$(cat "$STATE/active-version")
    current_agent="$RUNTIME_ROOT/releases/$current/projection/agents/$agent_name"
    [ -f "$current_agent" ] && cmp -s "$current_agent" "$installed" && return 0
  fi
  return 1
}

project_workspace() {
  release_root=$1
  desired_previous=${2:-}
  [ ! -L "$WORKSPACE" ] && [ ! -L "$WORKSPACE/.claude" ] && [ ! -L "$WORKSPACE/.maestro-script" ] || { echo 'MAESTRO-SCRIPT-PATH: workspace ou estado e link simbolico' >&2; return 1; }
  mkdir -p "$WORKSPACE/.claude/skills" "$WORKSPACE/.claude/agents" "$WORKSPACE/.maestro-script"
  [ ! -L "$WORKSPACE/.claude/skills" ] && [ ! -L "$WORKSPACE/.claude/agents" ] || { echo 'MAESTRO-SCRIPT-PATH: diretorio gerenciado e link simbolico' >&2; return 1; }
  settings="$WORKSPACE/.claude/settings.local.json"
  settings_receipt="$WORKSPACE/.maestro-script/settings-local.sha256"
  hooks_root="$WORKSPACE/.maestro-script/hooks"
  managed_agents="$WORKSPACE/.maestro-script/managed-agents"
  projection_receipt="$WORKSPACE/.maestro-script/projection-receipt.json"
  for candidate in "$WORKSPACE/.maestro-script/managed-skills" "$managed_agents" "$WORKSPACE/.maestro-script/capabilities.json" "$WORKSPACE/.maestro-script/active-version" "$projection_receipt" "$settings" "$settings_receipt" "$hooks_root" "$WORKSPACE/CLAUDE.md"; do
    [ ! -L "$candidate" ] || { echo 'MAESTRO-SCRIPT-PATH: arquivo gerenciado e link simbolico' >&2; return 1; }
  done
  if [ -e "$settings" ]; then
    [ -f "$settings" ] && [ -f "$settings_receipt" ] || { echo 'MAESTRO-SCRIPT-CONFLICT: settings.local.json nao pertence ao Maestro; nada foi sobrescrito' >&2; return 1; }
    expected_settings=$(cat "$settings_receipt")
    actual_settings=$(shasum -a 256 "$settings" | awk '{print $1}')
    if [ "$actual_settings" != "$expected_settings" ]; then
      recognized=0
      cmp -s "$release_root/projection/settings.local.json" "$settings" && recognized=1
      if [ "$recognized" -eq 0 ] && [ -f "$STATE/active-version" ]; then
        current=$(cat "$STATE/active-version")
        current_settings="$RUNTIME_ROOT/releases/$current/projection/settings.local.json"
        [ -f "$current_settings" ] && cmp -s "$current_settings" "$settings" && recognized=1
      fi
      [ "$recognized" -eq 1 ] || { echo 'MAESTRO-SCRIPT-CONFLICT: settings.local.json foi alterado pelo owner; nada foi sobrescrito' >&2; return 1; }
    fi
  fi
  managed_list="$WORKSPACE/.maestro-script/managed-skills"
  if [ -f "$managed_list" ]; then
    while IFS= read -r name; do
      case "$name" in ''|*[!a-z0-9-]*) echo 'MAESTRO-SCRIPT-STATE: skill gerenciada invalida' >&2; return 1;; esac
    done < "$managed_list"
  fi
  if [ -f "$managed_agents" ]; then
    while IFS= read -r name; do
      case "$name" in ''|*[!a-z0-9.-]*|.*|*..*) echo 'MAESTRO-SCRIPT-STATE: agente gerenciado invalido' >&2; return 1;; esac
    done < "$managed_agents"
  fi
  current_release=''
  if [ -f "$STATE/active-version" ]; then
    current_version=$(cat "$STATE/active-version")
    current_release="$RUNTIME_ROOT/releases/$current_version"
    verify_runtime "$current_release"
  fi
  for agent in "$release_root/projection/agents"/*.md; do
    [ -f "$agent" ] || continue
    name=$(basename "$agent")
    [ ! -L "$WORKSPACE/.claude/agents/$name" ] || { echo "MAESTRO-SCRIPT-PATH: agente existente e link simbolico: $name" >&2; return 1; }
    if [ -e "$WORKSPACE/.claude/agents/$name" ]; then
      [ -f "$managed_agents" ] && grep -Fx "$name" "$managed_agents" >/dev/null && managed_agent_is_known "$release_root" "$name" || { echo "MAESTRO-SCRIPT-CONFLICT: agente local existente ou alterado: $name; nada foi sobrescrito" >&2; return 1; }
    fi
  done
  if [ -n "$current_release" ]; then
    for source in "$current_release/projection/skills"/*; do
      [ -d "$source" ] || continue
      name=$(basename "$source")
      [ -d "$release_root/projection/skills/$name" ] && continue
      actual="$WORKSPACE/.claude/skills/$name"
      [ ! -e "$actual" ] || projection_tree_is_known "$source" "$actual" || { echo "MAESTRO-SCRIPT-CONFLICT: skill gerenciada removida foi alterada: $name; nada foi sobrescrito" >&2; return 1; }
    done
    for source in "$current_release/projection/agents"/*.md; do
      [ -f "$source" ] || continue
      name=$(basename "$source")
      [ -f "$release_root/projection/agents/$name" ] && continue
      actual="$WORKSPACE/.claude/agents/$name"
      [ ! -e "$actual" ] || { [ -f "$actual" ] && [ ! -L "$actual" ] && cmp -s "$source" "$actual"; } || { echo "MAESTRO-SCRIPT-CONFLICT: agente gerenciado removido foi alterado: $name; nada foi sobrescrito" >&2; return 1; }
    done
  fi
  base="$WORKSPACE/CLAUDE.md"
  if [ -f "$base" ]; then
    awk '
      /<!-- MAESTRO SCRIPT MANAGED BEGIN -->/ { begin++; if (open) bad=1; open=1 }
      /<!-- MAESTRO SCRIPT MANAGED END -->/ { end++; if (!open) bad=1; open=0 }
      END { if (bad || open || begin != end || begin > 1) exit 1 }
    ' "$base" || { echo 'MAESTRO-SCRIPT-STATE: bloco CLAUDE gerenciado invalido' >&2; return 1; }
    if grep -F '<!-- MAESTRO SCRIPT MANAGED BEGIN -->' "$base" >/dev/null; then
      actual_block="$WORKSPACE/.maestro-script/claude-block-preflight.$$"
      current_block="$WORKSPACE/.maestro-script/claude-current-preflight.$$"
      target_block="$WORKSPACE/.maestro-script/claude-target-preflight.$$"
      perl -0777 -e 'local $/; my $body=<>; my $b="<!-- MAESTRO SCRIPT MANAGED BEGIN -->"; my $e="<!-- MAESTRO SCRIPT MANAGED END -->"; my $s=index($body,$b); my $f=index($body,$e,$s); print substr($body,$s,$f+length($e)-$s) if $s>=0 && $f>=0' "$base" > "$actual_block"
      write_managed_claude_block "$release_root" "$target_block"
      perl -pi -e 'chomp if eof' "$target_block"
      known_block=0
      cmp -s "$actual_block" "$target_block" && known_block=1
      if [ "$known_block" -eq 0 ] && [ -n "$current_release" ]; then
        write_managed_claude_block "$current_release" "$current_block"
        perl -pi -e 'chomp if eof' "$current_block"
        cmp -s "$actual_block" "$current_block" && known_block=1
      fi
      if [ "$known_block" -eq 0 ] && [ -n "$current_release" ] && [ -f "$projection_receipt" ]; then
        receipt_version=$(plutil -extract version raw -o - "$projection_receipt" 2>/dev/null || true)
        receipt_block_digest=$(plutil -extract managed_claude_block_sha256 raw -o - "$projection_receipt" 2>/dev/null || true)
        case "$receipt_block_digest" in *[!a-f0-9]*|'') ;; *)
          if [ "${#receipt_block_digest}" -eq 64 ] && [ "$receipt_version" = "$current_version" ] && [ "$(shasum -a 256 "$actual_block" | awk '{print $1}')" = "$receipt_block_digest" ]; then known_block=1; fi
          ;;
        esac
      fi
      if [ "$known_block" -eq 0 ] && [ -n "$current_release" ]; then
        write_legacy_managed_claude_block "$current_release" "$current_block"
        perl -pi -e 'chomp if eof' "$current_block"
        cmp -s "$actual_block" "$current_block" && known_block=1
      fi
      rm -f "$actual_block" "$current_block" "$target_block"
      [ "$known_block" -eq 1 ] || { echo 'MAESTRO-SCRIPT-CONFLICT: bloco CLAUDE foi alterado pelo owner; nada foi sobrescrito' >&2; return 1; }
    fi
  fi
  skill_stage="$WORKSPACE/.maestro-script/skills-staging-$$"
  rm -rf "$skill_stage"
  mkdir -p "$skill_stage"
  : > "$skill_stage/names"
  for skill in "$release_root/projection/skills"/*; do
    [ -d "$skill" ] || continue
    name=$(basename "$skill")
    [ ! -L "$WORKSPACE/.claude/skills/$name" ] || { echo "MAESTRO-SCRIPT-PATH: skill existente e link simbolico: $name" >&2; rm -rf "$skill_stage"; return 1; }
    if [ -e "$WORKSPACE/.claude/skills/$name" ]; then
      [ -f "$managed_list" ] && grep -Fx "$name" "$managed_list" >/dev/null && managed_skill_is_known "$release_root" "$name" || { echo "MAESTRO-SCRIPT-CONFLICT: skill local existente ou alterada: $name; nada foi sobrescrito" >&2; rm -rf "$skill_stage"; return 1; }
    fi
    cp -R "$skill" "$skill_stage/$name"
    printf '%s\n' "$name" >> "$skill_stage/names"
  done
  begin_projection_transaction "$release_root" "$desired_previous"
  rm -f "$projection_receipt"
  failure_point after_receipt_invalidated
  if [ -n "$current_release" ]; then for source in "$current_release/projection/skills"/*; do [ ! -d "$source" ] || rm -rf "$WORKSPACE/.claude/skills/$(basename "$source")"; done; fi
  for skill in "$skill_stage"/*; do
    [ -d "$skill" ] || continue
    mv "$skill" "$WORKSPACE/.claude/skills/$(basename "$skill")"
  done
  mv "$skill_stage/names" "$managed_list"
  rm -rf "$skill_stage"
  failure_point after_skills
  agent_stage="$WORKSPACE/.maestro-script/agents-staging-$$"
  rm -rf "$agent_stage"
  mkdir -p "$agent_stage"
  : > "$agent_stage/names"
  for agent in "$release_root/projection/agents"/*.md; do
    [ -f "$agent" ] || continue
    name=$(basename "$agent")
    [ ! -L "$WORKSPACE/.claude/agents/$name" ] || { echo "MAESTRO-SCRIPT-PATH: agente existente e link simbolico: $name" >&2; rm -rf "$agent_stage"; return 1; }
    if [ -e "$WORKSPACE/.claude/agents/$name" ]; then
      [ -f "$managed_agents" ] && grep -Fx "$name" "$managed_agents" >/dev/null && managed_agent_is_known "$release_root" "$name" || { echo "MAESTRO-SCRIPT-CONFLICT: agente local existente ou alterado: $name; nada foi sobrescrito" >&2; rm -rf "$agent_stage"; return 1; }
    fi
    cp "$agent" "$agent_stage/$name"
    printf '%s\n' "$name" >> "$agent_stage/names"
  done
  if [ -n "$current_release" ]; then for source in "$current_release/projection/agents"/*.md; do [ ! -f "$source" ] || rm -f "$WORKSPACE/.claude/agents/$(basename "$source")"; done; fi
  for agent in "$agent_stage"/*.md; do
    [ -f "$agent" ] || continue
    mv "$agent" "$WORKSPACE/.claude/agents/$(basename "$agent")"
  done
  mv "$agent_stage/names" "$managed_agents"
  rm -rf "$agent_stage"
  failure_point after_agents
  hooks_stage="$WORKSPACE/.maestro-script/hooks-staging-$$"
  rm -rf "$hooks_stage"
  mkdir -p "$hooks_stage"
  cp "$release_root/maestro-hook.sh" "$hooks_stage/maestro-hook.sh"
  chmod 700 "$hooks_stage/maestro-hook.sh"
  rm -rf "$hooks_root"
  mv "$hooks_stage" "$hooks_root"
  failure_point after_hooks
  cp "$release_root/projection/settings.local.json" "$settings.tmp"
  mv "$settings.tmp" "$settings"
  shasum -a 256 "$settings" | awk '{print $1}' > "$settings_receipt.tmp"
  mv "$settings_receipt.tmp" "$settings_receipt"
  failure_point after_settings
  cp "$release_root/capabilities.json" "$WORKSPACE/.maestro-script/capabilities.json"
  printf '%s\n' "$(basename "$release_root")" > "$WORKSPACE/.maestro-script/active-version"
  temp="$WORKSPACE/.maestro-script/CLAUDE.md.tmp"
  render_projected_claude "$base" "$STATE/projection-transaction/claude-target-block" > "$temp"
  projected_hash=$(shasum -a 256 "$temp" | awk '{print $1}')
  [ "$projected_hash" = "$(cat "$STATE/projection-transaction/claude-target.sha256")" ] || { rm -f "$temp"; echo 'MAESTRO-SCRIPT-CONFLICT: CLAUDE.md mudou durante a projecao; nada foi sobrescrito' >&2; return 1; }
  mv "$temp" "$base"
  failure_point after_claude
  write_projection_receipt "$release_root" "$projection_receipt.tmp"
  mv "$projection_receipt.tmp" "$projection_receipt"
  failure_point after_receipt
  commit_projection_transaction
}

rollback_runtime() {
  [ -f "$STATE/active-version" ] && [ -f "$STATE/previous-version" ] || { echo 'MAESTRO-SCRIPT-ROLLBACK: nenhuma versao anterior' >&2; exit 5; }
  active=$(cat "$STATE/active-version")
  previous=$(cat "$STATE/previous-version")
  [ -d "$RUNTIME_ROOT/releases/$previous" ] || { echo 'MAESTRO-SCRIPT-ROLLBACK: versao anterior ausente' >&2; exit 5; }
  verify_runtime "$RUNTIME_ROOT/releases/$previous"
  project_workspace "$RUNTIME_ROOT/releases/$previous" "$active"
  echo "Maestro script-only voltou para $previous"
}

doctor_runtime() {
  [ ! -d "$STATE/projection-transaction" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: projection_state=repair_required; execute install novamente para recuperar' >&2; return 1; }
  [ -f "$STATE/active-version" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: runtime incompleto' >&2; return 1; }
  active=$(cat "$STATE/active-version")
  release_root="$RUNTIME_ROOT/releases/$active"
  [ -d "$release_root" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: runtime incompleto' >&2; return 1; }
  verify_runtime "$release_root"
  [ -f "$STATE/workspace" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: workspace ausente' >&2; return 1; }
  workspace=$(cat "$STATE/workspace")
  [ -d "$workspace" ] && [ ! -L "$workspace" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: workspace inseguro ou ausente' >&2; return 1; }
  [ -f "$workspace/.maestro-script/active-version" ] && [ "$(cat "$workspace/.maestro-script/active-version")" = "$active" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: versao global e projecao do workspace divergem' >&2; return 1; }
  settings="$workspace/.claude/settings.local.json"
  receipt="$workspace/.maestro-script/settings-local.sha256"
  hook="$workspace/.maestro-script/hooks/maestro-hook.sh"
  [ -f "$settings" ] && [ -f "$receipt" ] && [ -f "$hook" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: hooks ausentes; execute install novamente para reparar' >&2; return 1; }
  expected=$(cat "$receipt")
  actual=$(shasum -a 256 "$settings" | awk '{print $1}')
  [ "$actual" = "$expected" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: settings.local.json foi alterado; nada foi sobrescrito' >&2; return 1; }
  expected_hook=$(shasum -a 256 "$release_root/maestro-hook.sh" | awk '{print $1}')
  actual_hook=$(shasum -a 256 "$hook" | awk '{print $1}')
  [ "$actual_hook" = "$expected_hook" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: handler de hooks foi alterado' >&2; return 1; }
  cmp -s "$release_root/capabilities.json" "$workspace/.maestro-script/capabilities.json" || { echo 'MAESTRO-SCRIPT-DOCTOR: matriz de capacidades ausente ou alterada' >&2; return 1; }
  scratch=$(mktemp -d "${TMPDIR:-/tmp}/maestro-script-doctor.XXXXXX") || { echo 'MAESTRO-SCRIPT-DOCTOR: nao foi possivel criar diagnostico temporario' >&2; return 1; }
  trap 'rm -rf "$scratch"' EXIT HUP INT TERM
  : > "$scratch/expected-skills"
  for skill in "$release_root/projection/skills"/*; do [ ! -d "$skill" ] || basename "$skill" >> "$scratch/expected-skills"; done
  sort "$scratch/expected-skills" > "$scratch/expected-skills.sorted"
  [ -f "$workspace/.maestro-script/managed-skills" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: lista de skills gerenciadas ausente' >&2; return 1; }
  sort "$workspace/.maestro-script/managed-skills" > "$scratch/actual-skills.sorted"
  cmp -s "$scratch/expected-skills.sorted" "$scratch/actual-skills.sorted" || { echo 'MAESTRO-SCRIPT-DOCTOR: lista de skills gerenciadas diverge' >&2; return 1; }
  while IFS= read -r skill_name; do
    [ -n "$skill_name" ] || continue
    [ -d "$workspace/.claude/skills/$skill_name" ] && diff -qr "$release_root/projection/skills/$skill_name" "$workspace/.claude/skills/$skill_name" >/dev/null || { echo "MAESTRO-SCRIPT-DOCTOR: skill gerenciada ausente ou alterada: $skill_name" >&2; return 1; }
  done < "$scratch/expected-skills.sorted"
  : > "$scratch/expected-agents"
  for agent_path in "$release_root/projection/agents"/*.md; do [ ! -f "$agent_path" ] || basename "$agent_path" >> "$scratch/expected-agents"; done
  sort "$scratch/expected-agents" > "$scratch/expected-agents.sorted"
  [ -f "$workspace/.maestro-script/managed-agents" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: lista de agentes gerenciados ausente' >&2; return 1; }
  sort "$workspace/.maestro-script/managed-agents" > "$scratch/actual-agents.sorted"
  cmp -s "$scratch/expected-agents.sorted" "$scratch/actual-agents.sorted" || { echo 'MAESTRO-SCRIPT-DOCTOR: lista de agentes gerenciados diverge' >&2; return 1; }
  for event in SessionStart UserPromptSubmit PreToolUse PostToolUse Stop SubagentStart SubagentStop; do
    grep -F "\"$event\"" "$settings" >/dev/null || { echo "MAESTRO-SCRIPT-DOCTOR: hook ausente: $event" >&2; return 1; }
  done
  for agent in client-account-agent.md case-agent.md walter.md darwin.md pa-expert.md; do
    [ -f "$workspace/.claude/agents/$agent" ] && cmp -s "$release_root/projection/agents/$agent" "$workspace/.claude/agents/$agent" || { echo "MAESTRO-SCRIPT-DOCTOR: agente gerenciado ausente ou alterado: $agent" >&2; return 1; }
  done
  write_managed_claude_block "$release_root" "$scratch/expected-claude-block"
  [ -f "$workspace/CLAUDE.md" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: orientacao gerenciada ausente' >&2; return 1; }
  awk '/<!-- MAESTRO SCRIPT MANAGED BEGIN -->/{capture=1} capture{print} /<!-- MAESTRO SCRIPT MANAGED END -->/{capture=0}' "$workspace/CLAUDE.md" > "$scratch/actual-claude-block"
  cmp -s "$scratch/expected-claude-block" "$scratch/actual-claude-block" || { echo 'MAESTRO-SCRIPT-DOCTOR: bloco CLAUDE gerenciado ausente ou alterado' >&2; return 1; }
  [ -f "$workspace/.maestro-script/projection-receipt.json" ] || { echo 'MAESTRO-SCRIPT-DOCTOR: recibo de projecao ausente; execute install novamente para reparar' >&2; return 1; }
  write_projection_receipt "$release_root" "$scratch/expected-projection-receipt"
  cmp -s "$scratch/expected-projection-receipt" "$workspace/.maestro-script/projection-receipt.json" || { echo 'MAESTRO-SCRIPT-DOCTOR: recibo de projecao diverge; execute install novamente para reparar' >&2; return 1; }
  echo 'Maestro script-only: managed projection, seven Claude hooks and five specialists configured and intact on disk; Claude runtime observation pending; native CLI unavailable'
}

case "$ACTION" in
  install)
    [ ! -d "$STATE/projection-transaction" ] || recover_projection_transaction
    target="$RUNTIME_ROOT/releases/$VERSION"
    if [ -f "$STATE/active-version" ] && [ "$(cat "$STATE/active-version")" = "$VERSION" ] && [ -d "$target" ]; then
      verify_runtime "$target"
      prepare_workspace
      previous=''
      [ ! -f "$STATE/previous-version" ] || previous=$(cat "$STATE/previous-version")
      project_workspace "$target" "$previous"
      echo "Maestro script-only $VERSION ja estava preparado"
      echo "MAESTRO-SCRIPT-WORKSPACE: $WORKSPACE"
      echo 'Abra essa pasta em uma nova sessao do Claude Code para carregar os hooks.'
      reveal_workspace
      exit 0
    fi
    verify_package
    prepare_workspace
    active=''
    [ ! -f "$STATE/active-version" ] || active=$(cat "$STATE/active-version")
    if [ -n "$active" ] && [ "$active" != "$VERSION" ] && ! version_greater_than "$VERSION" "$active"; then
      echo 'MAESTRO-SCRIPT-VERSION: install aceita somente uma versao mais nova; use rollback para voltar' >&2
      exit 7
    fi
    mkdir -p "$RUNTIME_ROOT/releases" "$STATE"
    if [ -d "$target" ]; then
      cmp -s "$ROOT/runtime-inventory.sha256" "$target/runtime-inventory.sha256" || { echo 'MAESTRO-SCRIPT-RUNTIME: versao existente tem identidade diferente' >&2; exit 6; }
      verify_runtime "$target"
    else
      staging="$RUNTIME_ROOT/.staging-$VERSION-$$"
      trap 'rm -rf "$staging"' EXIT HUP INT TERM
      mkdir -p "$staging"
      cp -R "$ROOT/payload" "$staging/payload"
      cp -R "$ROOT/projection" "$staging/projection"
      cp "$ROOT/capabilities.json" "$staging/capabilities.json"
      cp "$ROOT/install.sh" "$staging/install.sh"
      cp "$ROOT/maestro-hook.sh" "$staging/maestro-hook.sh"
      cp "$ROOT/runtime-inventory.sha256" "$staging/runtime-inventory.sha256"
      verify_runtime "$staging"
      mv "$staging" "$target"
      trap - EXIT HUP INT TERM
    fi
    desired_previous=''
    if [ -n "$active" ] && [ "$active" != "$VERSION" ]; then desired_previous=$active; elif [ -f "$STATE/previous-version" ]; then desired_previous=$(cat "$STATE/previous-version"); fi
    project_workspace "$target" "$desired_previous"
    echo "Maestro script-only $VERSION preparado"
    echo "MAESTRO-SCRIPT-WORKSPACE: $WORKSPACE"
    echo 'Abra essa pasta em uma nova sessao do Claude Code para carregar os hooks.'
    reveal_workspace
    ;;
  rollback) [ ! -d "$STATE/projection-transaction" ] || recover_projection_transaction; rollback_runtime ;;
  status) active=''; previous=''; projection_state='configured_on_disk'; [ ! -f "$STATE/active-version" ] || active=$(cat "$STATE/active-version"); [ ! -f "$STATE/previous-version" ] || previous=$(cat "$STATE/previous-version"); [ ! -d "$STATE/projection-transaction" ] || projection_state='repair_required'; printf '{"schema_version":1,"profile":"macos-shell-local-beta","active_version":"%s","previous_version":"%s","projection_state":"%s","native_cli":"unavailable"}\n' "$active" "$previous" "$projection_state" ;;
  doctor) doctor_runtime ;;
  *) echo 'uso: install.sh <install|rollback|status|doctor> [--workspace DIR] [--reveal-workspace]' >&2; exit 2 ;;
esac
`, "@@VERSION@@", version))
}

func scriptPortableCMDStart() []byte {
	return []byte("@echo off\r\nsetlocal\r\necho O Maestro instalara somente scripts e conteudo gerenciado no seu perfil, sem Go ou administrador.\r\nchoice /C SN /N /M \"Pressione S para continuar ou N para cancelar: \"\r\nif errorlevel 2 (echo Preparacao cancelada; nada foi alterado.& pause& exit /b 0)\r\npowershell.exe -NoLogo -NoProfile -NonInteractive -File \"%~dp0Install-Maestro.ps1\" -Action install -RevealWorkspace\r\nset CODE=%ERRORLEVEL%\r\nif not \"%CODE%\"==\"0\" (echo Nao foi possivel preparar o Maestro. Nada do seu trabalho foi apagado.& pause& exit /b %CODE%)\r\necho Maestro preparado. O Explorer mostrou a pasta permanente; abra maestro-os no Claude Code e diga: Quero comecar.\r\nexit /b 0\r\n")
}

func scriptPortablePowerShellInstaller(version string) []byte {
	template := `param(
  [ValidateSet('install','rollback','status','doctor')][string]$Action = 'install',
  [string]$Workspace = '',
  [switch]$RevealWorkspace
)
$ErrorActionPreference = 'Stop'
$Version = '@@VERSION@@'
$Profile = 'windows-powershell-local-beta'
$Root = $PSScriptRoot
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'MAESTRO-SCRIPT-OS: este pacote requer Windows' }
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'MAESTRO-SCRIPT-ADMIN: execute como usuario normal, nunca elevado' }
$runtimeRoot = if ($env:MAESTRO_SCRIPT_HOME) { $env:MAESTRO_SCRIPT_HOME } else { Join-Path $env:LOCALAPPDATA 'Maestro\script-runtime' }
$stateRoot = Join-Path $runtimeRoot 'state'
if (-not $Workspace) {
  $workspaceState = Join-Path $stateRoot 'workspace'
  $workspaceHome = if ($env:MAESTRO_WORKSPACE_HOME) { $env:MAESTRO_WORKSPACE_HOME } else { Join-Path $env:USERPROFILE 'Maestro' }
  $Workspace = if (Test-Path -LiteralPath $workspaceState -PathType Leaf) { (Get-Content -LiteralPath $workspaceState -Raw).Trim() } else { Join-Path $workspaceHome 'maestro-os' }
}

function Initialize-StableWorkspace {
  if (-not (Test-Path -LiteralPath $Workspace)) {
    New-Item -ItemType Directory -Path $Workspace -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $Root 'maestro-os\README.md') -Destination (Join-Path $Workspace 'README.md')
  }
  if (-not (Test-Path -LiteralPath $Workspace -PathType Container) -or ((Get-Item -LiteralPath $Workspace -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-SCRIPT-WORKSPACE: destino permanente inseguro ou invalido' }
}

function Show-StableWorkspace {
  if (-not $RevealWorkspace) { return }
  try {
    Invoke-Item -LiteralPath $Workspace -ErrorAction Stop
  } catch {
    [Console]::Error.WriteLine("MAESTRO-SCRIPT-REVEAL: nao consegui mostrar a pasta; abra manualmente: $Workspace")
  }
}

function Test-PackageIntegrity {
  $inventory = Join-Path $Root 'inventory.sha256'
  if (-not (Test-Path -LiteralPath $inventory -PathType Leaf)) { throw 'MAESTRO-SCRIPT-INTEGRITY: inventario ausente' }
  if (Get-ChildItem -LiteralPath $Root -Force -Recurse | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint } | Select-Object -First 1) { throw 'MAESTRO-SCRIPT-INTEGRITY: reparse point no pacote' }
  $declared = @{}
  foreach ($line in Get-Content -LiteralPath $inventory) {
    if ($line -notmatch '^([a-f0-9]{64})  (.+)$') { throw 'MAESTRO-SCRIPT-INTEGRITY: linha invalida' }
    $relative = $Matches[2]
    if ([IO.Path]::IsPathRooted($relative) -or $relative.Contains('..') -or $relative.Contains('\')) { throw 'MAESTRO-SCRIPT-INTEGRITY: caminho invalido' }
    $path = Join-Path $Root ($relative.Replace('/', [IO.Path]::DirectorySeparatorChar))
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "MAESTRO-SCRIPT-INTEGRITY: arquivo ausente: $relative" }
    $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $Matches[1]) { throw "MAESTRO-SCRIPT-INTEGRITY: arquivo alterado: $relative" }
    $declared[$relative] = $true
  }
  foreach ($file in Get-ChildItem -LiteralPath $Root -File -Recurse) {
    $relative = $file.FullName.Substring($Root.Length).TrimStart('\').Replace('\','/')
    if ($relative -eq 'inventory.sha256') { continue }
    if (-not $declared.ContainsKey($relative)) { throw "MAESTRO-SCRIPT-INTEGRITY: arquivo nao declarado: $relative" }
  }
}

function Test-RuntimeIntegrity([string]$base) {
  $inventory = Join-Path $base 'runtime-inventory.sha256'
  if (-not (Test-Path -LiteralPath $inventory -PathType Leaf)) { throw 'MAESTRO-SCRIPT-RUNTIME: inventario ausente' }
  if (Get-ChildItem -LiteralPath $base -Force -Recurse | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint } | Select-Object -First 1) { throw 'MAESTRO-SCRIPT-RUNTIME: reparse point no runtime' }
  $declared = @{}
  foreach ($line in Get-Content -LiteralPath $inventory) {
    if ($line -notmatch '^([a-f0-9]{64})  (.+)$') { throw 'MAESTRO-SCRIPT-RUNTIME: linha invalida' }
    $relative = $Matches[2]
    if ([IO.Path]::IsPathRooted($relative) -or $relative.Contains('..') -or $relative.Contains('\')) { throw 'MAESTRO-SCRIPT-RUNTIME: caminho invalido' }
    $path = Join-Path $base ($relative.Replace('/', [IO.Path]::DirectorySeparatorChar))
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "MAESTRO-SCRIPT-RUNTIME: arquivo ausente: $relative" }
    $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $Matches[1]) { throw "MAESTRO-SCRIPT-RUNTIME: arquivo alterado: $relative" }
    $declared[$relative] = $true
  }
  foreach ($file in Get-ChildItem -LiteralPath $base -File -Recurse) {
    $relative = $file.FullName.Substring($base.Length).TrimStart('\').Replace('\','/')
    if ($relative -eq 'runtime-inventory.sha256') { continue }
    if (-not $declared.ContainsKey($relative)) { throw "MAESTRO-SCRIPT-RUNTIME: arquivo nao declarado: $relative" }
  }
}

function Get-ManagedClaudeBlock([string]$releaseRoot) {
  $releaseName = Split-Path $releaseRoot -Leaf
  return "<!-- MAESTRO SCRIPT MANAGED BEGIN -->@@BT@@n## Maestro managed projection@@BT@@n@@BT@@nActive managed-content version: $releaseName. Read .maestro-script/capabilities.json before capability claims. Use the installed skills under .claude/skills and the five managed specialists under .claude/agents. Managed Atlas starts at $(Join-Path $releaseRoot 'payload\atlas\managed\index.md'); managed memory/profile policies remain under $(Join-Path $releaseRoot 'payload'). Claude script hooks are active through .claude/settings.local.json. agent-route-lite provides bounded metadata-only sequence assurance; authenticated native receipts, external-mutation challenges and native signed route authority remain unavailable. For status, doctor or rollback, Claude may invoke this installed script internally: $(Join-Path $releaseRoot 'Install-Maestro.ps1').@@BT@@n<!-- MAESTRO SCRIPT MANAGED END -->"
}

function Get-LegacyManagedClaudeBlock([string]$releaseRoot) {
  $releaseName = Split-Path $releaseRoot -Leaf
  return "<!-- MAESTRO SCRIPT MANAGED BEGIN -->@@BT@@n## Maestro managed projection@@BT@@n@@BT@@nActive managed-content version: $releaseName. Read .maestro-script/capabilities.json before capability claims. Use the installed skills under .claude/skills and the five managed specialists under .claude/agents. Managed Atlas starts at $(Join-Path $releaseRoot 'payload\atlas\managed\index.md'); managed memory/profile policies remain under $(Join-Path $releaseRoot 'payload'). Claude script hooks are active through .claude/settings.local.json. Authenticated native receipts, external-mutation challenges and deterministic specialist-route enforcement remain unavailable. For status, doctor or rollback, Claude may invoke this installed script internally: $(Join-Path $releaseRoot 'Install-Maestro.ps1').@@BT@@n<!-- MAESTRO SCRIPT MANAGED END -->"
}

function Get-ProjectedClaudeBody([string]$body, [string]$block) {
  $begin = '<!-- MAESTRO SCRIPT MANAGED BEGIN -->'
  $end = '<!-- MAESTRO SCRIPT MANAGED END -->'
  $start = $body.IndexOf($begin, [StringComparison]::Ordinal)
  if ($start -ge 0) {
    $finish = $body.IndexOf($end, $start, [StringComparison]::Ordinal)
    if ($finish -lt 0) { throw 'MAESTRO-SCRIPT-STATE: bloco CLAUDE gerenciado invalido' }
    return $body.Substring(0, $start) + $block + $body.Substring($finish + $end.Length)
  }
  $separator = if ($body -and -not $body.EndsWith("@@BT@@n")) { "@@BT@@n" } else { '' }
  return $body + $separator + $block + "@@BT@@n"
}

function Get-TextSHA256([string]$body) {
  $sha = [Security.Cryptography.SHA256]::Create()
  try { return ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($body)))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
}

$script:StrictUTF8 = New-Object System.Text.UTF8Encoding -ArgumentList @($false, $true)

function Read-StrictUTF8([string]$path) {
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { return '' }
  $bytes = [IO.File]::ReadAllBytes($path)
  if (($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) -or
      ($bytes.Length -ge 2 -and (($bytes[0] -eq 0xFF -and $bytes[1] -eq 0xFE) -or ($bytes[0] -eq 0xFE -and $bytes[1] -eq 0xFF)))) {
    throw 'MAESTRO-SCRIPT-ENCODING: CLAUDE.md precisa ser UTF-8 sem BOM; nada foi sobrescrito'
  }
  try { return $script:StrictUTF8.GetString($bytes) } catch { throw 'MAESTRO-SCRIPT-ENCODING: CLAUDE.md precisa ser UTF-8 sem BOM; nada foi sobrescrito' }
}

function Write-StrictUTF8([string]$path, [string]$body) {
  [IO.File]::WriteAllText($path, $body, $script:StrictUTF8)
}

function Get-ProjectionReceiptBody([string]$releaseRoot) {
  $runtimeDigest = (Get-FileHash -LiteralPath (Join-Path $releaseRoot 'runtime-inventory.sha256') -Algorithm SHA256).Hash.ToLowerInvariant()
  $settingsDigest = (Get-FileHash -LiteralPath (Join-Path $Workspace '.claude\settings.local.json') -Algorithm SHA256).Hash.ToLowerInvariant()
  $blockDigest = Get-TextSHA256 (Get-ManagedClaudeBlock $releaseRoot)
  return ([ordered]@{schema_version=1;profile=$Profile;version=(Split-Path $releaseRoot -Leaf);runtime_inventory_sha256=$runtimeDigest;hook_settings_sha256=$settingsDigest;managed_claude_block_sha256=$blockDigest;state='configured_on_disk'} | ConvertTo-Json -Compress)
}

function Test-ProjectionTree([string]$expectedRoot, [string]$actualRoot, [string]$label) {
  if (-not (Test-Path -LiteralPath $actualRoot -PathType Container)) { throw "MAESTRO-SCRIPT-DOCTOR: $label ausente" }
  if (Get-ChildItem -LiteralPath $actualRoot -Force -Recurse | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint } | Select-Object -First 1) { throw "MAESTRO-SCRIPT-DOCTOR: $label contem reparse point" }
  $expected = @{}
  foreach ($file in Get-ChildItem -LiteralPath $expectedRoot -File -Recurse) {
    $relative = $file.FullName.Substring($expectedRoot.Length).TrimStart('\')
    $expected[$relative] = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
  }
  $actual = @{}
  foreach ($file in Get-ChildItem -LiteralPath $actualRoot -File -Recurse) {
    $relative = $file.FullName.Substring($actualRoot.Length).TrimStart('\')
    $actual[$relative] = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
  }
  if ($expected.Count -ne $actual.Count) { throw "MAESTRO-SCRIPT-DOCTOR: $label diverge" }
  foreach ($relative in $expected.Keys) {
    if (-not $actual.ContainsKey($relative) -or $actual[$relative] -ne $expected[$relative]) { throw "MAESTRO-SCRIPT-DOCTOR: $label diverge" }
  }
}

function Test-ProjectionTreeIsKnown([string]$expectedRoot, [string]$actualRoot) {
  if (-not (Test-Path -LiteralPath $actualRoot -PathType Container)) { return $false }
  if (Get-ChildItem -LiteralPath $actualRoot -Force -Recurse | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint } | Select-Object -First 1) { return $false }
  $expected = @{}
  foreach ($file in Get-ChildItem -LiteralPath $expectedRoot -File -Recurse) {
    $relative = $file.FullName.Substring($expectedRoot.Length).TrimStart('\')
    $expected[$relative] = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
  }
  foreach ($file in Get-ChildItem -LiteralPath $actualRoot -File -Recurse) {
    $relative = $file.FullName.Substring($actualRoot.Length).TrimStart('\')
    if (-not $expected.ContainsKey($relative) -or $expected[$relative] -ne (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash) { return $false }
  }
  return $true
}

function Test-ManagedAgentIsKnown([string]$releaseRoot, [string]$agentName, [string]$installed) {
  $target = Join-Path $releaseRoot "projection\agents\$agentName"
  if ((Test-Path -LiteralPath $target -PathType Leaf) -and (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -eq (Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash) { return $true }
  $activePath = Join-Path $stateRoot 'active-version'
  if (Test-Path -LiteralPath $activePath -PathType Leaf) {
    $current = ([IO.File]::ReadAllText($activePath)).Trim()
    $currentAgent = Join-Path $runtimeRoot "releases\$current\projection\agents\$agentName"
    if ((Test-Path -LiteralPath $currentAgent -PathType Leaf) -and (Get-FileHash -LiteralPath $currentAgent -Algorithm SHA256).Hash -eq (Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash) { return $true }
  }
  return $false
}

function Invoke-FailurePoint([string]$Phase) {}

function Enter-ProjectionLock {
  $lock = Join-Path $stateRoot 'projection-lock'
  $reclaim = Join-Path $stateRoot 'projection-lock-reclaim'
  $script:ProjectionLockToken = "${PID}:$([DateTime]::UtcNow.Ticks)"
  $candidate = Join-Path $stateRoot ".projection-lock-$PID"
  [IO.File]::WriteAllText($candidate, $script:ProjectionLockToken + "@@BT@@n")
  try {
    [IO.File]::Move($candidate, $lock)
    return
  } catch {
    Remove-Item -LiteralPath $candidate -Force -ErrorAction SilentlyContinue
  }
  if (-not (Test-Path -LiteralPath $lock -PathType Leaf) -or ((Get-Item -LiteralPath $lock -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-SCRIPT-BUSY: lock de projecao incompleto; nada foi alterado' }
  $observed = ([IO.File]::ReadAllText($lock)).Trim()
  if ($observed -notmatch '^(\d+):(\d+)$') { throw 'MAESTRO-SCRIPT-BUSY: lock de projecao invalido; nada foi alterado' }
  $owner = [int]$Matches[1]
  if (Get-Process -Id $owner -ErrorAction SilentlyContinue) { throw 'MAESTRO-SCRIPT-BUSY: outra projecao esta em andamento; nada foi alterado' }
  $reclaimToken = "${PID}:$([DateTime]::UtcNow.Ticks)"
  $reclaimCandidate = Join-Path $stateRoot ".projection-lock-reclaim-$PID"
  [IO.File]::WriteAllText($reclaimCandidate, $reclaimToken + "@@BT@@n")
  try {
    [IO.File]::Move($reclaimCandidate, $reclaim)
  } catch {
    Remove-Item -LiteralPath $reclaimCandidate -Force -ErrorAction SilentlyContinue
    throw 'MAESTRO-SCRIPT-BUSY: recuperacao de lock em andamento; nada foi alterado'
  }
  try {
    if (-not (Test-Path -LiteralPath $lock -PathType Leaf) -or ([IO.File]::ReadAllText($lock)).Trim() -ne $observed) { throw 'MAESTRO-SCRIPT-BUSY: lock mudou durante recuperacao; nada foi alterado' }
    Remove-Item -LiteralPath $lock -Force
    [IO.File]::WriteAllText($candidate, $script:ProjectionLockToken + "@@BT@@n")
    try { [IO.File]::Move($candidate, $lock) } catch { throw 'MAESTRO-SCRIPT-BUSY: outra projecao assumiu o lock; nada foi alterado' }
  } finally {
    Remove-Item -LiteralPath $candidate -Force -ErrorAction SilentlyContinue
    if ((Test-Path -LiteralPath $reclaim -PathType Leaf) -and ([IO.File]::ReadAllText($reclaim)).Trim() -eq $reclaimToken) { Remove-Item -LiteralPath $reclaim -Force -ErrorAction SilentlyContinue }
  }
}

function Exit-ProjectionLock {
  $lock = Join-Path $stateRoot 'projection-lock'
  if ((Test-Path -LiteralPath $lock -PathType Leaf) -and $script:ProjectionLockToken -and ([IO.File]::ReadAllText($lock)).Trim() -eq $script:ProjectionLockToken) { Remove-Item -LiteralPath $lock -Force -ErrorAction SilentlyContinue }
  $script:ProjectionLockToken = ''
}

function Copy-ProjectionBackup([string]$fromRelease, [string]$backup) {
  $skillBackup = Join-Path $backup 'skills'
  $agentBackup = Join-Path $backup 'agents'
  $fixedBackup = Join-Path $backup 'fixed'
  New-Item -ItemType Directory -Force -Path $skillBackup, $agentBackup, $fixedBackup | Out-Null
  if ($fromRelease) {
    foreach ($skill in Get-ChildItem -LiteralPath (Join-Path $fromRelease 'projection\skills') -Directory) {
      $installed = Join-Path $Workspace ".claude\skills\$($skill.Name)"
      if (Test-Path -LiteralPath $installed) { Copy-Item -LiteralPath $installed -Destination (Join-Path $skillBackup $skill.Name) -Recurse }
    }
    foreach ($agent in Get-ChildItem -LiteralPath (Join-Path $fromRelease 'projection\agents') -File -Filter '*.md') {
      $installed = Join-Path $Workspace ".claude\agents\$($agent.Name)"
      if (Test-Path -LiteralPath $installed -PathType Leaf) { Copy-Item -LiteralPath $installed -Destination (Join-Path $agentBackup $agent.Name) }
    }
  }
  $fixed = [ordered]@{
    'claude-settings.local.json' = (Join-Path $Workspace '.claude\settings.local.json')
    'state-hooks' = (Join-Path $Workspace '.maestro-script\hooks')
    'state-capabilities.json' = (Join-Path $Workspace '.maestro-script\capabilities.json')
    'state-active-version' = (Join-Path $Workspace '.maestro-script\active-version')
    'state-managed-skills' = (Join-Path $Workspace '.maestro-script\managed-skills')
    'state-managed-agents' = (Join-Path $Workspace '.maestro-script\managed-agents')
    'state-settings-local.sha256' = (Join-Path $Workspace '.maestro-script\settings-local.sha256')
    'state-projection-receipt.json' = (Join-Path $Workspace '.maestro-script\projection-receipt.json')
  }
  foreach ($entry in $fixed.GetEnumerator()) { if (Test-Path -LiteralPath $entry.Value) { Copy-Item -LiteralPath $entry.Value -Destination (Join-Path $fixedBackup $entry.Key) -Recurse } }
  $claude = Join-Path $Workspace 'CLAUDE.md'
  if (Test-Path -LiteralPath $claude -PathType Leaf) {
    [IO.File]::WriteAllText((Join-Path $backup 'claude-original-present'), "present@@BT@@n")
    [IO.File]::WriteAllText((Join-Path $backup 'claude-original.sha256'), (Get-FileHash -LiteralPath $claude -Algorithm SHA256).Hash.ToLowerInvariant() + "@@BT@@n")
    [IO.File]::WriteAllText((Join-Path $backup 'claude-original.size'), (Get-Item -LiteralPath $claude -Force).Length.ToString() + "@@BT@@n")
    $matches = [regex]::Matches((Read-StrictUTF8 $claude), '(?s)<!-- MAESTRO SCRIPT MANAGED BEGIN -->.*?<!-- MAESTRO SCRIPT MANAGED END -->')
    if ($matches.Count -eq 1) { [IO.File]::WriteAllText((Join-Path $backup 'managed-claude-block'), $matches[0].Value) }
  }
  foreach ($name in @('active-version','previous-version','workspace')) {
    $source = Join-Path $stateRoot $name
    if (Test-Path -LiteralPath $source -PathType Leaf) { Copy-Item -LiteralPath $source -Destination (Join-Path $backup "global-$name") }
  }
}

function Remove-ReleaseProjectionPaths([string]$release) {
  if (-not $release -or -not (Test-Path -LiteralPath $release -PathType Container)) { return }
  foreach ($skill in Get-ChildItem -LiteralPath (Join-Path $release 'projection\skills') -Directory) { Remove-Item -LiteralPath (Join-Path $Workspace ".claude\skills\$($skill.Name)") -Recurse -Force -ErrorAction SilentlyContinue }
  foreach ($agent in Get-ChildItem -LiteralPath (Join-Path $release 'projection\agents') -File -Filter '*.md') { Remove-Item -LiteralPath (Join-Path $Workspace ".claude\agents\$($agent.Name)") -Force -ErrorAction SilentlyContinue }
}

function Restore-ProjectionPath([string]$backup, [string]$destination) {
  Remove-Item -LiteralPath $destination -Recurse -Force -ErrorAction SilentlyContinue
  if (Test-Path -LiteralPath $backup) { Copy-Item -LiteralPath $backup -Destination $destination -Recurse }
}

function Test-RecoveryLiveProjection([string]$fromRelease, [string]$toRelease) {
  foreach ($release in @($fromRelease,$toRelease)) {
    if (-not $release -or -not (Test-Path -LiteralPath $release -PathType Container)) { continue }
    foreach ($skill in Get-ChildItem -LiteralPath (Join-Path $release 'projection\skills') -Directory) {
      $actual = Join-Path $Workspace ".claude\skills\$($skill.Name)"
      if (-not (Test-Path -LiteralPath $actual)) { continue }
      $known = $false
      if ($fromRelease) {
        $source = Join-Path $fromRelease "projection\skills\$($skill.Name)"
        if (Test-Path -LiteralPath $source -PathType Container) { $known = Test-ProjectionTreeIsKnown $source $actual }
      }
      if (-not $known) {
        $source = Join-Path $toRelease "projection\skills\$($skill.Name)"
        if (Test-Path -LiteralPath $source -PathType Container) { $known = Test-ProjectionTreeIsKnown $source $actual }
      }
      if (-not $known) { throw "MAESTRO-SCRIPT-CONFLICT: skill mudou durante recuperacao: $($skill.Name); nada foi sobrescrito" }
    }
    foreach ($agent in Get-ChildItem -LiteralPath (Join-Path $release 'projection\agents') -File -Filter '*.md') {
      $actual = Join-Path $Workspace ".claude\agents\$($agent.Name)"
      if (-not (Test-Path -LiteralPath $actual)) { continue }
      $known = $false
      foreach ($candidateRoot in @($fromRelease,$toRelease)) {
        if (-not $candidateRoot) { continue }
        $candidate = Join-Path $candidateRoot "projection\agents\$($agent.Name)"
        if ((Test-Path -LiteralPath $candidate -PathType Leaf) -and (Get-FileHash -LiteralPath $candidate -Algorithm SHA256).Hash -eq (Get-FileHash -LiteralPath $actual -Algorithm SHA256).Hash) { $known = $true; break }
      }
      if (-not $known) { throw "MAESTRO-SCRIPT-CONFLICT: agente mudou durante recuperacao: $($agent.Name); nada foi sobrescrito" }
    }
  }
  $knownFile = {
    param([string]$actual, [string[]]$candidates)
    if (-not (Test-Path -LiteralPath $actual)) { return $true }
    if (-not (Test-Path -LiteralPath $actual -PathType Leaf) -or ((Get-Item -LiteralPath $actual -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { return $false }
    $actualHash = (Get-FileHash -LiteralPath $actual -Algorithm SHA256).Hash
    foreach ($candidate in $candidates) { if ($candidate -and (Test-Path -LiteralPath $candidate -PathType Leaf) -and (Get-FileHash -LiteralPath $candidate -Algorithm SHA256).Hash -eq $actualHash) { return $true } }
    return $false
  }
  $fromSettings = if ($fromRelease) { Join-Path $fromRelease 'projection\settings.local.json' } else { '' }
  $fromCapabilities = if ($fromRelease) { Join-Path $fromRelease 'capabilities.json' } else { '' }
  $fromHook = if ($fromRelease) { Join-Path $fromRelease 'Maestro-Hook.ps1' } else { '' }
  if (-not (& $knownFile (Join-Path $Workspace '.claude\settings.local.json') @($fromSettings,(Join-Path $toRelease 'projection\settings.local.json')))) { throw 'MAESTRO-SCRIPT-CONFLICT: settings mudou durante recuperacao; nada foi sobrescrito' }
  if (-not (& $knownFile (Join-Path $Workspace '.maestro-script\capabilities.json') @($fromCapabilities,(Join-Path $toRelease 'capabilities.json')))) { throw 'MAESTRO-SCRIPT-CONFLICT: capacidades mudaram durante recuperacao; nada foi sobrescrito' }
  $hookRoot = Join-Path $Workspace '.maestro-script\hooks'
  if (Test-Path -LiteralPath $hookRoot) {
    if (-not (Test-Path -LiteralPath $hookRoot -PathType Container) -or ((Get-Item -LiteralPath $hookRoot -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-SCRIPT-CONFLICT: hooks mudaram durante recuperacao; nada foi sobrescrito' }
    $hookFiles = @(Get-ChildItem -LiteralPath $hookRoot -Force)
    if ($hookFiles.Count -ne 1 -or $hookFiles[0].PSIsContainer -or $hookFiles[0].Name -ne 'Maestro-Hook.ps1' -or -not (& $knownFile $hookFiles[0].FullName @($fromHook,(Join-Path $toRelease 'Maestro-Hook.ps1')))) { throw 'MAESTRO-SCRIPT-CONFLICT: hooks mudaram durante recuperacao; nada foi sobrescrito' }
  }
}

function Restore-ProjectionTransaction {
  $tx = Join-Path $stateRoot 'projection-transaction'
  if (-not (Test-Path -LiteralPath (Join-Path $tx 'prepared') -PathType Leaf)) { Remove-Item -LiteralPath $tx -Recurse -Force -ErrorAction SilentlyContinue; return }
  $from = ([IO.File]::ReadAllText((Join-Path $tx 'from-version'))).Trim()
  $to = ([IO.File]::ReadAllText((Join-Path $tx 'to-version'))).Trim()
  $txWorkspace = ([IO.File]::ReadAllText((Join-Path $tx 'workspace'))).Trim()
  if ($txWorkspace -ne $Workspace) { throw 'MAESTRO-SCRIPT-CONFLICT: journal pertence a outro workspace; nada foi sobrescrito' }
  $fromRelease = if ($from) { Join-Path $runtimeRoot "releases\$from" } else { '' }
  $toRelease = Join-Path $runtimeRoot "releases\$to"
  Test-RuntimeIntegrity $toRelease
  if ($fromRelease) { Test-RuntimeIntegrity $fromRelease }
  Test-RecoveryLiveProjection $fromRelease $toRelease
  $claude = Join-Path $Workspace 'CLAUDE.md'
  $originalHashPath = Join-Path $tx 'backup\claude-original.sha256'
  $originalHash = if (Test-Path -LiteralPath $originalHashPath -PathType Leaf) { ([IO.File]::ReadAllText($originalHashPath)).Trim() } else { '' }
  $targetHash = ([IO.File]::ReadAllText((Join-Path $tx 'claude-target.sha256'))).Trim()
  $currentHash = if (Test-Path -LiteralPath $claude -PathType Leaf) { (Get-FileHash -LiteralPath $claude -Algorithm SHA256).Hash.ToLowerInvariant() } else { 'absent' }
  $originalWasAbsent = -not (Test-Path -LiteralPath (Join-Path $tx 'backup\claude-original-present') -PathType Leaf)
  if ($currentHash -ne $targetHash -and (-not $originalHash -or $currentHash -ne $originalHash) -and -not ($currentHash -eq 'absent' -and $originalWasAbsent)) { throw 'MAESTRO-SCRIPT-CONFLICT: CLAUDE.md mudou durante recuperacao; nada foi sobrescrito' }
  Remove-ReleaseProjectionPaths $fromRelease
  Remove-ReleaseProjectionPaths $toRelease
  foreach ($skill in Get-ChildItem -LiteralPath (Join-Path $tx 'backup\skills') -Directory) { Copy-Item -LiteralPath $skill.FullName -Destination (Join-Path $Workspace ".claude\skills\$($skill.Name)") -Recurse }
  foreach ($agent in Get-ChildItem -LiteralPath (Join-Path $tx 'backup\agents') -File -Filter '*.md') { Copy-Item -LiteralPath $agent.FullName -Destination (Join-Path $Workspace ".claude\agents\$($agent.Name)") }
  $fixed = [ordered]@{
    'claude-settings.local.json' = (Join-Path $Workspace '.claude\settings.local.json')
    'state-hooks' = (Join-Path $Workspace '.maestro-script\hooks')
    'state-capabilities.json' = (Join-Path $Workspace '.maestro-script\capabilities.json')
    'state-active-version' = (Join-Path $Workspace '.maestro-script\active-version')
    'state-managed-skills' = (Join-Path $Workspace '.maestro-script\managed-skills')
    'state-managed-agents' = (Join-Path $Workspace '.maestro-script\managed-agents')
    'state-settings-local.sha256' = (Join-Path $Workspace '.maestro-script\settings-local.sha256')
    'state-projection-receipt.json' = (Join-Path $Workspace '.maestro-script\projection-receipt.json')
  }
  foreach ($entry in $fixed.GetEnumerator()) { Restore-ProjectionPath (Join-Path $tx "backup\fixed\$($entry.Key)") $entry.Value }
  if ($currentHash -eq $targetHash) {
    if ($originalWasAbsent) {
      Remove-Item -LiteralPath $claude -Force -ErrorAction SilentlyContinue
    } else {
      $blockPath = Join-Path $tx 'backup\managed-claude-block'
      if (Test-Path -LiteralPath $blockPath -PathType Leaf) {
        $restored = Get-ProjectedClaudeBody (Read-StrictUTF8 $claude) ([IO.File]::ReadAllText($blockPath))
        Write-StrictUTF8 "$claude.recovery.tmp" $restored
      } else {
        $size = [int64]([IO.File]::ReadAllText((Join-Path $tx 'backup\claude-original.size'))).Trim()
        $currentBytes = [IO.File]::ReadAllBytes($claude)
        if ($size -gt $currentBytes.Length) { throw 'MAESTRO-SCRIPT-CONFLICT: CLAUDE.md nao pode ser restaurado sem perda; nada foi sobrescrito' }
        $originalBytes = New-Object byte[] $size
        [Array]::Copy($currentBytes, $originalBytes, $size)
        [IO.File]::WriteAllBytes("$claude.recovery.tmp", $originalBytes)
      }
      if ((Get-FileHash -LiteralPath "$claude.recovery.tmp" -Algorithm SHA256).Hash.ToLowerInvariant() -ne $originalHash) { Remove-Item -LiteralPath "$claude.recovery.tmp" -Force -ErrorAction SilentlyContinue; throw 'MAESTRO-SCRIPT-CONFLICT: CLAUDE.md nao pode ser restaurado sem perda; nada foi sobrescrito' }
      Move-Item -LiteralPath "$claude.recovery.tmp" -Destination $claude -Force
    }
  }
  foreach ($name in @('active-version','previous-version','workspace')) { Restore-ProjectionPath (Join-Path $tx "backup\global-$name") (Join-Path $stateRoot $name) }
  Remove-Item -LiteralPath $tx -Recurse -Force
}

function Repair-PendingProjection {
  $tx = Join-Path $stateRoot 'projection-transaction'
  if (-not (Test-Path -LiteralPath $tx -PathType Container)) { return }
  Enter-ProjectionLock
  Restore-ProjectionTransaction
  Exit-ProjectionLock
}

function Start-ProjectionTransaction([string]$releaseRoot, [string]$desiredPrevious) {
  Enter-ProjectionLock
  $staging = Join-Path $stateRoot ".projection-transaction-$PID"
  Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Path $staging | Out-Null
  $from = if (Test-Path -LiteralPath (Join-Path $stateRoot 'active-version') -PathType Leaf) { ([IO.File]::ReadAllText((Join-Path $stateRoot 'active-version'))).Trim() } else { '' }
  $fromRelease = if ($from) { Join-Path $runtimeRoot "releases\$from" } else { '' }
  Copy-ProjectionBackup $fromRelease (Join-Path $staging 'backup')
  $targetBlock = Get-ManagedClaudeBlock $releaseRoot
  [IO.File]::WriteAllText((Join-Path $staging 'claude-target-block'), $targetBlock)
  $claudePath = Join-Path $Workspace 'CLAUDE.md'
  $claudeBody = Read-StrictUTF8 $claudePath
  [IO.File]::WriteAllText((Join-Path $staging 'claude-target.sha256'), (Get-TextSHA256 (Get-ProjectedClaudeBody $claudeBody $targetBlock)) + "@@BT@@n")
  [IO.File]::WriteAllText((Join-Path $staging 'from-version'), "$from@@BT@@n")
  [IO.File]::WriteAllText((Join-Path $staging 'to-version'), (Split-Path $releaseRoot -Leaf) + "@@BT@@n")
  [IO.File]::WriteAllText((Join-Path $staging 'desired-previous-version'), "$desiredPrevious@@BT@@n")
  [IO.File]::WriteAllText((Join-Path $staging 'workspace'), "$Workspace@@BT@@n")
  [IO.File]::WriteAllText((Join-Path $staging 'prepared'), "prepared@@BT@@n")
  Move-Item -LiteralPath $staging -Destination (Join-Path $stateRoot 'projection-transaction')
  [IO.File]::WriteAllText((Join-Path $stateRoot 'workspace'), "$Workspace@@BT@@n")
  Invoke-FailurePoint 'after_prepared'
}

function Complete-ProjectionTransaction {
  $tx = Join-Path $stateRoot 'projection-transaction'
  $previous = ([IO.File]::ReadAllText((Join-Path $tx 'desired-previous-version'))).Trim()
  $active = ([IO.File]::ReadAllText((Join-Path $tx 'to-version'))).Trim()
  $previousPath = Join-Path $stateRoot 'previous-version'
  if ($previous) { [IO.File]::WriteAllText("$previousPath.tmp", "$previous@@BT@@n"); Move-Item -LiteralPath "$previousPath.tmp" -Destination $previousPath -Force } else { Remove-Item -LiteralPath $previousPath -Force -ErrorAction SilentlyContinue }
  Invoke-FailurePoint 'after_previous_pointer'
  $activePath = Join-Path $stateRoot 'active-version'
  [IO.File]::WriteAllText("$activePath.tmp", "$active@@BT@@n")
  Move-Item -LiteralPath "$activePath.tmp" -Destination $activePath -Force
  Invoke-FailurePoint 'after_active_pointer'
  Remove-Item -LiteralPath $tx -Recurse -Force
  Exit-ProjectionLock
}

function Set-WorkspaceProjection([string]$releaseRoot, [string]$desiredPrevious = '') {
  $claudeRoot = Join-Path $Workspace '.claude'
  $scriptRoot = Join-Path $Workspace '.maestro-script'
  $claude = Join-Path $Workspace 'CLAUDE.md'
  foreach ($candidate in @($Workspace, $claudeRoot, $scriptRoot)) {
    if ((Test-Path -LiteralPath $candidate) -and ((Get-Item -LiteralPath $candidate -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-SCRIPT-PATH: workspace ou estado e reparse point' }
  }
  $body = Read-StrictUTF8 $claude
  New-Item -ItemType Directory -Force -Path (Join-Path $claudeRoot 'skills'), (Join-Path $claudeRoot 'agents'), $scriptRoot | Out-Null
  foreach ($candidate in @((Join-Path $claudeRoot 'skills'), (Join-Path $claudeRoot 'agents'), (Join-Path $scriptRoot 'managed-skills'), (Join-Path $scriptRoot 'managed-agents'))) {
    if ((Test-Path -LiteralPath $candidate) -and ((Get-Item -LiteralPath $candidate -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-SCRIPT-PATH: caminho gerenciado e reparse point' }
  }
  $settings = Join-Path $claudeRoot 'settings.local.json'
  $settingsReceipt = Join-Path $scriptRoot 'settings-local.sha256'
  $hooksRoot = Join-Path $scriptRoot 'hooks'
  $projectionReceipt = Join-Path $scriptRoot 'projection-receipt.json'
  foreach ($candidate in @((Join-Path $scriptRoot 'capabilities.json'), (Join-Path $scriptRoot 'active-version'), $projectionReceipt, $settings, $settingsReceipt, $hooksRoot, (Join-Path $Workspace 'CLAUDE.md'))) {
    if ((Test-Path -LiteralPath $candidate) -and ((Get-Item -LiteralPath $candidate -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-SCRIPT-PATH: arquivo gerenciado e reparse point' }
  }
  if (Test-Path -LiteralPath $settings) {
    if (-not (Test-Path -LiteralPath $settings -PathType Leaf) -or -not (Test-Path -LiteralPath $settingsReceipt -PathType Leaf)) { throw 'MAESTRO-SCRIPT-CONFLICT: settings.local.json nao pertence ao Maestro; nada foi sobrescrito' }
    $expectedSettings = ([IO.File]::ReadAllText($settingsReceipt)).Trim()
    $actualSettings = (Get-FileHash -LiteralPath $settings -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualSettings -ne $expectedSettings) {
      $recognized = $actualSettings -eq (Get-FileHash -LiteralPath (Join-Path $releaseRoot 'projection\settings.local.json') -Algorithm SHA256).Hash.ToLowerInvariant()
      $activePath = Join-Path $stateRoot 'active-version'
      if (-not $recognized -and (Test-Path -LiteralPath $activePath -PathType Leaf)) {
        $current = ([IO.File]::ReadAllText($activePath)).Trim()
        $currentSettings = Join-Path $runtimeRoot "releases\$current\projection\settings.local.json"
        if (Test-Path -LiteralPath $currentSettings -PathType Leaf) { $recognized = $actualSettings -eq (Get-FileHash -LiteralPath $currentSettings -Algorithm SHA256).Hash.ToLowerInvariant() }
      }
      if (-not $recognized) { throw 'MAESTRO-SCRIPT-CONFLICT: settings.local.json foi alterado pelo owner; nada foi sobrescrito' }
    }
  }
  $managedList = Join-Path $scriptRoot 'managed-skills'
  $managedAgents = Join-Path $scriptRoot 'managed-agents'
  $oldNames = if (Test-Path -LiteralPath $managedList) { @(Get-Content -LiteralPath $managedList) } else { @() }
  foreach ($name in $oldNames) { if ($name -and $name -notmatch '^[a-z0-9-]+$') { throw 'MAESTRO-SCRIPT-STATE: skill gerenciada invalida' } }
  $oldAgents = if (Test-Path -LiteralPath $managedAgents) { @(Get-Content -LiteralPath $managedAgents) } else { @() }
  foreach ($name in $oldAgents) { if ($name -and $name -notmatch '^[a-z0-9-]+\.md$') { throw 'MAESTRO-SCRIPT-STATE: agente gerenciado invalido' } }
  $currentRelease = ''
  if (Test-Path -LiteralPath (Join-Path $stateRoot 'active-version') -PathType Leaf) {
    $currentVersion = ([IO.File]::ReadAllText((Join-Path $stateRoot 'active-version'))).Trim()
    $currentRelease = Join-Path $runtimeRoot "releases\$currentVersion"
    Test-RuntimeIntegrity $currentRelease
  }
  foreach ($agent in Get-ChildItem -LiteralPath (Join-Path $releaseRoot 'projection\agents') -File -Filter '*.md') {
    $target = Join-Path $claudeRoot "agents\$($agent.Name)"
    if ((Test-Path -LiteralPath $target) -and ((Get-Item -LiteralPath $target -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "MAESTRO-SCRIPT-PATH: agente existente e reparse point: $($agent.Name)" }
    if ((Test-Path -LiteralPath $target) -and (($oldAgents -notcontains $agent.Name) -or -not (Test-ManagedAgentIsKnown $releaseRoot $agent.Name $target))) { throw "MAESTRO-SCRIPT-CONFLICT: agente local existente ou alterado: $($agent.Name); nada foi sobrescrito" }
  }
  if ($currentRelease) {
    foreach ($skill in Get-ChildItem -LiteralPath (Join-Path $currentRelease 'projection\skills') -Directory) {
      if (Test-Path -LiteralPath (Join-Path $releaseRoot "projection\skills\$($skill.Name)") -PathType Container) { continue }
      $actual = Join-Path $claudeRoot "skills\$($skill.Name)"
      if ((Test-Path -LiteralPath $actual) -and -not (Test-ProjectionTreeIsKnown $skill.FullName $actual)) { throw "MAESTRO-SCRIPT-CONFLICT: skill gerenciada removida foi alterada: $($skill.Name); nada foi sobrescrito" }
    }
    foreach ($agent in Get-ChildItem -LiteralPath (Join-Path $currentRelease 'projection\agents') -File -Filter '*.md') {
      if (Test-Path -LiteralPath (Join-Path $releaseRoot "projection\agents\$($agent.Name)") -PathType Leaf) { continue }
      $actual = Join-Path $claudeRoot "agents\$($agent.Name)"
      if ((Test-Path -LiteralPath $actual) -and ((Get-FileHash -LiteralPath $agent.FullName -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath $actual -Algorithm SHA256).Hash)) { throw "MAESTRO-SCRIPT-CONFLICT: agente gerenciado removido foi alterado: $($agent.Name); nada foi sobrescrito" }
    }
  }
  $beginCount = ([regex]::Matches($body, '<!-- MAESTRO SCRIPT MANAGED BEGIN -->')).Count
  $endCount = ([regex]::Matches($body, '<!-- MAESTRO SCRIPT MANAGED END -->')).Count
  if ($beginCount -ne $endCount -or $beginCount -gt 1 -or ($beginCount -eq 1 -and $body.IndexOf('<!-- MAESTRO SCRIPT MANAGED BEGIN -->') -gt $body.IndexOf('<!-- MAESTRO SCRIPT MANAGED END -->'))) { throw 'MAESTRO-SCRIPT-STATE: bloco CLAUDE gerenciado invalido' }
  if ($beginCount -eq 1) {
    $actualBlock = [regex]::Match($body, '(?s)<!-- MAESTRO SCRIPT MANAGED BEGIN -->.*?<!-- MAESTRO SCRIPT MANAGED END -->').Value
    $knownBlock = $actualBlock -eq (Get-ManagedClaudeBlock $releaseRoot)
    if (-not $knownBlock -and $currentRelease) { $knownBlock = $actualBlock -eq (Get-ManagedClaudeBlock $currentRelease) }
    if (-not $knownBlock -and $currentRelease -and (Test-Path -LiteralPath $projectionReceipt -PathType Leaf)) {
      try {
        $receiptValue = [IO.File]::ReadAllText($projectionReceipt) | ConvertFrom-Json
        $receiptDigest = [string]$receiptValue.managed_claude_block_sha256
        $knownBlock = $receiptValue.version -eq $currentVersion -and $receiptDigest -match '^[a-f0-9]{64}$' -and (Get-TextSHA256 $actualBlock) -eq $receiptDigest
      } catch { $knownBlock = $false }
    }
    if (-not $knownBlock -and $currentRelease) { $knownBlock = $actualBlock -eq (Get-LegacyManagedClaudeBlock $currentRelease) }
    if (-not $knownBlock) { throw 'MAESTRO-SCRIPT-CONFLICT: bloco CLAUDE foi alterado pelo owner; nada foi sobrescrito' }
  }
  $skillStage = Join-Path $scriptRoot "skills-staging-$PID"
  Remove-Item -LiteralPath $skillStage -Recurse -Force -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Path $skillStage | Out-Null
  $names = @()
  foreach ($skill in Get-ChildItem -LiteralPath (Join-Path $releaseRoot 'projection\skills') -Directory) {
    $target = Join-Path $claudeRoot "skills\$($skill.Name)"
    if ((Test-Path -LiteralPath $target) -and ((Get-Item -LiteralPath $target -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "MAESTRO-SCRIPT-PATH: skill existente e reparse point: $($skill.Name)" }
    if (Test-Path -LiteralPath $target) {
      $known = $oldNames -contains $skill.Name -and (Test-ProjectionTreeIsKnown $skill.FullName $target)
      if (-not $known -and (Test-Path -LiteralPath (Join-Path $stateRoot 'active-version') -PathType Leaf)) {
        $current = ([IO.File]::ReadAllText((Join-Path $stateRoot 'active-version'))).Trim()
        $currentSkill = Join-Path $runtimeRoot "releases\$current\projection\skills\$($skill.Name)"
        $known = $oldNames -contains $skill.Name -and (Test-Path -LiteralPath $currentSkill -PathType Container) -and (Test-ProjectionTreeIsKnown $currentSkill $target)
      }
      if (-not $known) { throw "MAESTRO-SCRIPT-CONFLICT: skill local existente ou alterada: $($skill.Name); nada foi sobrescrito" }
    }
    Copy-Item -LiteralPath $skill.FullName -Destination (Join-Path $skillStage $skill.Name) -Recurse
    $names += $skill.Name
  }
  Start-ProjectionTransaction $releaseRoot $desiredPrevious
  Remove-Item -LiteralPath $projectionReceipt -Force -ErrorAction SilentlyContinue
  Invoke-FailurePoint 'after_receipt_invalidated'
  if ($currentRelease) { foreach ($skill in Get-ChildItem -LiteralPath (Join-Path $currentRelease 'projection\skills') -Directory) { Remove-Item -LiteralPath (Join-Path $claudeRoot "skills\$($skill.Name)") -Recurse -Force -ErrorAction SilentlyContinue } }
  foreach ($skill in Get-ChildItem -LiteralPath $skillStage -Directory) { Move-Item -LiteralPath $skill.FullName -Destination (Join-Path $claudeRoot "skills\$($skill.Name)") }
  [IO.File]::WriteAllLines($managedList, $names)
  Remove-Item -LiteralPath $skillStage -Recurse -Force
  Invoke-FailurePoint 'after_skills'
  $agentStage = Join-Path $scriptRoot "agents-staging-$PID"
  Remove-Item -LiteralPath $agentStage -Recurse -Force -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Path $agentStage | Out-Null
  $agentNames = @()
  foreach ($agent in Get-ChildItem -LiteralPath (Join-Path $releaseRoot 'projection\agents') -File -Filter '*.md') {
    $target = Join-Path $claudeRoot "agents\$($agent.Name)"
    if ((Test-Path -LiteralPath $target) -and ((Get-Item -LiteralPath $target -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "MAESTRO-SCRIPT-PATH: agente existente e reparse point: $($agent.Name)" }
    if ((Test-Path -LiteralPath $target) -and (($oldAgents -notcontains $agent.Name) -or -not (Test-ManagedAgentIsKnown $releaseRoot $agent.Name $target))) { throw "MAESTRO-SCRIPT-CONFLICT: agente local existente ou alterado: $($agent.Name); nada foi sobrescrito" }
    Copy-Item -LiteralPath $agent.FullName -Destination (Join-Path $agentStage $agent.Name)
    $agentNames += $agent.Name
  }
  if ($currentRelease) { foreach ($agent in Get-ChildItem -LiteralPath (Join-Path $currentRelease 'projection\agents') -File -Filter '*.md') { Remove-Item -LiteralPath (Join-Path $claudeRoot "agents\$($agent.Name)") -Force -ErrorAction SilentlyContinue } }
  foreach ($agent in Get-ChildItem -LiteralPath $agentStage -File -Filter '*.md') { Move-Item -LiteralPath $agent.FullName -Destination (Join-Path $claudeRoot "agents\$($agent.Name)") }
  [IO.File]::WriteAllLines($managedAgents, $agentNames)
  Remove-Item -LiteralPath $agentStage -Recurse -Force
  Invoke-FailurePoint 'after_agents'
  $hooksStage = Join-Path $scriptRoot "hooks-staging-$PID"
  Remove-Item -LiteralPath $hooksStage -Recurse -Force -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Path $hooksStage | Out-Null
  Copy-Item -LiteralPath (Join-Path $releaseRoot 'Maestro-Hook.ps1') -Destination (Join-Path $hooksStage 'Maestro-Hook.ps1')
  Remove-Item -LiteralPath $hooksRoot -Recurse -Force -ErrorAction SilentlyContinue
  Move-Item -LiteralPath $hooksStage -Destination $hooksRoot
  Invoke-FailurePoint 'after_hooks'
  Copy-Item -LiteralPath (Join-Path $releaseRoot 'projection\settings.local.json') -Destination "$settings.tmp" -Force
  Move-Item -LiteralPath "$settings.tmp" -Destination $settings -Force
  [IO.File]::WriteAllText("$settingsReceipt.tmp", (Get-FileHash -LiteralPath $settings -Algorithm SHA256).Hash.ToLowerInvariant() + "@@BT@@n")
  Move-Item -LiteralPath "$settingsReceipt.tmp" -Destination $settingsReceipt -Force
  Invoke-FailurePoint 'after_settings'
  Copy-Item -LiteralPath (Join-Path $releaseRoot 'capabilities.json') -Destination (Join-Path $scriptRoot 'capabilities.json') -Force
  [IO.File]::WriteAllText((Join-Path $scriptRoot 'active-version'), (Split-Path $releaseRoot -Leaf) + "@@BT@@n")
  $block = [IO.File]::ReadAllText((Join-Path $stateRoot 'projection-transaction\claude-target-block'))
  $liveBody = Read-StrictUTF8 $claude
  Write-StrictUTF8 "$claude.tmp" (Get-ProjectedClaudeBody $liveBody $block)
  if ((Get-FileHash -LiteralPath "$claude.tmp" -Algorithm SHA256).Hash.ToLowerInvariant() -ne ([IO.File]::ReadAllText((Join-Path $stateRoot 'projection-transaction\claude-target.sha256'))).Trim()) { Remove-Item -LiteralPath "$claude.tmp" -Force -ErrorAction SilentlyContinue; throw 'MAESTRO-SCRIPT-CONFLICT: CLAUDE.md mudou durante a projecao; nada foi sobrescrito' }
  Move-Item -LiteralPath "$claude.tmp" -Destination $claude -Force
  Invoke-FailurePoint 'after_claude'
  [IO.File]::WriteAllText("$projectionReceipt.tmp", (Get-ProjectionReceiptBody $releaseRoot) + "@@BT@@n")
  Move-Item -LiteralPath "$projectionReceipt.tmp" -Destination $projectionReceipt -Force
  Invoke-FailurePoint 'after_receipt'
  Complete-ProjectionTransaction
}

function Invoke-Rollback {
  $activePath = Join-Path $stateRoot 'active-version'
  $previousPath = Join-Path $stateRoot 'previous-version'
  if (-not (Test-Path $activePath) -or -not (Test-Path $previousPath)) { throw 'MAESTRO-SCRIPT-ROLLBACK: nenhuma versao anterior' }
  $active = (Get-Content $activePath -Raw).Trim()
  $previous = (Get-Content $previousPath -Raw).Trim()
  $releaseRoot = Join-Path $runtimeRoot "releases\$previous"
  if (-not (Test-Path $releaseRoot -PathType Container)) { throw 'MAESTRO-SCRIPT-ROLLBACK: versao anterior ausente' }
  Test-RuntimeIntegrity $releaseRoot
  Set-WorkspaceProjection $releaseRoot $active
  Write-Output "Maestro script-only voltou para $previous"
}

function Test-InstalledHooks {
  if (Test-Path -LiteralPath (Join-Path $stateRoot 'projection-transaction') -PathType Container) { throw 'MAESTRO-SCRIPT-DOCTOR: projection_state=repair_required; execute install novamente para recuperar' }
  $activePath = Join-Path $stateRoot 'active-version'
  $workspacePath = Join-Path $stateRoot 'workspace'
  if (-not (Test-Path -LiteralPath $activePath -PathType Leaf) -or -not (Test-Path -LiteralPath $workspacePath -PathType Leaf)) { throw 'MAESTRO-SCRIPT-DOCTOR: runtime ou workspace incompleto' }
  $active = ([IO.File]::ReadAllText($activePath)).Trim()
  $releaseRoot = Join-Path $runtimeRoot "releases\$active"
  Test-RuntimeIntegrity $releaseRoot
  $installedWorkspace = ([IO.File]::ReadAllText($workspacePath)).Trim()
  if (-not (Test-Path -LiteralPath $installedWorkspace -PathType Container) -or ((Get-Item -LiteralPath $installedWorkspace -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'MAESTRO-SCRIPT-DOCTOR: workspace inseguro ou ausente' }
  $workspaceActive = Join-Path $installedWorkspace '.maestro-script\active-version'
  if (-not (Test-Path -LiteralPath $workspaceActive -PathType Leaf) -or ([IO.File]::ReadAllText($workspaceActive)).Trim() -ne $active) { throw 'MAESTRO-SCRIPT-DOCTOR: versao global e projecao do workspace divergem' }
  $settings = Join-Path $installedWorkspace '.claude\settings.local.json'
  $receipt = Join-Path $installedWorkspace '.maestro-script\settings-local.sha256'
  $hook = Join-Path $installedWorkspace '.maestro-script\hooks\Maestro-Hook.ps1'
  if (-not (Test-Path -LiteralPath $settings -PathType Leaf) -or -not (Test-Path -LiteralPath $receipt -PathType Leaf) -or -not (Test-Path -LiteralPath $hook -PathType Leaf)) { throw 'MAESTRO-SCRIPT-DOCTOR: hooks ausentes; execute install novamente para reparar' }
  $expected = ([IO.File]::ReadAllText($receipt)).Trim()
  $actual = (Get-FileHash -LiteralPath $settings -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw 'MAESTRO-SCRIPT-DOCTOR: settings.local.json foi alterado; nada foi sobrescrito' }
  $expectedHook = (Get-FileHash -LiteralPath (Join-Path $releaseRoot 'Maestro-Hook.ps1') -Algorithm SHA256).Hash
  $actualHook = (Get-FileHash -LiteralPath $hook -Algorithm SHA256).Hash
  if ($actualHook -ne $expectedHook) { throw 'MAESTRO-SCRIPT-DOCTOR: handler de hooks foi alterado' }
  $installedCapabilities = Join-Path $installedWorkspace '.maestro-script\capabilities.json'
  if (-not (Test-Path -LiteralPath $installedCapabilities -PathType Leaf) -or (Get-FileHash -LiteralPath $installedCapabilities -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath (Join-Path $releaseRoot 'capabilities.json') -Algorithm SHA256).Hash) { throw 'MAESTRO-SCRIPT-DOCTOR: matriz de capacidades ausente ou alterada' }
  $managedSkills = Join-Path $installedWorkspace '.maestro-script\managed-skills'
  if (-not (Test-Path -LiteralPath $managedSkills -PathType Leaf)) { throw 'MAESTRO-SCRIPT-DOCTOR: lista de skills gerenciadas ausente' }
  $expectedSkills = @((Get-ChildItem -LiteralPath (Join-Path $releaseRoot 'projection\skills') -Directory | ForEach-Object { $_.Name } | Sort-Object))
  $actualSkills = @((Get-Content -LiteralPath $managedSkills | Sort-Object))
  if (@(Compare-Object $expectedSkills $actualSkills).Count -ne 0) { throw 'MAESTRO-SCRIPT-DOCTOR: lista de skills gerenciadas diverge' }
  foreach ($skill in $expectedSkills) {
    Test-ProjectionTree (Join-Path $releaseRoot "projection\skills\$skill") (Join-Path $installedWorkspace ".claude\skills\$skill") "skill gerenciada $skill"
  }
  $managedAgents = Join-Path $installedWorkspace '.maestro-script\managed-agents'
  if (-not (Test-Path -LiteralPath $managedAgents -PathType Leaf)) { throw 'MAESTRO-SCRIPT-DOCTOR: lista de agentes gerenciados ausente' }
  $expectedAgents = @((Get-ChildItem -LiteralPath (Join-Path $releaseRoot 'projection\agents') -File -Filter '*.md' | ForEach-Object { $_.Name } | Sort-Object))
  $actualAgents = @((Get-Content -LiteralPath $managedAgents | Sort-Object))
  if (@(Compare-Object $expectedAgents $actualAgents).Count -ne 0) { throw 'MAESTRO-SCRIPT-DOCTOR: lista de agentes gerenciados diverge' }
  $settingsBody = [IO.File]::ReadAllText($settings)
  foreach ($event in @('SessionStart','UserPromptSubmit','PreToolUse','PostToolUse','Stop','SubagentStart','SubagentStop')) {
    if (-not $settingsBody.Contains('"' + $event + '"')) { throw "MAESTRO-SCRIPT-DOCTOR: hook ausente: $event" }
  }
  foreach ($agent in @('client-account-agent.md','case-agent.md','walter.md','darwin.md','pa-expert.md')) {
    $expectedAgent = Join-Path $releaseRoot "projection\agents\$agent"
    $installedAgent = Join-Path $installedWorkspace ".claude\agents\$agent"
    if (-not (Test-Path -LiteralPath $installedAgent -PathType Leaf) -or (Get-FileHash -LiteralPath $installedAgent -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath $expectedAgent -Algorithm SHA256).Hash) { throw "MAESTRO-SCRIPT-DOCTOR: agente gerenciado ausente ou alterado: $agent" }
  }
  $claude = Join-Path $installedWorkspace 'CLAUDE.md'
  if (-not (Test-Path -LiteralPath $claude -PathType Leaf)) { throw 'MAESTRO-SCRIPT-DOCTOR: orientacao gerenciada ausente' }
  $matches = [regex]::Matches((Read-StrictUTF8 $claude), '(?s)<!-- MAESTRO SCRIPT MANAGED BEGIN -->.*?<!-- MAESTRO SCRIPT MANAGED END -->')
  if ($matches.Count -ne 1 -or $matches[0].Value -ne (Get-ManagedClaudeBlock $releaseRoot)) { throw 'MAESTRO-SCRIPT-DOCTOR: bloco CLAUDE gerenciado ausente ou alterado' }
  $projectionReceipt = Join-Path $installedWorkspace '.maestro-script\projection-receipt.json'
  if (-not (Test-Path -LiteralPath $projectionReceipt -PathType Leaf)) { throw 'MAESTRO-SCRIPT-DOCTOR: recibo de projecao ausente; execute install novamente para reparar' }
  if (([IO.File]::ReadAllText($projectionReceipt)).Trim() -ne (Get-ProjectionReceiptBody $releaseRoot)) { throw 'MAESTRO-SCRIPT-DOCTOR: recibo de projecao diverge; execute install novamente para reparar' }
  Write-Output 'Maestro script-only: managed projection, seven Claude hooks and five specialists configured and intact on disk; Claude runtime observation pending; native CLI unavailable'
}

switch ($Action) {
  'install' {
    if (Test-Path -LiteralPath (Join-Path $stateRoot 'projection-transaction') -PathType Container) { Repair-PendingProjection }
    $target = Join-Path $runtimeRoot "releases\$Version"
    $activePath = Join-Path $stateRoot 'active-version'
    if ((Test-Path -LiteralPath $activePath -PathType Leaf) -and
        ((Get-Content -LiteralPath $activePath -Raw).Trim() -eq $Version) -and
        (Test-Path -LiteralPath $target -PathType Container)) {
      Test-RuntimeIntegrity $target
      Initialize-StableWorkspace
      $previous = if (Test-Path -LiteralPath (Join-Path $stateRoot 'previous-version') -PathType Leaf) { ([IO.File]::ReadAllText((Join-Path $stateRoot 'previous-version'))).Trim() } else { '' }
      Set-WorkspaceProjection $target $previous
      Write-Output "Maestro script-only $Version ja estava preparado"
      Write-Output "MAESTRO-SCRIPT-WORKSPACE: $Workspace"
      Write-Output 'Abra essa pasta em uma nova sessao do Claude Code para carregar os hooks.'
      Show-StableWorkspace
      exit 0
    }
    Test-PackageIntegrity
    Initialize-StableWorkspace
    $active = if (Test-Path $activePath) { (Get-Content $activePath -Raw).Trim() } else { '' }
    if ($active -and $active -ne $Version -and ([version]$Version -le [version]$active)) { throw 'MAESTRO-SCRIPT-VERSION: install aceita somente uma versao mais nova; use rollback para voltar' }
    New-Item -ItemType Directory -Force -Path (Join-Path $runtimeRoot 'releases'), $stateRoot | Out-Null
    if (Test-Path -LiteralPath $target) {
      $expectedInventory = (Get-FileHash -LiteralPath (Join-Path $Root 'runtime-inventory.sha256') -Algorithm SHA256).Hash
      $actualInventory = (Get-FileHash -LiteralPath (Join-Path $target 'runtime-inventory.sha256') -Algorithm SHA256).Hash
      if ($expectedInventory -ne $actualInventory) { throw 'MAESTRO-SCRIPT-RUNTIME: versao existente tem identidade diferente' }
      Test-RuntimeIntegrity $target
    } else {
      $staging = Join-Path $runtimeRoot ".staging-$Version-$PID"
      New-Item -ItemType Directory -Path $staging | Out-Null
      try {
        Copy-Item -LiteralPath (Join-Path $Root 'payload') -Destination (Join-Path $staging 'payload') -Recurse
        Copy-Item -LiteralPath (Join-Path $Root 'projection') -Destination (Join-Path $staging 'projection') -Recurse
        Copy-Item -LiteralPath (Join-Path $Root 'capabilities.json') -Destination (Join-Path $staging 'capabilities.json')
        Copy-Item -LiteralPath $PSCommandPath -Destination (Join-Path $staging 'Install-Maestro.ps1')
        Copy-Item -LiteralPath (Join-Path $Root 'Maestro-Hook.ps1') -Destination (Join-Path $staging 'Maestro-Hook.ps1')
        Copy-Item -LiteralPath (Join-Path $Root 'runtime-inventory.sha256') -Destination (Join-Path $staging 'runtime-inventory.sha256')
        Test-RuntimeIntegrity $staging
        Move-Item -LiteralPath $staging -Destination $target
      } finally { Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue }
    }
    $desiredPrevious = if ($active -and $active -ne $Version) { $active } elseif (Test-Path -LiteralPath (Join-Path $stateRoot 'previous-version') -PathType Leaf) { ([IO.File]::ReadAllText((Join-Path $stateRoot 'previous-version'))).Trim() } else { '' }
    Set-WorkspaceProjection $target $desiredPrevious
    Write-Output "Maestro script-only $Version preparado"
    Write-Output "MAESTRO-SCRIPT-WORKSPACE: $Workspace"
    Write-Output 'Abra essa pasta em uma nova sessao do Claude Code para carregar os hooks.'
    Show-StableWorkspace
  }
  'rollback' { if (Test-Path -LiteralPath (Join-Path $stateRoot 'projection-transaction') -PathType Container) { Repair-PendingProjection }; Invoke-Rollback }
  'status' {
    $active = if (Test-Path (Join-Path $stateRoot 'active-version')) { (Get-Content (Join-Path $stateRoot 'active-version') -Raw).Trim() } else { '' }
    $previous = if (Test-Path (Join-Path $stateRoot 'previous-version')) { (Get-Content (Join-Path $stateRoot 'previous-version') -Raw).Trim() } else { '' }
    $projectionState = if (Test-Path -LiteralPath (Join-Path $stateRoot 'projection-transaction') -PathType Container) { 'repair_required' } else { 'configured_on_disk' }
    [ordered]@{schema_version=1;profile='windows-powershell-local-beta';active_version=$active;previous_version=$previous;projection_state=$projectionState;native_cli='unavailable'} | ConvertTo-Json -Compress
  }
  'doctor' {
    Test-InstalledHooks
  }
}
`
	template = strings.ReplaceAll(template, "@@BT@@", "`")
	return []byte(strings.ReplaceAll(template, "@@VERSION@@", version))
}
