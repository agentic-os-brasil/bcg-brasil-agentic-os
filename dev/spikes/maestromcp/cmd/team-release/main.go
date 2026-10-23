// team-release publishes signed, synthetic pilot artifacts to a local feed.
// It never obtains SharePoint credentials or synchronizes files itself.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/distribution"
)

func run(args []string, out, diagnostic io.Writer) int {
	return runWith(args, out, diagnostic, distribution.Publish)
}

func runWith(args []string, out, diagnostic io.Writer, publish func(distribution.ReleaseInput) (distribution.Manifest, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(diagnostic, "Uso: team-release keygen --private-key <novo arquivo> --public-key <novo arquivo>; publish --key <arquivo privado> --feed <pasta> --version <versão> --sequence <n> --target <plataforma> --server <binário> --installer <binário>")
		return 2
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	switch args[0] {
	case "keygen":
		private := flags.String("private-key", "", "novo arquivo privado; nunca publicar na pasta compartilhada")
		public := flags.String("public-key", "", "novo arquivo público para bootstrap separado")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		if flags.NArg() != 0 || *private == "" || *public == "" {
			fmt.Fprintln(diagnostic, "Informe dois caminhos novos em Developer para --private-key e --public-key.")
			return 2
		}
		if err := developerPaths(*private, *public); err != nil {
			fmt.Fprintln(diagnostic, err)
			return 2
		}
		if err := generateKeys(*private, *public, out); err != nil {
			fmt.Fprintln(diagnostic, "Chaves não geradas:", err)
			return 1
		}
		return 0
	case "publish":
		keyFile := flags.String("key", "", "arquivo privado Ed25519 hexadecimal fora da pasta compartilhada")
		feed := flags.String("feed", "", "pasta de publicação desta plataforma")
		version := flags.String("version", "", "versão experimental")
		sequence := flags.Uint64("sequence", 0, "sequência monotônica da publicação")
		target := flags.String("target", "", "darwin-arm64, darwin-amd64 ou windows-amd64")
		server := flags.String("server", "", "executável nativo do servidor")
		installer := flags.String("installer", "", "executável nativo do instalador com chave pública embutida")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		if flags.NArg() != 0 || *keyFile == "" || *feed == "" || *version == "" || *sequence == 0 || *target == "" || *server == "" || *installer == "" {
			fmt.Fprintln(diagnostic, "Todos os campos de publish são obrigatórios; sequence deve ser maior que zero.")
			return 2
		}
		if err := developerPaths(*keyFile); err != nil {
			fmt.Fprintln(diagnostic, err)
			return 2
		}
		if err := keyOutsideFeed(*keyFile, *feed); err != nil {
			fmt.Fprintln(diagnostic, err)
			return 2
		}
		info, err := os.Lstat(*keyFile)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 512 {
			fmt.Fprintln(diagnostic, "A chave privada deve ser arquivo regular pequeno, sem symlink.")
			return 2
		}
		encoded, err := os.ReadFile(*keyFile)
		if err != nil {
			fmt.Fprintln(diagnostic, "Não foi possível ler a chave privada.")
			return 2
		}
		key, err := decodePrivateKey(encoded)
		if err != nil {
			fmt.Fprintln(diagnostic, "Chave privada inválida:", err)
			return 2
		}
		manifest, err := publish(distribution.ReleaseInput{FeedRoot: *feed, Version: *version, Sequence: *sequence, Target: *target, Server: *server, Installer: *installer, PrivateKey: key})
		if err != nil {
			fmt.Fprintln(diagnostic, "Publicação não concluída:", err)
			return 1
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(manifest); err != nil {
			fmt.Fprintln(diagnostic, "Falha ao emitir manifesto:", err)
			return 1
		}
		fmt.Fprintln(diagnostic, "Publicação local concluída. Isso não atesta sincronização do SharePoint, instalação dos clientes ou autorização de produção.")
		return 0
	default:
		fmt.Fprintln(diagnostic, "Subcomando desconhecido; use keygen ou publish.")
		return 2
	}
}

func decodePrivateKey(encoded []byte) (ed25519.PrivateKey, error) {
	key, err := hex.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("esperados 64 bytes Ed25519 em hexadecimal")
	}
	expected := ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])
	if !bytes.Equal(expected, key) {
		return nil, errors.New("metade pública não corresponde à seed privada")
	}
	return ed25519.PrivateKey(key), nil
}

func generateKeys(privatePath, publicPath string, out io.Writer) error {
	if filepath.Clean(privatePath) == filepath.Clean(publicPath) {
		return errors.New("arquivos público e privado devem ser diferentes")
	}
	for _, path := range []string{privatePath, publicPath} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return errors.New("os dois arquivos devem ser novos; nada será sobrescrito")
		}
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	for _, item := range []struct {
		path  string
		value []byte
	}{{privatePath, private}, {publicPath, public}} {
		file, err := os.OpenFile(item.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := fmt.Fprintln(file, hex.EncodeToString(item.value))
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "Arquivos criados. Chave privada permanece local; distribua somente a chave pública pelo canal autorizado.\nChave pública: %s\n", hex.EncodeToString(public))
	return err
}

func developerPaths(paths ...string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	developer, err := filepath.EvalSymlinks(filepath.Join(home, "Developer"))
	if err != nil {
		return errors.New("a pasta Developer local deve existir")
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return errors.New("use caminhos absolutos em Developer para as chaves")
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil {
			return errors.New("a pasta das chaves deve existir")
		}
		rel, err := filepath.Rel(developer, parent)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("as chaves devem ficar fisicamente em Developer")
		}
	}
	return nil
}

func keyOutsideFeed(keyFile, feed string) error {
	resolvedFeed, err := filepath.EvalSymlinks(feed)
	if err != nil {
		return errors.New("a pasta de publicação deve existir antes de publicar")
	}
	resolvedKeyParent, err := filepath.EvalSymlinks(filepath.Dir(keyFile))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(resolvedFeed, filepath.Join(resolvedKeyParent, filepath.Base(keyFile)))
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("a chave privada não pode ficar dentro da pasta de publicação")
	}
	return nil
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
