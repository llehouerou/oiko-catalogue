package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// ManifestFile is the manifest of a type of Bridge, at its module's root.
const ManifestFile = "oiko-bridge.json"

// Type is a type of Bridge as its manifest describes it.
type Type struct {
	Description string `json:"description"`
	// Config is an example of the body of the type's section of bridges.
	Config json.RawMessage `json:"config"`
}

// ParseManifest reads a manifest, {"types": {"<type>": Type}}, refusing
// unknown fields, and checks it: see checkTypes.
func ParseManifest(data []byte) (map[string]Type, error) {
	var m struct {
		Types map[string]Type `json:"types"`
	}
	if err := decodeStrict(data, &m); err != nil {
		return nil, err
	}
	if err := checkTypes(m.Types); err != nil {
		return nil, err
	}
	return m.Types, nil
}

// decodeStrict decodes the single JSON value data holds into v, refusing
// unknown fields.
func decodeStrict(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("data after the JSON value")
	}
	return nil
}

// checkTypes checks a module's types: at least one, each named as Oiko names
// a Bridge (the catalogue names its example section after the type), with a
// description, and a config that is a JSON object whose "type", if any, is
// the type's name.
func checkTypes(types map[string]Type) error {
	if len(types) == 0 {
		return errors.New("no types")
	}
	for _, name := range slices.Sorted(maps.Keys(types)) {
		t := types[name]
		if name == "" || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			return fmt.Errorf("type %q: a name is made of lowercase letters, digits and dashes", name)
		}
		if strings.TrimSpace(t.Description) == "" {
			return fmt.Errorf("type %s: no description", name)
		}
		var config map[string]json.RawMessage
		if err := json.Unmarshal(t.Config, &config); err != nil || config == nil {
			return fmt.Errorf("type %s: config is not a JSON object", name)
		}
		if raw, ok := config["type"]; ok {
			var typ string
			if err := json.Unmarshal(raw, &typ); err != nil || typ != name {
				return fmt.Errorf("type %s: config's \"type\" is not %q", name, name)
			}
		}
	}
	return nil
}
