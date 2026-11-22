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

// One selected prompt is delivered per human turn; this is not persisted run state.
type pilotStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
}

type pilotCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action"`
}

// This is shareable diagnostic metadata, never a dump of context, logs or paths.
type pilotReport struct {
	SchemaVersion      int                                  `json:"schema_version"`
	Status             string                               `json:"status"`
	Checks             []pilotCheck                         `json:"checks"`
	WorkspaceID        string                               `json:"workspace_id"`
	KitID              string                               `json:"kit_id"`
	ReferenceHost      packaging.Target                     `json:"reference_host"`
	Target             packaging.Target                     `json:"target"`
	ClientTarget       string                               `json:"client_target,omitempty"`
	Markdown           string                               `json:"markdown"`
	CodeSteps          []pilotStep                          `json:"code_steps,omitempty"`
	DesktopSteps       []pilotStep                          `json:"desktop_steps,omitempty"`
	CodePrompt         string                               `json:"code_prompt"`
	CodeReadbackPrompt string                               `json:"code_readback_prompt"`
	CodeReadyPrompt    string                               `json:"code_ready_prompt"`
	DesktopPrompt      string                               `json:"desktop_prompt"`
	DesktopReadyPrompt string                               `json:"desktop_ready_prompt"`
	DesktopExtension   string                               `json:"desktop_extension,omitempty"`
	Runtime            *packaging.EngineeringRuntimeBinding `json:"runtime,omitempty"`
	Conversion         *pilotConversionExpectation          `json:"conversion,omitempty"`
}

func (r *pilotReport) add(name, status, reason, action string) {
	r.Checks = append(r.Checks, pilotCheck{Name: name, Status: status, Reason: reason, NextAction: action})
}

