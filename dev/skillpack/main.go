// skillpack prepares native method discovery trees in a fresh release stage.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"

	base "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	tech "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func run(args []string) error {
	flags := flag.NewFlagSet("skillpack", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	destination := flags.String("root", "", "fresh managed release stage")
	if err := flags.Parse(args); err != nil || *destination == "" || flags.NArg() != 0 {
		return errors.New("usage: skillpack --root RELEASE_STAGE")
	}
	root, err := os.OpenRoot(*destination)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, surface := range []string{".claude", ".agents"} {
		if info, err := root.Lstat(surface); err == nil && !info.IsDir() {
			return errors.New("native discovery parent must be an ordinary directory")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	catalog := skillsindex.Catalog{SchemaVersion: 1}
	files := map[string][]byte{}
	for _, source := range []struct {
		catalog func() (skillsindex.Catalog, error)
		files   func(string) (map[string][]byte, error)
	}{{base.Catalog, base.Files}, {tech.Catalog, tech.Files}} {
		entries, err := source.catalog()
		if err != nil {
			return err
		}
		catalog.Skills = append(catalog.Skills, entries.Skills...)
		for _, skill := range entries.Skills {
			resources, err := source.files(skill.ID)
			if err != nil {
				return err
			}
			for name, body := range resources {
				if !fs.ValidPath(name) || !fs.ValidPath(skill.ID) {
					return errors.New("invalid embedded method resource")
				}
				files[path.Join(skill.ID, name)] = body
			}
		}
	}
	sort.Slice(catalog.Skills, func(i, j int) bool { return catalog.Skills[i].ID < catalog.Skills[j].ID })
	files["catalog.json"], err = skillsindex.RenderJSON(catalog)
	if err != nil {
		return err
	}
	files["INDEX.md"], err = skillsindex.RenderMarkdown(catalog)
	if err != nil {
		return err
	}
	files["pocock-upstream.json"] = tech.Provenance()
	for _, surface := range []string{".claude/skills", ".agents/skills"} {
		for name := range files {
			file := path.Join(surface, name)
			if _, err := root.Lstat(file); !os.IsNotExist(err) {
				return errors.New("native method target already exists; existing files preserved")
			}
			for parent := path.Dir(file); parent != "."; parent = path.Dir(parent) {
				info, err := root.Lstat(parent)
				if err == nil && !info.IsDir() {
					return errors.New("native method ancestor must be an ordinary directory")
				}
				if err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
	}
	for _, surface := range []string{".claude/skills", ".agents/skills"} {
		for name, body := range files {
			file := path.Join(surface, name)
			if err := root.MkdirAll(path.Dir(file), 0755); err != nil {
				return err
			}
			output, err := root.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				return err
			}
			_, writeErr := output.Write(body)
			closeErr := output.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
