package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pilotCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action"`
}

// This is shareable diagnostic metadata, never a dump of context, logs or paths.
type pilotReport struct {
	SchemaVersion    int              `json:"schema_version"`
	Status           string           `json:"status"`
	Checks           []pilotCheck     `json:"checks"`
	WorkspaceID      string           `json:"workspace_id"`
	KitID            string           `json:"kit_id"`
	ReferenceHost    packaging.Target `json:"reference_host"`
	Target           packaging.Target `json:"target"`
	Markdown         string           `json:"markdown"`
	CodePrompt       string           `json:"code_prompt"`
	DesktopPrompt    string           `json:"desktop_prompt"`
	DesktopExtension string           `json:"desktop_extension,omitempty"`
}

func (r *pilotReport) add(name, status, reason, action string) {
	r.Checks = append(r.Checks, pilotCheck{Name: name, Status: status, Reason: reason, NextAction: action})
}

// runPilotCheck only inspects an explicitly prepared pilot layout. Wrappers own
// report files; this command never initializes, repairs or writes participant state.
func runPilotCheck(ctx context.Context, args []string, output io.Writer) error {
	r := pilotReport{SchemaVersion: 1, Status: "partial", Checks: []pilotCheck{}, ReferenceHost: packaging.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}}
	if _, err := processidentity.Current(); err != nil {
		r.add("process_identity", "failed", "process_identity_unavailable", "Execute Testar Maestro a partir do kit verificado e preserve os arquivos existentes.")
		return finishPilotReport(&r, output)
	}
	flags := flag.NewFlagSet("pilot-check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var o product.Options
	flags.StringVar(&o.ProductRoot, "product-root", "", "prepared pilot product")
	flags.StringVar(&o.UserHome, "user-home", "", "prepared pilot owner")
	flags.StringVar(&o.WorkspacePath, "workspace", "", "prepared pilot workspace")
	valid := flags.Parse(args) == nil && flags.NArg() == 0
	for _, path := range []string{o.ProductRoot, o.UserHome, o.WorkspacePath} {
		valid = valid && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n")
	}
	root := filepath.Dir(o.ProductRoot)
	valid = valid && filepath.Base(o.ProductRoot) == "product" && o.UserHome == filepath.Join(root, "owner") && o.WorkspacePath == filepath.Join(root, "workspace")
	if !valid {
		r.add("pilot_layout", "failed", "pilot_layout_invalid", "Execute Testar Maestro na pasta do ensaio preparado; preserve os arquivos existentes.")
		return finishPilotReport(&r, output)
	}
	r.add("pilot_layout", "passed", "pilot_layout_selected", "")
	if os.Getenv("MAESTRO_HOME") != "" {
		r.add("binding_admission", "failed", "home_override_present", "Feche o terminal com a seleção externa de memória e execute Testar Maestro pelo launcher do ensaio, sem MAESTRO_HOME definido.")
		return finishPilotReport(&r, output)
	}
	if body, err := readPilotFile(filepath.Join(root, ".maestro-pilot-owned"), 128); err == nil && pilotKitID.MatchString(strings.TrimSpace(string(body))) {
		r.KitID = strings.TrimSpace(string(body))
		r.add("kit_identity", "passed", "local_kit_identifier_recorded_not_publisher_authority", "")
	} else {
		r.add("kit_identity", "not_tested", "kit_identifier_unavailable", "Inclua a identificação do kit ao compartilhar o diagnóstico.")
	}
	codeRoot := filepath.Join(root, "exports", "code")
	packet, packetErr := packaging.ValidatePacket(codeRoot)
	codeOK := packetErr == nil && packet.Format == "plugin" && packet.Target == r.ReferenceHost
	if codeOK {
		r.Target = packet.Target
		r.add("code_packet", "passed", "packet_integrity_verified", "")
	} else {
		r.add("code_packet", "failed", "code_packet_unavailable", "Preserve o ensaio e compartilhe este diagnóstico; prepare um novo ensaio com o kit verificado.")
	}
	archive, archiveErr := packaging.ValidateArchive(filepath.Join(root, "exports", "maestro-product.mcpb"))
	if archiveErr == nil && archive.Format == "mcpb" && archive.Target == r.ReferenceHost && codeOK && archive.ServerSHA256 == packet.ServerSHA256 {
		r.add("desktop_archive", "passed", "archive_integrity_verified_not_client_loading", "")
	} else {
		r.add("desktop_archive", "failed", "desktop_archive_unavailable", "Preserve o ensaio e prepare os pacotes novamente em uma nova pasta usando o kit verificado.")
	}

	// Always diagnose admission, including when init never produced preparation.json.
	home, homeErr := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome})
	var inspection workspace.Inspection
	inspectErr := errors.New("home_not_admitted")
	if homeErr == nil && home.Writable {
		inspection, inspectErr = workspace.Inspect(o.WorkspacePath, filepath.Join(home.Home, "state"))
	}
	bindingOK := homeErr == nil && home.Writable && inspectErr == nil && inspection.State == "ready" && pilotWorkspaceID.MatchString(inspection.WorkspaceID)
	if bindingOK {
		r.add("binding_admission", "passed", "existing_binding_admitted", "")
	} else {
		reason := datahome.DiagnosticReason(homeErr)
		if reason == "" {
			reason = "workspace_unavailable"
		}
		r.add("binding_admission", "failed", reason, "Preserve a pasta e envie o diagnóstico; não altere permissões ou execute como administrador para contornar a recusa.")
	}
	var prepared struct {
		State       string `json:"state"`
		WorkspaceID string `json:"workspace_id"`
	}
	body, readErr := readPilotFile(filepath.Join(root, "preparation.json"), 4096)
	preparedOK := readErr == nil && json.Unmarshal(body, &prepared) == nil && prepared.State == "ready" && pilotWorkspaceID.MatchString(prepared.WorkspaceID)
	if preparedOK && bindingOK && prepared.WorkspaceID == inspection.WorkspaceID {
		r.add("preparation", "passed", "workspace_matches_preparation", "")
	} else {
		r.add("preparation", "failed", "preparation_missing_or_workspace_mismatch", "A preparação não foi confirmada para este workspace. Preserve os arquivos e compartilhe o diagnóstico.")
		preparedOK = false
	}
	if archiveErr == nil && archive.DesktopPilot != nil {
		pilot := archive.DesktopPilot
		if !bindingOK || !preparedOK || pilot.UserHome != o.UserHome || pilot.Workspace != o.WorkspacePath || pilot.WorkspaceID != inspection.WorkspaceID {
			r.add("desktop_binding", "failed", "desktop_pilot_binding_mismatch", "Preserve o ensaio e prepare um novo pacote para este workspace.")
		} else {
			r.DesktopExtension = pilot.DisplayName()
			r.add("desktop_binding", "passed", "isolated_desktop_defaults_match_preparation", "")
		}
	}
	if _, err := exec.LookPath("claude"); err == nil {
		r.add("code_cli", "passed", "code_cli_found_login_not_tested", "")
	} else {
		r.add("code_cli", "not_tested", "code_cli_not_found", "Instale o Claude Code pelo fluxo normal autorizado pela organização antes de abrir Abrir Code.")
	}
	if codeOK && bindingOK && preparedOK {
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := probePilotPacket(probeCtx, codeRoot, o, prepared.WorkspaceID, &r); err != nil {
			r.add("mcp_probe", "failed", "packet_mcp_probe_failed", "Feche os clientes do ensaio, execute Testar Maestro novamente e envie este diagnóstico se a recusa persistir.")
		}
	} else {
		r.add("mcp_connection", "not_tested", "preparation_required", "Conclua a preparação admitida antes de testar o processo MCP.")
	}
	r.add("code_client", "not_tested", "native_client_not_observed", "Abra Abrir Code, faça login pelo fluxo normal e execute o prompt Code gerado após a verificação.")
	r.add("desktop_client", "not_tested", "native_client_not_observed", "Abra Abrir Desktop, instale a extensão deste ensaio e execute o prompt Desktop após o teste Code.")
	for _, name := range []string{"onboarding", "skills", "hooks", "authored_brain", "memory"} {
		r.add(name, "not_tested", "native_journey_not_observed", "Registre o resultado da jornada guiada nos clientes reais; a verificação automática não executa esta funcionalidade.")
	}
	return finishPilotReport(&r, output)
}

