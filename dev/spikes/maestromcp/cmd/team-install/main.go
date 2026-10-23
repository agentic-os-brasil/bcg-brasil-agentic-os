// team-install stages a verified synthetic pilot release and optionally asks
// supported host CLIs to activate it. It never writes Claude profile internals.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/distribution"
)

// TrustedKeyHex is pinned by the pilot publisher at build time using -ldflags
// '-X main.TrustedKeyHex=<public-key-hex>'. No key is learned from the feed.
var TrustedKeyHex string

type dependencies struct {
	home, target, platform string
	apply                  func(distribution.InstallInput) (distribution.Receipt, error)
	inspect                func(string) (distribution.Receipt, error)
	execute                func(string, ...string) error
}

type report struct {
	distribution.Receipt
	CodeStatus        string `json:"code_status"`
	DesktopStatus     string `json:"desktop_status"`
	HostRuntimeStatus string `json:"host_runtime_status"`
}

func run(args []string, out, diagnostic io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(diagnostic, "Não foi possível localizar a pasta do usuário:", err)
		return 2
	}
	d := dependencies{
		home:   filepath.Join(home, "Developer", "Maestro Team Pilot"),
		target: runtime.GOOS + "-" + runtime.GOARCH, platform: runtime.GOOS,
		apply: distribution.Apply, inspect: distribution.Inspect,
		execute: func(name string, args ...string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, name, args...)
			command.Stdout, command.Stderr = diagnostic, diagnostic
			return command.Run()
		},
	}
	return runWith(args, out, diagnostic, d, TrustedKeyHex)
}

func runWith(args []string, out, diagnostic io.Writer, d dependencies, trustedKey string) int {
	if len(args) == 0 || (args[0] != "install" && args[0] != "update" && args[0] != "status") {
		fmt.Fprintln(diagnostic, "Uso: maestro-team-pilot install|update --feed <pasta sincronizada> [--home <pasta>] [--activate-code] [--open-desktop]; status [--home <pasta>]")
		return 2
	}
	operation := args[0]
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	home := flags.String("home", d.home, "pasta local do piloto, separada do SharePoint")
	var feed *string
	var activateCode, openDesktop *bool
	if operation != "status" {
		feed = flags.String("feed", "", "pasta local sincronizada do canal desta plataforma")
		activateCode = flags.Bool("activate-code", false, "registrar marketplace e instalar plugin pelo CLI oficial")
		openDesktop = flags.Bool("open-desktop", false, "abrir extensão; a confirmação no Desktop continua necessária")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostic, "Argumentos posicionais inesperados.")
		return 2
	}
	if strings.TrimSpace(*home) == "" {
		fmt.Fprintln(diagnostic, "--home não pode ser vazio.")
		return 2
	}
	if operation == "status" {
		receipt, err := d.inspect(*home)
		if err != nil {
			fmt.Fprintln(diagnostic, "Não foi possível ler a instalação:", err)
			return 1
		}
		return emit(out, diagnostic, report{Receipt: receipt, CodeStatus: "NOT_CHECKED", DesktopStatus: "NOT_CHECKED"}, 0)
	}
	if strings.TrimSpace(*feed) == "" {
		fmt.Fprintln(diagnostic, "Informe --feed com a pasta local sincronizada da sua plataforma.")
		return 2
	}
	publicKey, err := hex.DecodeString(trustedKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		fmt.Fprintln(diagnostic, "Este instalador não contém uma chave pública válida. Obtenha o instalador de bootstrap pelo canal autorizado; nenhuma chave será aceita da pasta compartilhada.")
		return 2
	}
	receipt, err := d.apply(distribution.InstallInput{FeedRoot: *feed, Home: *home, Target: d.target, PublicKey: ed25519.PublicKey(publicKey)})
	if err != nil {
		fmt.Fprintln(diagnostic, "Publicação não aplicada:", err)
		return 1
	}
	result := report{Receipt: receipt, CodeStatus: "NOT_REQUESTED", DesktopStatus: "NOT_REQUESTED"}
	exitCode := 0
	if *activateCode {
		if operation == "update" {
			if err := d.execute("claude", "plugin", "update", "maestro-team-pilot@maestro-team-pilot-releases"); err != nil {
				fmt.Fprintln(diagnostic, "Falha ao atualizar plugin no Code:", err)
				result.CodeStatus, exitCode = "CODE_ACTIVATION_FAILED", 1
			} else {
				result.CodeStatus = "CODE_INSTALL_COMMANDS_SUCCEEDED"
			}
		} else if err := d.execute("claude", "plugin", "marketplace", "add", receipt.Marketplace); err != nil {
			fmt.Fprintln(diagnostic, "Falha ao registrar marketplace no Code:", err)
			result.CodeStatus, exitCode = "CODE_ACTIVATION_FAILED", 1
		} else if err := d.execute("claude", "plugin", "install", "maestro-team-pilot@maestro-team-pilot-releases"); err != nil {
			fmt.Fprintln(diagnostic, "Falha ao instalar plugin no Code:", err)
			result.CodeStatus, exitCode = "CODE_ACTIVATION_FAILED", 1
		} else {
			result.CodeStatus = "CODE_INSTALL_COMMANDS_SUCCEEDED"
		}
		if result.CodeStatus == "CODE_INSTALL_COMMANDS_SUCCEEDED" {
			fmt.Fprintln(diagnostic, "Code: comandos de instalação concluídos. Abra uma nova sessão ou faça reload; chamadas reais do MCP ainda precisam ser verificadas.")
		}
	}
	if *openDesktop {
		var openErr error
		switch d.platform {
		case "darwin":
			openErr = d.execute("open", "-a", "Claude", receipt.Extension)
		case "windows":
			openErr = d.execute("explorer.exe", receipt.Extension)
		default:
			result.DesktopStatus = "DESKTOP_MANUAL_INSTALL_REQUIRED"
			fmt.Fprintln(diagnostic, "Abra manualmente o pacote no Desktop:", receipt.Extension)
		}
		if result.DesktopStatus != "DESKTOP_MANUAL_INSTALL_REQUIRED" {
			if openErr != nil {
				fmt.Fprintln(diagnostic, "Não foi possível abrir o pacote; instale-o manualmente:", receipt.Extension, openErr)
				result.DesktopStatus, exitCode = "DESKTOP_OPEN_FAILED", 1
			} else {
				result.DesktopStatus = "DESKTOP_CONFIRMATION_REQUIRED"
				fmt.Fprintln(diagnostic, "Desktop: confirme a instalação/atualização na interface e verifique o MCP. Abrir o pacote não atesta instalação.")
			}
		}
	}
	return emit(out, diagnostic, result, exitCode)
}

func emit(out, diagnostic io.Writer, value report, exitCode int) int {
	value.HostRuntimeStatus = "HOST_RUNTIME_UNATTESTED"
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(diagnostic, "Falha ao emitir resultado:", err)
		return 1
	}
	return exitCode
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
