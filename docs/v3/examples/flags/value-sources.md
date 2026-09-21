---
tags:
  - v3
search:
  boost: 2
---

Flags can have their default values set from different sources. The following sources are
provided by default with `urfave/cli`

 - Environment
 - Text Files

The library also provides a framework for users to plugin their own implementation of value sources
to be fetched via other mechanisms(http and so on). 

In addition there is a `urfave/cli-altsrc` repo which hosts some common value sources to read 
from files or via http/https. 

 - YAML
 - JSON
 - TOML

#### Values from the Environment

To set a value from the environment use `cli.EnvVars`.  e.g.

<!-- {
  "args": ["&#45;&#45;help"],
  "output": "language for the greeting.*APP_LANG"
} -->
```go
package main

import (
	"log"
	"os"
	"context"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "lang",
				Aliases: []string{"l"},
				Value:   "english",
				Usage:   "language for the greeting",
				Sources: cli.EnvVars("APP_LANG"),
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
```

If `cli.EnvVars` contains more than one string, the first environment variable that
resolves is used.

<!-- {
  "args": ["&#45;&#45;help"],
  "output": "language for the greeting.*LEGACY_COMPAT_LANG.*APP_LANG.*LANG"
} -->
```go
package main

import (
	"log"
	"os"
	"context"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "lang",
				Aliases: []string{"l"},
				Value:   "english",
				Usage:   "language for the greeting",
				Sources: cli.EnvVars("LEGACY_COMPAT_LANG", "APP_LANG", "LANG"),
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
```

#### Values from files

You can also have the default value set from file via `cli.File`.  e.g.

<!-- {
  "args": ["&#45;&#45;help"],
  "output": "password for the mysql database"
} -->
```go
package main

import (
	"log"
	"os"
	"context"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "password",
				Aliases:  []string{"p"},
				Usage:    "password for the mysql database",
				Sources: cli.Files("/etc/mysql/password"),
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
```

Note that default values are set in the same order as they are defined in the
`Sources` param. This allows the user to choose order of priority

#### Override precedence and source tracking

Regardless of how sources are combined, the merge precedence for any flag is
locked to:

1. command line flags (highest)
2. environment variable sources
3. file / structured config sources
4. the code `Value` default (lowest)

The first source in a `Sources` chain must not be a lower-precedence layer
than a source that follows it. A chain such as
`NewValueSourceChain(File("cfg"), EnvVar("APP_X"))` puts a file ahead of an
environment variable, which contradicts the locked precedence. Such a chain
fails at flag install time with an error naming both offending layers,
instead of producing an undefined merge. Chains that already follow
environment-then-file order continue to merge exactly as before.

After a command has run you can ask where a flag's final value came from via
`Command.ValueSource`:

```go
origin, ok := cmd.ValueSource("name")
if !ok {
	// the flag does not exist or its tracked origin does not match the
	// actual merge result
}
fmt.Println(origin.Layer)  // "command-line", "environment", "file", or "default"
fmt.Println(origin)        // human readable, e.g. environment variable "APP_NAME"
```

`ValueSource` cross-checks the tracked origin against the real merge result:
if the reported layer cannot be reconciled with how the value was actually
resolved, it returns `ok == false` rather than reporting a fabricated source.

#### Values from alternate input sources (YAML, TOML, and others)

There is a separate package [altsrc](https://github.com/urfave/cli-altsrc) that adds support for getting flag values
from other file input sources.

Currently supported input source formats by that library are:

- YAML
- JSON
- TOML

A simple straight forward usage would be

```go
package main

import (
	"log"
	"os"
	"context"

	"github.com/urfave/cli/v3"
	"github.com/urfave/cli-altsrc/v3"
	yaml "github.com/urfave/cli-altsrc/v3/yaml"
)

func main() {
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "password",
				Aliases: []string{"p"},
				Usage:   "password for the mysql database",
				Sources: cli.NewValueSourceChain(yaml.YAML("somekey", altsrc.StringSourcer("/path/to/filename"))),
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
```

Sometime the source name is itself provided by another CLI flag. To allow the library to "lazy-load"
the file when needed we use the `altsrc.NewStringPtrSourcer` function to bind the value of the flag 
to a pointer that is set as a destination of another flag

```go
package main

import (
	"log"
	"os"
	"context"

	"github.com/urfave/cli/v3"
	"github.com/urfave/cli-altsrc/v3"
	yaml "github.com/urfave/cli-altsrc/v3/yaml"
)

func main() {
	var filename string
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "file",
				Aliases:     []string{"f"},
				Value:       "/path/to/default",
				Usage:       "filename for mysql database",
				Destination: &filename,
			},
			&cli.StringFlag{
				Name:    "password",
				Aliases: []string{"p"},
				Usage:   "password for the mysql database",
				Sources: cli.NewValueSourceChain(yaml.YAML("somekey", altsrc.NewStringPtrSourcer(&filename))),
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
```