var pilotWorkspaceID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var pilotKitID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func readPilotFile(path string, limit int64) ([]byte, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := scheduler.ValidateSingleLink(f); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, errors.New("pilot_file_unavailable")
	}
	return body, nil
}

func pilotPacketCommand(root string, o product.Options) (string, []string, error) {
	body, err := readPilotFile(filepath.Join(root, ".mcp.json"), 32<<10)
	if err != nil {
		return "", nil, err
	}
	var configuration struct {
		Servers map[string]struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err = json.Unmarshal(body, &configuration); err != nil {
		return "", nil, err
	}
	server, ok := configuration.Servers["maestro"]
	name := "maestro-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if !ok || len(configuration.Servers) != 1 || server.Type != "stdio" || server.Command != "${CLAUDE_PLUGIN_ROOT}/scripts/"+name {
		return "", nil, errors.New("pilot_command_invalid")
	}
	replace := strings.NewReplacer("${CLAUDE_PLUGIN_ROOT}", root, "${user_config.user_home}", o.UserHome, "${user_config.workspace}", o.WorkspacePath)
	args := make([]string, len(server.Args))
	for i, arg := range server.Args {
		args[i] = replace.Replace(arg)
	}
	want := []string{"serve", "--packet", "--product-root", root, "--user-home", o.UserHome, "--workspace", o.WorkspacePath}
	if !reflect.DeepEqual(args, want) {
		return "", nil, errors.New("pilot_binding_invalid")
	}
	return filepath.Join(root, "scripts", name), args, nil
}

