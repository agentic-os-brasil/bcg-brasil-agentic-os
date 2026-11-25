package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type installedPilotSelection struct {
	ProductRoot, UserHome, Workspace, CarrierID, InstallerVersion string
	AdmissionSHA256, InstalledRoot                                string
	Functional, CheckOnly                                         bool
	ObservationClient                                             string
}

type installedPilotReport struct {
	pilotReport
	Profile             string `json:"profile"`
	Evidence            string `json:"evidence_class"`
	Protocol            string `json:"protocol"`
	AdmissionSHA256     string `json:"admission_sha256,omitempty"`
	CurrentRelease      string `json:"current_release,omitempty"`
	CurrentManifest     string `json:"current_manifest_sha256,omitempty"`
	CurrentBackend      string `json:"current_backend_sha256,omitempty"`
	CodeRegistration    string `json:"code_registration"`
	SessionLoading      string `json:"session_loading"`
	NativeQualification string `json:"native_qualification"`
	FunctionalCheck     string `json:"functional_check"`
	CheckMode           string `json:"check_mode"`
	FunctionalItemID    string `json:"functional_item_id,omitempty"`
	FunctionalStage     string `json:"functional_stage,omitempty"`
	FunctionalReason    string `json:"functional_reason,omitempty"`
	PrivateDiagnostic   string `json:"private_diagnostic,omitempty"`
}

func parseInstalledPilotSelection(args []string) (installedPilotSelection, error) {
	var out installedPilotSelection
	if len(args) > 32 {
		return out, errors.New("installed_selection_invalid")
	}
	allowed := map[string]bool{"product-root": true, "user-home": true, "workspace": true, "carrier-id": true, "installer-version": true, "experimental-seed-sha256": true, "experimental-installed-root": true, "check-only": true, "functional": true, "observation-client": true}
	seen := map[string]bool{}
	for _, arg := range args {
		if len(arg) > 4096 || strings.ContainsAny(arg, "\x00\r\n") {
			return out, errors.New("installed_selection_invalid")
		}
		if strings.HasPrefix(arg, "-") {
			name := strings.TrimPrefix(strings.TrimPrefix(strings.SplitN(arg, "=", 2)[0], "-"), "-")
			if !allowed[name] || seen[name] {
				return out, errors.New("installed_selection_invalid")
			}
			seen[name] = true
		}
	}
	f := flag.NewFlagSet("installed-pilot-check", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&out.ProductRoot, "product-root", "", "fixed installed product")
	f.StringVar(&out.UserHome, "user-home", "", "fixed participant")
	f.StringVar(&out.Workspace, "workspace", "", "fixed admitted workspace")
	f.StringVar(&out.CarrierID, "carrier-id", "", "fixed carrier")
	f.StringVar(&out.InstallerVersion, "installer-version", "", "fixed installer version")
	f.StringVar(&out.AdmissionSHA256, "experimental-seed-sha256", "", "fixed admitted role")
	f.StringVar(&out.InstalledRoot, "experimental-installed-root", "", "fixed installed root")
	f.StringVar(&out.ObservationClient, "observation-client", "", "optional Desktop-only observation; does not change selected clients")
	f.BoolVar(&out.CheckOnly, "check-only", false, "read-only diagnostics without opening clients")
	f.BoolVar(&out.Functional, "functional", false, "explicit bounded synthetic conversion and execution check")
	if err := f.Parse(args); err != nil || f.NArg() != 0 {
		return out, errors.New("installed_selection_invalid")
	}
	for _, path := range []string{out.ProductRoot, out.UserHome, out.Workspace, out.InstalledRoot} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return out, errors.New("installed_selection_invalid")
		}
	}
	if out.InstalledRoot != out.ProductRoot || !pilotWorkspaceID.MatchString(out.CarrierID) || !pilotKitID.MatchString(out.AdmissionSHA256) || !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(out.InstallerVersion) {
		return out, errors.New("installed_selection_invalid")
	}
	if seen["functional"] && !out.Functional || seen["check-only"] && !out.CheckOnly || out.Functional && out.CheckOnly {
		return out, errors.New("installed_selection_invalid")
	}
	if out.ObservationClient != "" && out.ObservationClient != "desktop" {
		return out, errors.New("installed_selection_invalid")
	}
	return out, nil
}