// runPilotCheck only inspects an explicitly prepared pilot layout. Wrappers own
// report files; this command never initializes, repairs or writes participant state.
func runPilotCheck(ctx context.Context, args []string, output io.Writer) error {
	r := pilotReport{SchemaVersion: 1, Status: "partial", Checks: []pilotCheck{}, ReferenceHost: packaging.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}}
	running, err := processidentity.Current()
	if err != nil {
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
	codeRoot, packet, packetErr := pilotCodePacket(o, r.ReferenceHost, running.SHA256)
	codeOK := packetErr == nil
	if codeOK {
		r.Target = packet.Target
		r.add("code_packet", "passed", "packet_integrity_verified", "")
		r.add("code_profile", "passed", packet.NativeMode, "O perfil descreve o pacote selecionado; o carregamento no cliente continua sendo verificado separadamente.")
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
			r.add("desktop_binding", "passed", "isolated_desktop_binding_matches_preparation", "")
		}
		if experiment := pilot.Runtime; experiment != nil {
			expectedServer := filepath.Join(o.ProductRoot, "server", "maestro-mcp")
			if runtime.GOOS == "windows" {
				expectedServer += ".exe"
			}
			if !codeOK || !bindingOK || !preparedOK || running.Path != expectedServer || running.SHA256 != experiment.ServerSHA256 || Version != experiment.ClientVersion || AuthorityRegistrySHA256 != experiment.RegistrySHA256 {
				r.add("document_runtime", "failed", "engineering_runtime_image_or_binding_mismatch", "Preserve o ensaio e solicite um kit experimental novo; não altere os caminhos ou as permissões.")
			} else {
				seedDoclingAuthority(&o)
				if err := experiment.ValidateSelection(o); err != nil {
					r.add("document_runtime", "failed", "engineering_runtime_expired_or_changed", "Preserve seus dados e solicite um kit experimental novo; a verificação não repara nem amplia a validade.")
				} else {
					r.Runtime = experiment
				}
			}
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

func pilotCodePacket(o product.Options, host packaging.Target, serverSHA string) (string, packaging.PacketStatus, error) {
	root := filepath.Join(filepath.Dir(o.ProductRoot), "exports", "code")
	base, err := packaging.ValidatePacket(root)
	if err != nil || base.Format != "plugin" || base.Target != host || base.ServerSHA256 != serverSHA {
		return "", packaging.PacketStatus{}, errors.New("pilot Code packet unavailable")
	}
	native := filepath.Join(filepath.Dir(o.ProductRoot), "exports", "code-native-qualified")
	if _, err := os.Lstat(native); errors.Is(err, os.ErrNotExist) {
		return root, base, nil
	} else if err != nil {
		return "", packaging.PacketStatus{}, err
	}
	selected, err := packaging.ValidatePacket(native)
	if err != nil || selected.Format != "plugin" || selected.Target != host || selected.ServerSHA256 != serverSHA {
		return "", packaging.PacketStatus{}, errors.New("pilot native export unavailable")
	}
	switch selected.NativeMode {
	case "native":
		if err := packaging.ValidateNativeSelection(o, selected.NativeBindingSHA256); err != nil {
			return "", packaging.PacketStatus{}, err
		}
		return native, selected, nil
	case "mcp_only":
		return root, base, nil
	default:
		return "", packaging.PacketStatus{}, errors.New("pilot native export is not qualified")
	}
}

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
	status, err := packaging.ValidatePacket(root)
	if err != nil || status.Format != "plugin" {
		return "", nil, errors.New("pilot_packet_invalid")
	}
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
	if selected := status.CodePilot; selected != nil {
		if selected.UserHome != o.UserHome || selected.Workspace != o.WorkspacePath {
			return "", nil, errors.New("pilot_binding_invalid")
		}
		want = append(want, "--owner-review-channel", selected.OwnerReviewChannel)
	}
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
	if r.Runtime != nil {
		var availability struct {
			State string `json:"state"`
			Pack  string `json:"pack_sha256"`
		}
		if err := pilotCall(ctx, session, "maestro_query", map[string]any{"operation": "content.status"}, &availability); err != nil || availability.State != "ready" || availability.Pack != r.Runtime.PackSHA256 {
			r.add("document_runtime", "failed", "actual_engineering_converter_unavailable", "Preserve o ensaio e compartilhe o relatório; nenhuma instalação ou requalificação foi executada.")
		} else {
			r.add("document_runtime", "passed", "actual_packet_converter_verified_no_conversion_performed", "Consulte content.status para os formatos disponíveis; este diagnóstico não é um teste de conversão ou de cliente.")
		}
	}
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
	if r.ClientTarget != "" && r.ClientTarget != "desktop" {
		r.add("client_target", "failed", "client_target_invalid", "Preserve o diagnóstico e a seleção de clientes existente.")
	}
	if r.Conversion != nil {
		if r.Conversion.State == "blocked_identity_mismatch" {
			r.Status = "blocked"
		}
		if err := r.Conversion.validate(); err != nil {
			r.add("conversion", "failed", "conversion_expectation_invalid", "Preserve o diagnóstico; nenhuma conversão foi autorizada pelo menu.")
		}
	}
	for _, check := range r.Checks {
		if check.Status == "failed" {
			r.Status = "blocked"
		}
	}
	if r.Status != "blocked" && pilotWorkspaceID.MatchString(r.WorkspaceID) {
		steps := pilotSteps(r.WorkspaceID)
		bind := func(source []pilotStep, prefix string) []pilotStep {
			result := append([]pilotStep(nil), source...)
			for i := range result {
				result[i].Prompt = prefix + result[i].Prompt
			}
			return result
		}
		if r.ClientTarget != "desktop" {
			codeSteps := make([]pilotStep, 0, len(steps))
			for _, step := range steps {
				if step.ID == "execution-resume" {
					continue
				}
				if step.ID == "execution-pause" {
					step.Prompt += " Com paused confirmado, oriente o participante: pressione Enter no menu da etapa para voltar e escolha 2 — Continuar no Desktop no menu principal. A continuação acontece no Desktop, em outro pedido."
				}
				codeSteps = append(codeSteps, step)
			}
			r.CodeSteps = bind(codeSteps, "Use somente o servidor MCP plugin:maestro-product:maestro deste ensaio. ")
		}
		desktopSteps := steps
		if r.ClientTarget != "desktop" {
			desktopSteps = []pilotStep{steps[0], steps[len(steps)-2], steps[len(steps)-1]}
		}
		prefix := "Use somente o Maestro deste ensaio. "
		if r.DesktopExtension != "" {
			prefix = fmt.Sprintf("Use somente a extensão %q deste ensaio; identifique seu servidor MCP sem escolher conectores semelhantes. ", r.DesktopExtension)
		}
		r.DesktopSteps = bind(desktopSteps, prefix)

		if r.ClientTarget != "desktop" {
			r.CodeReadyPrompt = fmt.Sprintf("Use somente o servidor MCP plugin:maestro-product:maestro, declarado pelo plugin Maestro deste ensaio. Chame maestro_context uma única vez, sem parâmetros. Confirme workspace_id exatamente %s e state ready. Mostre o workspace_id e o estado retornados e encerre após essa leitura. Se esse servidor estiver ausente, ambíguo ou recusado, se houver erro ou se os valores divergirem, pare e explique. Esta conferência autoriza somente essa leitura de contexto.", r.WorkspaceID)
		}
		if r.DesktopExtension != "" {
			r.DesktopReadyPrompt = fmt.Sprintf("Use somente a extensão %q deste ensaio, vinculada às pastas aprovadas na preparação. Identifique com segurança o servidor MCP dessa extensão; não escolha outra extensão ou conector de nome semelhante. Chame maestro_context uma única vez, sem parâmetros. Confirme workspace_id exatamente %s e state ready. Mostre somente o workspace_id e o estado de saúde da conexão retornados e encerre após essa leitura. O estado dos trabalhos não foi consultado. Se essa extensão estiver ausente, ambígua ou recusada, se não conseguir identificar seu conector, se houver erro ou se os valores divergirem, pare e explique. Esta conferência autoriza somente essa leitura de contexto.", r.DesktopExtension, r.WorkspaceID)
		}
		conversion := ""
		if r.Runtime != nil {
			conversion = fmt.Sprintf("Consulte content.status; continue somente com state ready e pack_sha256 exatamente %s. Converta somente maestro-pilot.docx por maestro_apply content.convert uma única vez. Leia o artifact_ref realmente retornado com content.read nos formatos markdown e document, max_bytes 4096, seguindo next_offset enquanto has_more. Exija paginação completa, marcador MAESTRO_DOCLING_OFFLINE_20261101 e tabela alpha/7. Mostre rota, artefato e resultado; disponibilidade do pacote não comprova conversão.", r.Runtime.PackSHA256)
		}
		if r.Conversion != nil {
			if r.Conversion.State == "ready_for_participant_test" {
				conversion = installedConversionJourney(*r.Conversion)
			} else {
				conversion = "Conversão de documento: não testada, pois o runtime atual está pendente ou indisponível. Registre a pendência e encerre sem preparar documento ou executar conversão."
			}
		}
		if conversion != "" {
			step := pilotStep{ID: "conversion", Title: "Conferir um documento", Prompt: pilotStepScope(r.WorkspaceID) + conversion + " Encerre este turno após o resultado desta etapa."}
			if r.ClientTarget != "desktop" {
				r.CodeSteps = append(r.CodeSteps, bind([]pilotStep{step}, "Use somente o servidor MCP plugin:maestro-product:maestro deste ensaio. ")[0])
			}
			r.DesktopSteps = append(r.DesktopSteps, bind([]pilotStep{step}, prefix)[0])
		}
		// Preserve the existing JSON string fields, with a safe read-only entry.
		if len(r.CodeSteps) != 0 {
			r.CodePrompt = r.CodeSteps[0].Prompt
			for _, step := range r.CodeSteps {
				if step.ID == "readback" {
					r.CodeReadbackPrompt = step.Prompt
					break
				}
			}
		}
		r.DesktopPrompt = r.DesktopSteps[0].Prompt

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
		fmt.Fprintf(&md, "Extensão Desktop deste ensaio: **%s**. Confirme sua instalação; ela já está vinculada às pastas aprovadas na preparação.\n\n", r.DesktopExtension)
	}
	if r.Runtime != nil {
		fmt.Fprintf(&md, "Runtime experimental: versão `%s`, alvo `%s/%s`, expiração `%s`. Identidade do pack: `%s`. A conferência apenas verificou sua disponibilidade; a conversão real fica na jornada guiada com o DOCX sintético. A chave experimental não é assinatura organizacional nem atualização de produção.\n\n", r.Runtime.ClientVersion, r.Runtime.Target.OS, r.Runtime.Target.Arch, r.Runtime.ExpiresAt, r.Runtime.PackSHA256)
	}
	if r.Conversion != nil {
		fmt.Fprintf(&md, "Jornada de conversão: `%s`. Expectativas do Current autenticado: manifest `%s`, pack `%s`; documento público `%s`, SHA256 `%s`. Metadados de diagnóstico e roteiro, sem autoridade de instalação ou prova de conversão. O verificador não cria o documento no workspace; somente a ação explícita de teste completo prepara uma cópia sem substituir arquivos. Conversão e leitura paginada reais continuam pendentes até o ensaio no cliente.\n\n", r.Conversion.State, r.Conversion.ManifestSHA256, r.Conversion.PackSHA256, r.Conversion.Source, r.Conversion.SourceSHA256)
	}
	md.WriteString("| Verificação | Estado | Motivo | Próxima ação |\n| --- | --- | --- | --- |\n")
	for _, check := range r.Checks {
		fmt.Fprintf(&md, "| %s | %s | %s | %s |\n", check.Name, check.Status, check.Reason, check.NextAction)
	}
	if r.ClientTarget == "desktop" {
		md.WriteString("\nCliente selecionado: **Desktop**. Este diagnóstico faz apenas leituras locais; o conector e o catálogo verificados não comprovam carregamento no cliente. O prompt Desktop inicia o ensaio sintético sem exigir trabalho anterior em outro cliente. Hooks do Code e agentes com runtime independente exigem sua própria preparação e não foram qualificados por este diagnóstico.\n")
		md.WriteString("\nUse a conferência de conexão Desktop para uma leitura do workspace e da saúde da conexão, ou o ensaio Desktop para testar métodos, página, memória e continuidade com confirmação normal do cliente. Para um reinício frio, feche somente a conexão Desktop deste ensaio pelo fluxo normal, preservando outros trabalhos. Conversa nova não comprova reinício do servidor; registre o reinício informado separadamente. Um diagnóstico bloqueado deve ser enviado ao time antes de abrir o cliente.\n")
	} else {
		md.WriteString("\nEste diagnóstico faz apenas leituras locais. Integridade do MCPB e conexão SDK com o pacote Code não comprovam carregamento, login, permissões, hooks ou retomada nos clientes nativos. Os prompts Code e Desktop guiam esse teste com dados sintéticos e confirmação normal do cliente.\n")
		md.WriteString("\nPara uma conferência rápida somente leitura, use 3 — Conferir conexão Code ou 4 — Conferir conexão Desktop. Esses atalhos mostram o workspace e a saúde da conexão, sem consultar trabalhos. Para o ensaio completo, use 1 — Testar ou continuar no Code e depois 2 — Continuar no Desktop. Cada escolha abre apenas a etapa atual; avance conscientemente após conferir o resultado no cliente. Uma interrupção volta à conferência somente leitura, sem repetição automática. Depois, use 6 — Conferir persistência no Code (somente leitura), sem copiar IDs ou outro texto. Para um reinício frio, feche antes somente as sessões Code e conexões Desktop deste ensaio pelo fluxo normal, preservando outros trabalhos. Abrir uma conversa nova não comprova reinício do servidor; registre o reinício informado pelo participante separadamente. Um diagnóstico bloqueado deve ser enviado ao time antes de abrir clientes.\n")
	}
	r.Markdown = md.String()
	if err := json.NewEncoder(output).Encode(r); err != nil {
		return err
	}
	if r.Status == "blocked" {
		return errors.New("pilot_check_blocked")
	}
	return nil
}

func pilotStepScope(id string) string {
	return fmt.Sprintf("ETAPA ATUAL SOMENTE. Chame maestro_context e confirme workspace_id exatamente %s e state ready. Inspecione os schemas atuais. Use apenas dados sintéticos e este conector. Ambiguidade, erro, divergência, timeout, confirmation_unavailable ou resultado incerto encerram TODO este turno: informe pendente, preserve a evidência anterior e volte à conferência somente leitura em outro turno. Faça somente a operação desta etapa, uma vez, com a revisão humana normal; sem repetição automática ou aprovação inventada. Ao terminar, mostre passou, falhou ou pendente e encerre. ", id)
}

func pilotSteps(id string) []pilotStep {
	marker := "MAESTRO_PILOT_" + id
	page := "knowledge/maestro-pilot-" + id + ".md"
	accepted, declined := marker+"_MEMORY_ACCEPT", marker+"_MEMORY_DECLINE"
	acceptedText := "Preferência inteiramente fictícia: conferir o resultado salvo antes da entrega. " + accepted
	declinedText := "Memória inteiramente fictícia do controle de recusa; este registro deve ser recusado. " + declined
	selection := fmt.Sprintf("Leia execution.list em páginas de até 32, seguindo next_offset enquanto has_more, máximo 128 itens. execution.list retorna somente metadados. Chame execution.next somente com item_id retornado, para cada item_id listado; se a lista estiver vazia, pule execution.next. Somente após ler todas as projeções selecione summary ou next_step exatamente %q. Paginação incompleta ou duplicidade encerra o turno; zero resultados significa ausente. Use IDs, revisões e tentativa realmente retornados, nunca a conversa anterior. ", marker)
	memory := fmt.Sprintf("Use memory.search para cada marcador exato %q e %q, limit 32, seguindo next_offset até -1, máximo 128 resultados. Conte apenas após paginação completa. Leia cada captura retornada por memory.read com capture_id igual ao id, expected_revision atual e max_bytes 4096, seguindo next_offset até terminar. O único texto aceito esperado é %q. Duplicidade, texto divergente ou presença do controle de recusa é FALHA DO CONTROLE: preserve tudo e pare, não apague. Ausência não comprova recusa explícita; sem confirmation_declined observado, registre recusa não atestada. ", accepted, declined, acceptedText)
	pageRead := fmt.Sprintf("Leia brain.read para page %q e purpose de conferir o ensaio, em páginas de até 4096 bytes, seguindo next_offset até terminar. Registre a revisão; verifique o marcador %q e o modelo completo com três entidades, definições e relações. Qualquer erro ou paginação incompleta encerra o turno sem inferir ausência nem alterar a página. ", page, marker)
	expectedPage := fmt.Sprintf("Exija exatamente um trabalho identificado pelo marcador e summary exatamente %q; sem essa condição, encerre antes de ler a página esperada. ", marker) + pageRead
	method := func(ids string) string {
		return "Descubra " + ids + " por methods.plan com method_id exato, uma chamada por método; leia methods.read com resource_uri igual ao instruction_uri retornado, max_bytes 4096 e next_offset até terminar. Métodos não concedem autorização extra; consulta MCP não comprova invocação nativa de Skills, hooks ou agentes. "
	}
	step := func(key, title, body string) pilotStep {
		return pilotStep{ID: key, Title: title, Prompt: pilotStepScope(id) + body + " Encerre após esta etapa; a próxima exige outro pedido pelo menu."}
	}
	reconcile := "Esta etapa é somente leitura: use maestro_context e maestro_query. Consulte owner.status e brain.status. O estado do brain não estabelece existência da página específica: reporte página do ensaio: não verificada. Catálogo, templates ou navegação também não provam presença nem ausência dessa página. " + selection + memory + "Mostre apenas o estado atual verificado de perfil, capturas e trabalho, incluindo estado e revisão. Preserve separadamente a tentativa anterior falha e os resultados atuais. Nenhuma ausência autoriza repetir uma operação incerta. Informe o que ainda precisa de decisão humana, sem executar outra etapa. O reinício informado pelo participante permanece não atestado; uma conversa nova não comprova reinício do servidor."
	return []pilotStep{
		step("reconcile", "Conferir ou continuar", reconcile),
		step("onboarding", "Preparar meu perfil", method("maestro-onboarding")+"Consulte owner.status e preserve perfil já existente. Retome exatamente next_question, uma pergunta fictícia por turno e uma resposta revisada por vez. Use o fluxo do método para esta resposta somente; revisão final e confirmação são turnos separados. Se review_required, apresente a revisão final e peça a confirmação humana normal; se complete, relate completo sem reabrir. Declínio preserva o progresso. O participante repete esta etapa até owner.status confirmar conclusão; uma proposta não é conclusão."),
		step("page", "Criar e conferir a página", "Se esta etapa foi interrompida ou a página já é conhecida, faça somente a conferência de leitura; não repita uma criação incerta. Uma criação nova exige a escolha consciente do participante. "+method("maestro-operator e pocock-domain-modeling")+fmt.Sprintf("Demonstre um projeto fictício com três entidades, definições e relações. Consulte brain.templates e proponha page %q, título Ensaio Maestro, uma seção com o marcador literal %q e seção Modelo do ensaio com o modelo completo, usando template retornado e metadados autorais obrigatórios. A primeira operação de escrita na página deve ser brain.create uma única vez com page e text, pela revisão humana normal. Página existente ou criada concorrentemente é preservada; relate existente sem atualizar. Após sucesso confirmado, leia brain.read com page e purpose, max_bytes 4096 e next_offset até terminar. Compare texto completo efetivamente aprovado, modelo e revisão retornada; somente o marcador é insuficiente. Se o recibo for incerto, reporte a incerteza e encerre; confira somente leitura em outro turno, sem nova gravação.", page, marker)),
		step("memory-accept", "Salvar e conferir a memória de teste", memory+fmt.Sprintf("Se a captura aceita já existir exatamente uma vez com o texto esperado, relate já salva sem escrever. Se ambos os marcadores estiverem ausentes e esta for uma tentativa nova conscientemente escolhida pelo participante após a conferência, explique que deve aprovar o registro e chame memory.capture uma única vez com text exatamente %q. Leia de volta o capture_id e revision reais por memory.read paginado, max_bytes 4096, comparando o texto completo. Recusa explícita deste registro é pendência, sem outra operação.", acceptedText)),
		step("memory-decline", "Testar a recusa da memória", memory+fmt.Sprintf("Esta é uma escolha separada do participante; ausência da captura aceita não impede este controle. Se o controle de recusa estiver ausente, explique que o participante deve RECUSAR explicitamente este registro, e chame memory.capture uma única vez com text exatamente %q. Somente confirmation_declined por recusa explícita observada qualifica este controle. Timeout, cancelamento ou fechamento encerram o turno como pendente, sem repetição. Aprovação inesperada é FALHA DO CONTROLE: preserve o registro e pare. Após decisão explícita, pesquise novamente o marcador de recusa com paginação completa para verificar ausência; ausência sozinha nunca qualifica recusa. Este controle não impede outras etapas separadamente autorizadas.", declinedText)),
		step("execution-create", "Abrir o trabalho de teste", selection+fmt.Sprintf("Se houver um trabalho, relate seu estado sem escrever. Se ausente, esta escolha explícita autoriza somente execution.create uma vez com objective %q, next_step %q e artifacts [%q]. Não repita uma criação incerta; volte à conferência. Leia execution.next com item_id retornado e mostre o estado e state_revision reais.", marker, marker, "brain/"+page)),
		step("execution-start", "Iniciar o trabalho", selection+"Exija um único trabalho em estado ready; outro estado é pendência ou já realizado, sem escrita. Chame somente execution.start com item_id e expected_revision atuais uma vez; leia execution.next para o mesmo item e confirme running e a tentativa retornada."),
		step("execution-checkpoint", "Guardar o ponto de parada", selection+fmt.Sprintf("Exija um único trabalho running com attempt_id atual; se summary já for %q, relate ponto já salvo sem escrita. Chame somente execution.checkpoint uma vez com item_id, expected_revision e attempt_id atuais, summary %q e next_step %q. Leia execution.next e confirme resumo, next_step e revisão reais.", marker, marker, marker)),
		step("execution-pause", "Pausar o trabalho", selection+fmt.Sprintf("Exija um único trabalho running e summary exatamente %q. Se já paused, relate sem escrita. Chame somente execution.pause uma vez com item_id, expected_revision e attempt_id atuais; leia execution.next e confirme paused e resumo preservado.", marker)),
		step("execution-resume", "Retomar o trabalho", selection+expectedPage+fmt.Sprintf("Exija um único trabalho paused e summary exatamente %q; se running, relate já retomado sem escrita. Outros estados encerram como pendência. Chame somente execution.resume uma vez com item_id e expected_revision atuais, depois leia execution.next para o mesmo item e confirme running, nova tentativa e resumo preservado. A retomada só comprova passagem entre clientes quando observada nos dois clientes reais.", marker)),
		step("readback", "Conferir o que ficou salvo", reconcile+expectedPage+"Após leitura completa bem-sucedida, reporte a página verificada e sua revisão. Para aprovar persistência, exija página completa, exatamente uma captura aceita íntegra, zero controles de recusa e um trabalho único; pendências são preservadas. Releia execution.next do mesmo item e confirme estado e revisão inalterados nesta conferência. Memória lida é somente L0, sem prova de injeção passiva, dreaming ou manutenção."),
	}
}