func probePilotPacket(ctx context.Context, root string, o product.Options, id string, r *pilotReport) error {
	command, args, err := pilotPacketCommand(root, o)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stderr = io.Discard // Never export child logs, model prompts or private paths.
	client := mcp.NewClient(&mcp.Implementation{Name: "maestro-pilot-check", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	if err != nil {
		return err
	}
	defer session.Close()
	initialized := session.InitializeResult()
	if initialized == nil || initialized.ProtocolVersion == "" || initialized.Capabilities == nil || initialized.Capabilities.Tools == nil || initialized.Capabilities.Resources == nil {
		return errors.New("pilot_capabilities_unavailable")
	}
	var metadata struct {
		WorkspaceID string `json:"workspace_id"`
		State       string `json:"state"`
	}
	if err := pilotCall(ctx, session, "maestro_context", map[string]any{}, &metadata); err != nil || metadata.WorkspaceID != id || metadata.State != "ready" {
		return errors.New("pilot_context_mismatch")
	}
	r.WorkspaceID = id
	r.add("mcp_connection", "passed", "actual_packet_sdk_context_verified", "")
	tools := map[string]bool{}
	toolParams := &mcp.ListToolsParams{}
	seen := map[string]bool{}
	for pages := 0; ; pages++ {
		if pages >= 8 {
			return errors.New("pilot_tool_bound")
		}
		page, err := session.ListTools(ctx, toolParams)
		if err != nil {
			return err
		}
		if len(page.Tools) > 32 {
			return errors.New("pilot_tool_bound")
		}
		for _, tool := range page.Tools {
			tools[tool.Name] = true
		}
		if page.NextCursor == "" {
			break
		}
		if len(page.NextCursor) > 4096 || seen[page.NextCursor] {
			return errors.New("pilot_tool_cursor")
		}
		seen[page.NextCursor] = true
		toolParams.Cursor = page.NextCursor
	}
	for _, name := range []string{"maestro_context", "maestro_query", "maestro_apply"} {
		if !tools[name] {
			return errors.New("pilot_tool_missing")
		}
	}
	r.add("mcp_tools", "passed", "public_tools_discovered_no_apply_called", "")
	resources := map[string]bool{}
	resourceParams := &mcp.ListResourcesParams{}
	seen = map[string]bool{}
	for pages := 0; ; pages++ {
		if pages >= 16 {
			return errors.New("pilot_resource_bound")
		}
		page, err := session.ListResources(ctx, resourceParams)
		if err != nil {
			return err
		}
		if len(page.Resources) > 512 {
			return errors.New("pilot_resource_bound")
		}
		for _, resource := range page.Resources {
			resources[resource.URI] = true
		}
		if len(resources) > 4096 {
			return errors.New("pilot_resource_bound")
		}
		if page.NextCursor == "" {
			break
		}
		if len(page.NextCursor) > 4096 || seen[page.NextCursor] {
			return errors.New("pilot_resource_cursor")
		}
		seen[page.NextCursor] = true
		resourceParams.Cursor = page.NextCursor
	}
	if !resources["maestro://methods/METHODS.md"] {
		return errors.New("pilot_resource_missing")
	}
	resource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "maestro://methods/METHODS.md"})
	if err != nil || len(resource.Contents) != 1 || resource.Contents[0].URI != "maestro://methods/METHODS.md" || len(resource.Contents[0].Text) == 0 || len(resource.Contents[0].Text) > 4096 {
		return errors.New("pilot_resource_read_failed")
	}
	r.add("mcp_resources", "passed", "public_resources_discovered", "")
	methods, err := product.PackagedMethods()
	if err != nil {
		return err
	}
	var got []string
	for offset := 0; ; {
		if offset > 128 {
			return errors.New("pilot_method_bound")
		}
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Next int  `json:"next_offset"`
			More bool `json:"has_more"`
		}
		if err := pilotCall(ctx, session, "maestro_query", map[string]any{"operation": "methods.list", "offset": offset, "limit": 32}, &page); err != nil {
			return err
		}
		if len(page.Items) > 32 || page.Next != offset+len(page.Items) || page.More && page.Next <= offset {
			return errors.New("pilot_method_page")
		}
		for _, method := range page.Items {
			got = append(got, method.ID)
		}
		if !page.More {
			break
		}
		offset = page.Next
	}
	want := make([]string, 0, len(methods.Methods))
	for _, method := range methods.Methods {
		want = append(want, method.ID)
	}
	if !reflect.DeepEqual(got, want) {
		return errors.New("pilot_method_catalog_mismatch")
	}
	r.add("method_catalog", "passed", "packaged_method_catalog_verified_not_executed", "")
	return nil
}