// This is the installed variant of the existing read-only pilot report. Only
// the separately compiled engineering dispatcher exposes this command.
func runInstalledPilotCheck(ctx context.Context, args []string, output io.Writer) error {
	r := installedPilotReport{pilotReport: pilotReport{SchemaVersion: 2, Status: "partial", Checks: []pilotCheck{}, ReferenceHost: packaging.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}}, Profile: installer.ExperimentalSeedProfile, Evidence: "engineering_only", Protocol: "maestro-experimental-installed-pilot-check-v1", CodeRegistration: "unavailable", SessionLoading: "not_observed", NativeQualification: "not_observed", FunctionalCheck: "not_requested", CheckMode: "check_only"}
	fail := func(name, reason string) error {
		next := "Preserve a instalação e compartilhe este diagnóstico; a verificação não inicializa, repara, atualiza nem abre clientes."
		if r.CheckMode == "functional" {
			r.FunctionalCheck = "incomplete"
			if r.FunctionalStage == "" {
				r.FunctionalStage, r.FunctionalReason = name, reason
			}
			next = "Preserve este relatório e o trabalho sintético informado; reconcilie a primeira falha antes de iniciar outro teste. Não há repetição automática nem reparo."
		}
		r.add(name, "failed", reason, next)
		return finishInstalledPilotReport(&r, output)
	}
	selection, err := parseInstalledPilotSelection(args)
	if err != nil {
		return fail("installed_selection", "installed_selection_invalid")
	}
	if ctx.Err() != nil {
		return fail("installed_selection", "check_cancelled")
	}
	if selection.Functional {
		r.CheckMode, r.FunctionalCheck = "functional", "incomplete"
	}
	r.ObservationClient = selection.ObservationClient
	r.add("installed_selection", "passed", "fixed_local_selection_received_not_authority", "")
	role, err := installer.LoadExperimentalSeed(selection.ProductRoot, AuthorityRegistrySHA256, selection.InstallerVersion, time.Now)
	if err != nil || role.Identity().AdmissionSHA256 != selection.AdmissionSHA256 {
		return fail("experimental_admission", "signed_installed_role_unavailable_or_changed")
	}
	i := role.Identity()
	if i.TargetOS != runtime.GOOS || i.TargetArch != runtime.GOARCH {
		return fail("experimental_admission", "role_target_mismatch")
	}
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	operation := installer.SeededOperationOptions{ExperimentalSeed: role, Self: filepath.Join(selection.ProductRoot, name), Version: selection.InstallerVersion, AuthorityRegistrySHA256: AuthorityRegistrySHA256, UserHome: selection.UserHome, ManagedRoot: selection.ProductRoot, DataRoot: filepath.Join(selection.UserHome, ".maestro-install")}
	if err := installer.VerifyExperimentalInstalledBinding(operation); err != nil {
		return fail("experimental_admission", "installed_role_or_existing_brain_binding_unavailable")
	}
	r.AdmissionSHA256 = i.AdmissionSHA256
	r.add("experimental_admission", "passed", "exact_engineering_role_and_original_binding_verified", "")
	state, err := installtx.ValidateQualifiedLaunch(operation.ManagedRoot, operation.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil {
		return fail("current_backend", "current_source_or_release_unavailable")
	}
	running, err := processidentity.Current()
	if err != nil || !installedCurrentImageMatches(running.Path, running.SHA256, Version, selection.ProductRoot, state) {
		return fail("current_backend", "executing_checker_is_not_current_installed_backend")
	}
	if err := role.VerifyReleaseIdentity(state.Binding.ManifestSHA256, state.Binding.ClientInputsSHA256); err != nil {
		return fail("current_backend", "current_release_outside_finite_rehearsal")
	}
	r.Target = packaging.Target{OS: state.TargetOS, Arch: state.TargetArch}
	r.CurrentRelease, r.CurrentManifest, r.CurrentBackend = state.Release, state.Binding.ManifestSHA256, state.Server.SHA256
	r.add("current_backend", "passed", "executing_image_and_current_source_binding_verified", "")
	options := installer.ClientOptions{SeededOperationOptions: operation, Request: installer.ClientRequest{CarrierID: selection.CarrierID, Workspace: selection.Workspace}}
	status, err := installer.ReadClientsSeeded(ctx, options)
	if err != nil || status.Record == nil || status.State != "current_materialization" && (status.State != installer.ClientPartialPreparation || !status.Record.CodePreparationPending()) {
		return fail("current_clients", "current_client_materialization_unavailable")
	}
	record := *status.Record
	if record.Binding.Workspace != selection.Workspace || !pilotWorkspaceID.MatchString(record.Binding.WorkspaceID) {
		return fail("current_clients", "current_workspace_binding_mismatch")
	}
	r.WorkspaceID = record.Binding.WorkspaceID
	r.ClientTarget = installer.ClientSelectedTarget(record)
	codePending := record.CodePreparationPending()
	if codePending {
		r.ClientPreparation = installer.ClientPreparationDesktopReady
	}
	r.CodeRegistration, r.SessionLoading = status.CodeRegistration, status.SessionLoading
	if status.Phases != nil {
		r.NativeQualification = status.Phases.NativeQualification
	}
	r.add("current_clients", "passed", "current_record_and_exact_selected_files_verified", "")
	m := record.Materialization
	if codePending {
		r.add("code_packet", "not_tested", "code_qualification_pending", "Retome explicitamente o Code pelo gerenciador; a preparação Desktop não o qualifica.")
		r.add("code_profile", "not_tested", "code_qualification_pending", "")
	} else if m.DesktopOnly() {
		r.NativeQualification = "not_applicable"
		r.add("code_packet", "not_applicable", "code_not_selected", "")
		r.add("code_profile", "not_applicable", "code_not_selected", "")
	} else if selection.ObservationClient == "desktop" {
		r.add("code_packet", "not_tested", "outside_desktop_observation", "O registro anterior foi validado; o Code não foi observado nesta tentativa.")
		r.add("code_profile", "not_tested", "outside_desktop_observation", "")
	} else {
		plugin := filepath.Join(m.CodeRoot, "plugins", "maestro-product")
		packet, err := packaging.ValidatePacket(plugin)
		if err != nil || packet.Format != "plugin" || packet.Target != r.Target || packet.ServerSHA256 != state.Server.SHA256 || packet.RegistrySHA256 != AuthorityRegistrySHA256 || packet.Stable == nil || packet.Stable.ID != record.CarrierID {
			return fail("code_packet", "current_code_packet_unavailable")
		}
		r.add("code_packet", "passed", "current_canonical_packet_verified_not_native_loading", "")
		r.add("code_profile", "passed", packet.NativeMode, "O perfil descreve os arquivos verificados; não comprova carregamento ou execução nesta sessão.")
	}
	carrier := packaging.StableCarrier{Profile: record.Profile, Protocol: record.Protocol, ID: record.CarrierID, Version: record.CarrierVersion}
	if err := packaging.InspectReusedStableExtension(m.DesktopPath, carrier, m.DesktopSHA256, m.BootstrapServerSHA256, m.BootstrapEmbeddedInputsSHA256, m.DesktopNativeContractSHA256); err != nil {
		return fail("desktop_archive", "recorded_desktop_archive_changed")
	}
	if err := setup.VerifyRecordedDesktopContract(m.DesktopPath, options, state.TargetOS, m.DesktopNativeContractSHA256); err != nil {
		return fail("desktop_archive", "recorded_desktop_native_contract_unavailable")
	}
	// Historical inert resources were admitted with the recorded archive. Its
	// exact unchanged bytes and native launch are checked above; rebuilding those
	// old resources using this binary's newer canonical assets would be incorrect.
	if m.BootstrapServerSHA256 == state.Server.SHA256 && m.BootstrapEmbeddedInputsSHA256 == state.Server.EmbeddedInputsSHA256 {
		archive, err := packaging.ValidateArchive(m.DesktopPath)
		if err != nil || installedDesktopMatches(record, archive, r.Target) != nil {
			return fail("desktop_archive", "current_desktop_canonical_assets_unavailable")
		}
	}
	r.DesktopExtension, err = installedDesktopExtensionName(record)
	if err != nil {
		return fail("desktop_archive", "recorded_desktop_connector_identity_unavailable")
	}
	r.add("desktop_archive", "passed", "recorded_contract_verified_historical_bootstrap_separate_from_current_backend", "Instalação e carregamento no Desktop permanecem verificações do cliente real.")
	if m.DesktopMethods == nil {
		return fail("desktop_methods", "current_method_bootstrap_unavailable")
	}
	frontend, err := packaging.VerifyDesktopLiveMethodFrontend(m.DesktopMethods.Archive, m.DesktopMethods.Version)
	if err != nil || frontend.FileCount != m.DesktopMethods.Files || frontend.ArchiveSHA256 != m.DesktopMethods.SHA256 || frontend.BootstrapProtocol != m.DesktopMethods.BootstrapProtocol {
		return fail("desktop_methods", "current_method_bootstrap_changed")
	}
	r.add("desktop_methods", "passed", "current_bootstrap_archive_verified_host_installation_unobserved", "")
	if codePending {
		r.add("code_cli", "not_tested", "code_qualification_pending", "Nenhuma conferência do executável ou login Code foi iniciada.")
	} else if m.DesktopOnly() {
		r.add("code_cli", "not_applicable", "code_not_selected", "")
	} else if selection.ObservationClient == "desktop" {
		r.add("code_cli", "not_tested", "outside_desktop_observation", "Nenhuma conferência do executável ou login Code foi iniciada.")
	} else if record.Code.Executable == "" || !filepath.IsAbs(record.Code.Executable) {
		r.add("code_cli", "not_tested", "no_absolute_executable_selected", "Selecione o Claude Code uma vez pelo gerenciador admitido.")
	} else if record.Code.ExecutableSHA256 == "" {
		r.add("code_cli", "not_tested", "absolute_executable_selected_identity_not_yet_observed", "Conclua o registro normal pelo gerenciador; este diagnóstico não executa nem qualifica o Code.")
	} else {
		if !installedCodeExecutableMatches(record.Code) {
			return fail("code_cli", "selected_code_executable_changed")
		}
		r.add("code_cli", "passed", "selected_absolute_executable_verified_login_not_tested", "")
	}
	o := product.Options{ExperimentalSeed: role, ExperimentalSeedRoot: selection.ProductRoot, ProductRoot: selection.ProductRoot, UserHome: selection.UserHome, WorkspacePath: selection.Workspace, AuthorityRegistryPath: filepath.Join(selection.ProductRoot, "trust", "release-authority-registry.json"), AuthorityRegistrySHA256: AuthorityRegistrySHA256, CLIVersion: Version, InstalledManifestSHA256: state.Binding.ManifestSHA256, InstalledClientInputsSHA256: state.Binding.ClientInputsSHA256, ManagementCarrierID: selection.CarrierID, ManagementInstallerVersion: selection.InstallerVersion}
	o.OwnerReviewChannel = record.Materialization.OwnerReviewChannel
	expectedRuntime, err := installedExpectedRuntime(role, state)
	if err != nil {
		return fail("document_runtime", "closed_current_runtime_identity_unavailable")
	}
	if err := installedPilotTransport(ctx, o, record.Binding.WorkspaceID, state.Binding.ManifestSHA256, expectedRuntime, &r, selection.Functional); err != nil {
		r.PrivateDiagnostic = err.Error()
		var readback *installedPilotReadbackError
		if errors.As(err, &readback) {
			r.PrivateDiagnostic = readback.cause.Error()
			return fail("mcp_readback", readback.Error())
		}
		return fail("mcp_readback", "current_installed_sdk_readback_failed")
	}
	for _, name := range []string{"code_client", "desktop_client", "onboarding", "skills", "hooks", "authored_brain", "memory", "conversion"} {
		if selection.Functional && name == "conversion" {
			continue
		}
		if (codePending || selection.ObservationClient == "desktop") && (name == "code_client" || name == "hooks") {
			reason := "outside_desktop_observation"
			if codePending {
				reason = "code_qualification_pending"
			}
			r.add(name, "not_tested", reason, "A seleção original foi preservada; este diagnóstico não testa nem qualifica o Code.")
			continue
		}
		if m.DesktopOnly() && (name == "code_client" || name == "hooks") {
			r.add(name, "not_applicable", "code_not_selected", "")
			continue
		}
		r.add(name, "not_tested", "native_journey_not_observed", "Use a jornada existente nos clientes reais e registre os resultados; o diagnóstico não executa esta funcionalidade.")
	}
	return finishInstalledPilotReport(&r, output)
}

