package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeFileSource creates a temp file containing data and returns its path.
func writeFileSource(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.txt")
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
	return path
}

func TestValueOriginPrecedenceString(t *testing.T) {
	path := writeFileSource(t, "from-file")
	t.Setenv("APP_NAME", "from-env")

	tests := []struct {
		name       string
		args       []string
		wantValue  string
		wantLayer  ValueSourceLayer
		wantSource string
	}{
		{
			name:       "command line overrides env and file",
			args:       []string{"--name", "from-cli"},
			wantValue:  "from-cli",
			wantLayer:  LayerCommandLine,
			wantSource: "command line flag",
		},
		{
			name:       "env overrides file",
			args:       nil,
			wantValue:  "from-env",
			wantLayer:  LayerEnvironment,
			wantSource: "environment variable \"APP_NAME\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			cmd := &Command{
				Flags: []Flag{
					&StringFlag{
						Name:        "name",
						Value:       "code-default",
						Sources:     NewValueSourceChain(EnvVar("APP_NAME"), File(path)),
						Destination: &got,
					},
				},
				Action: func(ctx context.Context, c *Command) error { return nil },
			}
			require.NoError(t, cmd.Run(context.Background(), append([]string{"app"}, tt.args...)))
			require.Equal(t, tt.wantValue, got)

			origin, ok := cmd.ValueSource("name")
			require.True(t, ok)
			require.Equal(t, tt.wantLayer, origin.Layer)
			require.Contains(t, origin.String(), tt.wantSource)
		})
	}
}

func TestValueOriginFileBeatsCodeDefault(t *testing.T) {
	path := writeFileSource(t, "from-file")

	var got string
	cmd := &Command{
		Flags: []Flag{
			&StringFlag{
				Name:        "name",
				Value:       "code-default",
				Sources:     Files(path),
				Destination: &got,
			},
		},
		Action: func(ctx context.Context, c *Command) error { return nil },
	}
	require.NoError(t, cmd.Run(context.Background(), []string{"app"}))
	require.Equal(t, "from-file", got)

	origin, ok := cmd.ValueSource("name")
	require.True(t, ok)
	require.Equal(t, LayerFile, origin.Layer)
}

func TestValueOriginCodeDefault(t *testing.T) {
	var got string
	cmd := &Command{
		Flags: []Flag{
			&StringFlag{
				Name:        "name",
				Value:       "code-default",
				Destination: &got,
			},
		},
		Action: func(ctx context.Context, c *Command) error { return nil },
	}
	require.NoError(t, cmd.Run(context.Background(), []string{"app"}))
	require.Equal(t, "code-default", got)

	origin, ok := cmd.ValueSource("name")
	require.True(t, ok)
	require.Equal(t, LayerDefault, origin.Layer)
	require.Contains(t, origin.String(), "code default")
}

func TestValueOriginInt(t *testing.T) {
	t.Setenv("APP_COUNT", "42")

	var got int64
	cmd := &Command{
		Flags: []Flag{
			&Int64Flag{
				Name:        "count",
				Value:       1,
				Sources:     EnvVars("APP_COUNT"),
				Destination: &got,
			},
		},
		Action: func(ctx context.Context, c *Command) error { return nil },
	}
	require.NoError(t, cmd.Run(context.Background(), []string{"app", "--count", "7"}))
	require.Equal(t, int64(7), got)

	origin, ok := cmd.ValueSource("count")
	require.True(t, ok)
	require.Equal(t, LayerCommandLine, origin.Layer)
}

func TestValueOriginIntFromEnv(t *testing.T) {
	t.Setenv("APP_COUNT", "42")

	var got int64
	cmd := &Command{
		Flags: []Flag{
			&Int64Flag{
				Name:        "count",
				Value:       1,
				Sources:     EnvVars("APP_COUNT"),
				Destination: &got,
			},
		},
		Action: func(ctx context.Context, c *Command) error { return nil },
	}
	require.NoError(t, cmd.Run(context.Background(), []string{"app"}))
	require.Equal(t, int64(42), got)

	origin, ok := cmd.ValueSource("count")
	require.True(t, ok)
	require.Equal(t, LayerEnvironment, origin.Layer)
}

func TestValueOriginBoolFromCLI(t *testing.T) {
	var got bool
	cmd := &Command{
		Flags: []Flag{
			&BoolFlag{
				Name:        "verbose",
				Destination: &got,
			},
		},
		Action: func(ctx context.Context, c *Command) error { return nil },
	}
	require.NoError(t, cmd.Run(context.Background(), []string{"app", "--verbose"}))
	require.True(t, got)

	origin, ok := cmd.ValueSource("verbose")
	require.True(t, ok)
	require.Equal(t, LayerCommandLine, origin.Layer)
}

// Empty env value for a string flag is still an environment hit, distinct
// from the variable being unset (which falls through to code default).
func TestValueOriginEmptyEnvVersusUnsetString(t *testing.T) {
	t.Run("empty text from env", func(t *testing.T) {
		t.Setenv("APP_NAME", "")

		var got string = "untouched"
		cmd := &Command{
			Flags: []Flag{
				&StringFlag{
					Name:        "name",
					Value:       "code-default",
					Sources:     EnvVars("APP_NAME"),
					Destination: &got,
				},
			},
			Action: func(ctx context.Context, c *Command) error { return nil },
		}
		require.NoError(t, cmd.Run(context.Background(), []string{"app"}))
		require.Equal(t, "", got)

		origin, ok := cmd.ValueSource("name")
		require.True(t, ok)
		require.Equal(t, LayerEnvironment, origin.Layer)
	})

	t.Run("unset env falls back to code default", func(t *testing.T) {
		var got string
		cmd := &Command{
			Flags: []Flag{
				&StringFlag{
					Name:        "name",
					Value:       "code-default",
					Sources:     EnvVars("APP_NAME_DEFINITELY_UNSET_XYZ"),
					Destination: &got,
				},
			},
			Action: func(ctx context.Context, c *Command) error { return nil },
		}
		require.NoError(t, cmd.Run(context.Background(), []string{"app"}))
		require.Equal(t, "code-default", got)

		origin, ok := cmd.ValueSource("name")
		require.True(t, ok)
		require.Equal(t, LayerDefault, origin.Layer)
	})
}