// CommandTransport retains the SDK's finite 16-MiB JSON-RPC frame cap. Each
// call below additionally admits one small JSON text result and finite pages.
func pilotCall(ctx context.Context, s *mcp.ClientSession, name string, args map[string]any, out any) error {
	result, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err
	}
	if result.IsError || len(result.Content) != 1 {
		return errors.New("pilot_tool_result_invalid")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || len(text.Text) > 256<<10 {
		return errors.New("pilot_tool_result_bound")
	}
	return json.Unmarshal([]byte(text.Text), out)
}

func finishPilotReport(r *pilotReport, output io.Writer) error {
	for _, check := range r.Checks {
		if check.Status == "failed" {
			r.Status = "blocked"
		}
	}
	if r.Status != "blocked" && pilotWorkspaceID.MatchString(r.WorkspaceID) {
		r.CodePrompt, r.DesktopPrompt = pilotPrompts(r.WorkspaceID)
		if r.DesktopExtension != "" {
			r.DesktopPrompt = fmt.Sprintf("Use a extensão %q deste ensaio e confirme os caminhos preenchidos na importação. ", r.DesktopExtension) + r.DesktopPrompt
		}
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Maestro MCP — diagnóstico do ensaio\n\nStatus: **%s**.\n\n", r.Status)
	fmt.Fprintf(&md, "Máquina do diagnóstico: `%s/%s`.\n\n", r.ReferenceHost.OS, r.ReferenceHost.Arch)
	if r.Target.OS != "" {
		fmt.Fprintf(&md, "Destino do pacote verificado: `%s/%s`.\n\n", r.Target.OS, r.Target.Arch)
	}
	if r.KitID != "" {
		fmt.Fprintf(&md, "Identificador local do kit: `%s` (diagnóstico; não comprova a identidade do publicador).\n\n", r.KitID)
	} else {
		md.WriteString("Identificador do kit: não verificado.\n\n")
	}
	if r.WorkspaceID != "" {
		fmt.Fprintf(&md, "Workspace verificado: `%s`.\n\n", r.WorkspaceID)
	}
	if r.DesktopExtension != "" {
		fmt.Fprintf(&md, "Extensão Desktop deste ensaio: **%s**. Confirme os caminhos preenchidos ao importar.\n\n", r.DesktopExtension)
	}
	md.WriteString("| Verificação | Estado | Motivo | Próxima ação |\n| --- | --- | --- | --- |\n")
	for _, check := range r.Checks {
		fmt.Fprintf(&md, "| %s | %s | %s | %s |\n", check.Name, check.Status, check.Reason, check.NextAction)
	}
	md.WriteString("\nEste diagnóstico faz apenas leituras locais. Integridade do MCPB e conexão SDK com o pacote Code não comprovam carregamento, login, permissões, hooks ou retomada nos clientes nativos. Os prompts Code e Desktop guiam esse teste com dados sintéticos e confirmação normal do cliente.\n")
	r.Markdown = md.String()
	if err := json.NewEncoder(output).Encode(r); err != nil {
		return err
	}
	if r.Status == "blocked" {
		return errors.New("pilot_check_blocked")
	}
	return nil
}

func pilotPrompts(id string) (string, string) {
	marker := "MAESTRO_PILOT_" + id
	page := "knowledge/maestro-pilot-" + id + ".md"
	common := fmt.Sprintf("Use somente o Maestro deste ensaio. Primeiro chame maestro_context e confirme workspace_id exatamente %s e state ready. Se houver divergência, conector ambíguo, erro, limite de paginação ou estado inesperado, pare e explique, sem alterar outros casos. Não use outros conectores, shell, arquivos privados ou dados reais. Inspecione os schemas atuais de maestro_query/maestro_apply. Use execution.list em páginas de até 32 (máximo 128 itens); use execution.next para identificar um trabalho deste ensaio pelo marcador literal %s no next_step ou summary. Não copie IDs ou revisões manualmente: use os retornados pelas ferramentas. ", id, marker)
	journey := fmt.Sprintf("Quero testar explicitamente os métodos maestro-onboarding, maestro-operator e pocock-domain-modeling. Descubra cada método com methods.plan e leia suas instruções via methods.read usando somente URIs retornados e páginas limitadas. Métodos não concedem autorização extra. Conduza o onboarding sintético, uma pergunta por vez; proponha somente dados fictícios, preserve um perfil já existente e não declare conclusão sem a revisão e confirmação humanas exigidas. Demonstre pocock-domain-modeling com três entidades de um projeto fictício. Pelo método maestro-operator e o contrato de brain disponível, proponha criar a página sintética %q com título Ensaio Maestro e uma seção contendo o marcador literal %q; use um template real e os metadados autorais obrigatórios; confirme que o caminho é admitido, mostre a revisão humana exigida e só aplique após confirmação normal. Não invente tokens de aprovação. Se a página já existir, apenas leia e compare, sem sobrescrever. Leia de volta pelo MCP e confirme o marcador exato. Se a capacidade estiver indisponível, registre esse bloqueio e pare antes da continuidade. Hooks e memória devem ser reportados como não testados se não houver evidência real de execução; não os acione ou declare sucesso só porque estão configurados. Depois prossiga com o trabalho sintético abaixo. ", page, marker)
	code := common + journey + fmt.Sprintf("Este prompt autoriza criar e pausar somente um trabalho sintético. Se já houver exatamente um trabalho com esse marcador, relate seu estado e pare sem repetir escritas; se houver duplicidade, pare. Se não houver, chame execution.create com objective %q, next_step %q e artifacts [%q]. Use o item_id/revisão retornados para execution.start; com a tentativa e revisão atuais, execution.checkpoint com summary %q e next_step %q; depois execution.pause com a revisão atual e mesma tentativa. Cada escrita ocorre uma única vez, com expected_revision atual, e respeita a confirmação normal do cliente. Não marque concluído nem registre evidência fictícia. Ao final, leia execution.next e mostre workspace_id, item_id, estado, revisão e resumo; confirme paused. O próximo passo humano é abrir Desktop e usar o prompt Desktop gerado.", marker, marker, "brain/"+page, marker, marker)
	desktop := common + fmt.Sprintf("Primeiro recupere pelo contrato MCP de brain a página %q, somente leitura, e confirme que a seção do ensaio preserva o marcador literal %q; se ausente, divergente ou inacessível, pare. Este prompt autoriza somente retomar o trabalho sintético criado no Code. Exija exatamente um trabalho correspondente e summary exatamente %q. Se ausente ou duplicado, pare e peça o teste Code; se já running, apenas relate sem retomar novamente. Se paused, chame execution.resume uma única vez, com item_id e expected_revision atuais, respeitando a confirmação normal do cliente. Depois leia execution.next e confirme running, nova tentativa e resumo preservado. Não salve novo checkpoint, não conclua nem altere outros trabalhos. Mostre o resultado curto para anexar ao diagnóstico. Isso comprova a leitura da página e retomada entre clientes apenas quando executado nos dois clientes reais.", page, marker, marker)
	return code, desktop
}