func installedCurrentImageMatches(path, sha, version, root string, state installtx.State) bool {
	name := "maestro-mcp"
	if state.TargetOS == "windows" {
		name += ".exe"
	}
	return filepath.Clean(path) == filepath.Join(root, "bin", name) && pilotKitID.MatchString(sha) && sha == state.Server.SHA256 && version != "" && version == state.Server.Version && state.TargetOS == runtime.GOOS && state.TargetArch == runtime.GOARCH
}

func installedDesktopMatches(record installer.ClientRecord, archive packaging.PacketStatus, target packaging.Target) error {
	m := record.Materialization
	if archive.Format != "mcpb" || archive.Target != target || archive.Stable == nil || archive.Stable.Profile != record.Profile || archive.Stable.Protocol != record.Protocol || archive.Stable.ID != record.CarrierID || archive.Stable.Version != record.CarrierVersion || archive.RegistrySHA256 != record.Binding.RegistrySHA256 || archive.ServerSHA256 != m.BootstrapServerSHA256 || archive.EmbeddedInputsSHA256 != m.BootstrapEmbeddedInputsSHA256 || archive.NativeContractSHA256 != m.DesktopNativeContractSHA256 {
		return errors.New("installed_desktop_contract_mismatch")
	}
	return nil
}

// Called only after current record/archive/native-contract admission. Formatting
// this identity grants no connector access or native loading evidence.
func installedDesktopExtensionName(record installer.ClientRecord) (string, error) {
	if !pilotWorkspaceID.MatchString(record.CarrierID) || record.Profile != installer.StableClientProfile || record.Protocol != installer.StableClientProtocol || record.CarrierVersion != installer.StableCarrierVersion || !pilotKitID.MatchString(record.Materialization.DesktopNativeContractSHA256) {
		return "", errors.New("admitted stable Desktop contract required")
	}
	return "maestro-connector-" + record.CarrierID, nil
}