// BoolWithInverse negative spelling (--no-env) is a command line source.
func TestValueOriginBoolWithInverseNegative(t *testing.T) {
	t.Setenv("ENV", "true")

	var got bool = true
	cmd := &Command{
		Flags: []Flag{
			&BoolWithInverseFlag{
				Name:        "env",
				Sources:     EnvVars("ENV", "NO-ENV"),
				Destination: &got,
			},
		},
		Action: func(ctx context.Context, c *Command) error { return nil },
	}
	require.NoError(t, cmd.Run(context.Background(), []string{"app", "--no-env"}))
	require.False(t, got)

	origin, ok := cmd.ValueSource("env")
	require.True(t, ok)
	require.Equal(t, LayerCommandLine, origin.Layer)
}

// A subcommand flag with the same name shadows the persistent parent flag;
// the subcommand's own value and origin win.
func TestValueOriginSubcommandShadowsParentFlag(t *testing.T) {
	t.Setenv("PARENT_ONLY", "parent-env")

	var parentVal, subVal string
	root := &Command{
		Name: "app",
		Flags: []Flag{
			&StringFlag{
				Name:        "name",
				Value:       "parent-default",
				Destination: &parentVal,
			},
		},
		Commands: []*Command{
			{
				Name: "sub",
				Flags: []Flag{
					&StringFlag{
						Name:        "name",
						Value:       "sub-default",
						Destination: &subVal,
					},
				},
				Action: func(ctx context.Context, c *Command) error {
					origin, ok := c.ValueSource("name")
					require.True(t, ok)
					require.Equal(t, LayerCommandLine, origin.Layer)
					return nil
				},
			},
		},
	}
	require.NoError(t, root.Run(context.Background(), []string{"app", "sub", "--name", "sub-cli"}))
	require.Equal(t, "sub-cli", subVal)
	require.Equal(t, "parent-default", parentVal)
}

// Persistent parent flag is inherited and resolvable from a subcommand.
func TestValueOriginPersistentFlagFromEnv(t *testing.T) {
	t.Setenv("PARENT_NAME", "parent-env")

	root := &Command{
		Name: "app",
		Flags: []Flag{
			&StringFlag{
				Name:    "pname",
				Value:   "parent-default",
				Sources: EnvVars("PARENT_NAME"),
			},
		},
		Commands: []*Command{
			{
				Name: "sub",
				Action: func(ctx context.Context, c *Command) error {
					require.Equal(t, "parent-env", c.String("pname"))
					origin, ok := c.ValueSource("pname")
					require.True(t, ok)
					require.Equal(t, LayerEnvironment, origin.Layer)
					return nil
				},
			},
		},
	}
	require.NoError(t, root.Run(context.Background(), []string{"app", "sub"}))
}

// A required flag that is missing keeps the standard error; source tracking
// must not swallow or alter it.
func TestValueOriginRequiredMissingError(t *testing.T) {
	cmd := &Command{
		Flags: []Flag{
			&StringFlag{
				Name:     "needed",
				Required: true,
			},
		},
		Action: func(ctx context.Context, c *Command) error {
			t.Fatal("action must not run when required flag is missing")
			return nil
		},
	}
	err := cmd.Run(context.Background(), []string{"app"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `Required flag "needed" not set`)
}

// Installing a chain with file-before-env violates the locked precedence and
// must fail closed, naming the offending layers.
func TestValueSourceOrderViolationRejected(t *testing.T) {
	path := writeFileSource(t, "from-file")

	cmd := &Command{
		Flags: []Flag{
			&StringFlag{
				Name:    "name",
				Sources: NewValueSourceChain(File(path), EnvVar("APP_NAME")),
			},
		},
	}
	err := cmd.Run(context.Background(), []string{"app"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "value source order violation")
	require.Contains(t, err.Error(), "environment")
}

// A well-formed env-then-file chain installs and merges as before.
func TestValueSourceOrderEnvThenFileAccepted(t *testing.T) {
	path := writeFileSource(t, "from-file")

	var got string
	cmd := &Command{
		Flags: []Flag{
			&StringFlag{
				Name:        "name",
				Value:       "code-default",
				Sources:     NewValueSourceChain(EnvVar("APP_NAME_UNSET_XYZ"), File(path)),
				Destination: &got,
			},
		},
		Action: func(ctx context.Context, c *Command) error { return nil },
	}
	require.NoError(t, cmd.Run(context.Background(), []string{"app"}))
	require.Equal(t, "from-file", got)
}

// Querying an unknown flag reports no origin rather than fabricating one.
func TestValueOriginUnknownFlag(t *testing.T) {
	cmd := &Command{
		Flags: []Flag{
			&StringFlag{Name: "name", Value: "x"},
		},
		Action: func(ctx context.Context, c *Command) error { return nil },
	}
	require.NoError(t, cmd.Run(context.Background(), []string{"app"}))
	_, ok := cmd.ValueSource("does-not-exist")
	require.False(t, ok)
}
