// Command oiko-catalogue indexes the types of Bridge of the catalogue into
// index.json (ADR 0020 of Oiko), in three steps, each run by the workflow:
//
//	oiko-catalogue build -work DIR [-force] [-index index.json]
//	oiko-catalogue check -work DIR [-index index.json]
//	oiko-catalogue validate [-index index.json]
//
// build finds, resolves and builds the types, reading Oiko with credentials;
// check, once they are gone, runs what build built and writes the index;
// validate checks an index's shape, in the job that commits it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("oiko-catalogue: ")
	if len(os.Args) < 2 {
		log.Fatal("usage: oiko-catalogue build|check|validate [flags]")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	index := flags.String("index", "index.json", "the index")
	work := flags.String("work", "", "the directory build leaves to check")
	force := flags.Bool("force", false, "build every module, even against an Oiko it was built against")
	flags.Parse(os.Args[2:])
	if err := run(os.Args[1], *index, *work, *force); err != nil {
		log.Fatal(err)
	}
}

func run(step, index, work string, force bool) error {
	ctx := context.Background()
	if step != "validate" {
		if work == "" {
			return errors.New("-work is needed")
		}
		var err error
		if work, err = filepath.Abs(work); err != nil {
			return err
		}
	}
	switch step {
	case "build":
		prev, err := readIndex(index)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(work, 0o755); err != nil {
			return err
		}
		return Build(ctx, prev, work, force)
	case "check":
		ix, err := Check(ctx, work)
		if err != nil {
			return err
		}
		return os.WriteFile(index, ix.Encode(), 0o644)
	case "validate":
		data, err := os.ReadFile(index)
		if err != nil {
			return err
		}
		if _, err := ParseIndex(data); err != nil {
			return fmt.Errorf("%s: %w", index, err)
		}
		return nil
	}
	return fmt.Errorf("no step %q", step)
}

// readIndex reads the index at path, empty when there is none yet.
func readIndex(path string) (Index, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Index{}, nil
	}
	if err != nil {
		return Index{}, err
	}
	ix, err := ParseIndex(data)
	if err != nil {
		return Index{}, fmt.Errorf("%s: %w", path, err)
	}
	return ix, nil
}