type installedPilotReadbackError struct {
	stage string
	cause error
}

func (e *installedPilotReadbackError) Error() string {
	reason := "failed"
	if errors.Is(e.cause, context.DeadlineExceeded) {
		reason = "deadline_exceeded"
	} else if errors.Is(e.cause, context.Canceled) {
		reason = "cancelled"
	}
	if e.stage == "checker_connect" && reason == "failed" {
		return "current_installed_checker_transport_unavailable_or_unsupported"
	}
	return "current_installed_sdk_" + e.stage + "_" + reason
}

func (e *installedPilotReadbackError) Unwrap() error { return e.cause }

func installedPilotCall(ctx context.Context, session *mcp.ClientSession, stage, name string, args map[string]any, out any) error {
	// Each signed read keeps its finite bound without consuming the next read's budget.
	budget := 20 * time.Second
	if stage == "content_status" || stage == "content_convert" {
		// Several fifteen-second identity probes and authenticated inventories need
		// a separate finite request policy; synchronous teardown remains separate.
		budget = 3 * time.Minute
	}
	callCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	response, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err == nil {
		if response == nil || len(response.Content) != 1 {
			err = errors.New("installed_tool_result_invalid")
		} else if body, ok := response.Content[0].(*mcp.TextContent); !ok || len(body.Text) > 256<<10 {
			err = errors.New("installed_tool_result_bound")
		} else if response.IsError {
			// Retain the closed refusal code, never a prompt, document or arbitrary
			// provider error body, in the private first-stage diagnostic.
			var refusal struct {
				Code string `json:"code"`
			}
			if json.Unmarshal([]byte(body.Text), &refusal) == nil && regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`).MatchString(refusal.Code) {
				err = fmt.Errorf("installed_tool_refused:%s", refusal.Code)
			} else {
				err = errors.New("installed_tool_refused")
			}
		} else {
			err = json.Unmarshal([]byte(body.Text), out)
		}
	}
	if callCtx.Err() != nil {
		err = callCtx.Err()
	}
	if err != nil {
		return &installedPilotReadbackError{stage: stage, cause: err}
	}
	return nil
}

func installedExpectedRuntime(role *installer.VerifiedExperimentalSeed, state installtx.State) (string, error) {
	if role == nil || role.Revalidate() != nil {
		return "", errors.New("closed_current_runtime_identity_unavailable")
	}
	body, _ := role.RecordBytes()
	var record installer.ExperimentalSeedRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return "", err
	}
	for _, release := range record.Releases {
		if release.Release == state.Release && release.ManifestSHA256 == state.Binding.ManifestSHA256 && release.ClientInputsSHA256 == state.Binding.ClientInputsSHA256 && pilotKitID.MatchString(release.RuntimeSHA256) {
			return release.RuntimeSHA256, nil
		}
	}
	return "", errors.New("closed_current_runtime_identity_unavailable")
}

func finishInstalledPilotReport(r *installedPilotReport, output io.Writer) error {
	var ignored bytes.Buffer
	err := finishPilotReport(&r.pilotReport, &ignored)
	prefix := fmt.Sprintf("Perfil: `%s`; evidência de distribuição: `%s`. Este diagnóstico verifica a instalação admitida e conexão SDK pelo processo autenticado do carrier de autoteste instalado. Não abre clientes, executa manutenção agendada nem comprova carregamento, qualificação nativa ou atualização N→N+1.\n\n", r.Profile, r.Evidence)
	if r.CheckMode == "functional" {
		prefix += fmt.Sprintf("Autoteste sintético MCP: **%s**. Etapa: `%s`; motivo: `%s`. Não comprova uma sessão nativa de cliente, hooks, agentes, memória ou agendamento.\n\n", r.FunctionalCheck, r.FunctionalStage, r.FunctionalReason)
		if r.FunctionalItemID != "" {
			prefix += fmt.Sprintf("Trabalho sintético deste teste: `%s`. Preserve esta identidade em caso de interrupção; nenhum outro trabalho foi retomado.\n\n", r.FunctionalItemID)
		}
		r.Markdown = strings.ReplaceAll(r.Markdown, "Este diagnóstico faz apenas leituras locais.", "Esta ação explícita testa conversão e continuidade com dados sintéticos pelo MCP instalado; preserve resultados incompletos.")
	}
	r.Markdown = prefix + strings.ReplaceAll(r.Markdown, "Identificador do kit: não verificado.", "Este é o perfil instalado; a identidade da admissão e da versão atual consta no diagnóstico estruturado.")
	if encodeErr := json.NewEncoder(output).Encode(r); encodeErr != nil {
		return encodeErr
	}
	return err
}

func installedCodeExecutableMatches(code installer.CodeObservation) bool {
	hash, _, err := packaging.InspectNativeExecutable(code.Executable)
	return err == nil && hash == code.ExecutableSHA256
}
