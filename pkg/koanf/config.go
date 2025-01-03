package koanf

import (
	"fmt"
	"log"
	"strings"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// New reads configuration with koanf.
// service name is used for reading environment variables
// and def contains the default values.
func Provide[T interface{}](service string, def T) T {
	k := koanf.New(".")

	// prefix indicates environment variables prefix.
	prefix := fmt.Sprintf("%s_", strings.ToUpper(service))

	// create a new instance based-on given time.
	var instance T

	// load default configuration from default function
	if err := k.Load(structs.Provider(def, "koanf"), nil); err != nil {
		log.Fatalf("error loading default: %s", err)
	}

	// load configuration from file
	if err := k.Load(file.Provider("config.toml"), toml.Parser()); err != nil {
		log.Printf("error loading config.toml: %s", err)
	}

	// load environment variables
	if err := k.Load(
		// replace __ with . in environment variables so you can reference field a in struct b
		// as a__b.
		env.Provider(prefix, ".", func(source string) string {
			base := strings.ToLower(strings.TrimPrefix(source, prefix))

			return strings.ReplaceAll(base, "__", ".")
		}),
		nil,
	); err != nil {
		log.Printf("error loading environment variables: %s", err)
	}

	if err := k.Unmarshal("", &instance); err != nil {
		log.Fatalf("error un-marshalling config: %s", err)
	}

	return instance
}
