package cli

import (
	"fmt"
	"os"
	"strings"
)

// cliSourceRecorder is implemented by flags that can record that their
// value was supplied directly on the command line. It is called from the
// existing Command.set merge step rather than from a parallel parser.
type cliSourceRecorder interface {
	RecordCLISource(name string)
}

// ValueSource is a source which can be used to look up a value,
// typically for use with a cli.Flag
type ValueSource interface {
	fmt.Stringer
	fmt.GoStringer

	// Lookup returns the value from the source and if it was found
	// or returns an empty string and false
	Lookup() (string, bool)
}

// EnvValueSource is to specifically detect env sources when
// printing help text
type EnvValueSource interface {
	IsFromEnv() bool
	Key() string
}

// MapSource is a source which can be used to look up a value
// based on a key
// typically for use with a cli.Flag
type MapSource interface {
	fmt.Stringer
	fmt.GoStringer

	// Lookup returns the value from the source based on key
	// and if it was found
	// or returns an empty string and false
	Lookup(string) (any, bool)
}

// ValueSourceChain contains an ordered series of ValueSource that
// allows for lookup where the first ValueSource to resolve is
// returned
type ValueSourceChain struct {
	Chain []ValueSource
}

// ValueSourceLayer identifies which layer of the value merge chain a
// resolved value came from. Layers earlier in the documented precedence
// always override later layers:
//
//	command line > environment > file/config > code default
type ValueSourceLayer string

const (
	// LayerCommandLine is the value provided on the command line,
	// e.g. --name foo. It has the highest precedence.
	LayerCommandLine ValueSourceLayer = "command-line"
	// LayerEnvironment is a value resolved from an environment
	// variable source.
	LayerEnvironment ValueSourceLayer = "environment"
	// LayerFile is a value resolved from a file or other structured
	// configuration source.
	LayerFile ValueSourceLayer = "file"
	// LayerDefault is the code default declared on the flag itself.
	// It has the lowest precedence.
	LayerDefault ValueSourceLayer = "default"
)

// layerOrder is the locked precedence of the layers. A source at a
// lower index always overrides sources at a higher index.
var layerOrder = map[ValueSourceLayer]int{
	LayerCommandLine: 0,
	LayerEnvironment: 1,
	LayerFile:        2,
	LayerDefault:     3,
}

// LayeredValueSource may be implemented by custom [ValueSource]
// implementations to declare which merge layer they belong to.
// Sources that do not implement this interface are classified as
// [LayerFile], since user-provided sources (altsrc YAML/JSON/TOML,
// http, maps) are configuration inputs rather than code defaults.
type LayeredValueSource interface {
	Layer() ValueSourceLayer
}

// sourceLayer classifies a resolved source into one of the locked
// merge layers.
func sourceLayer(src ValueSource) ValueSourceLayer {
	if src == nil {
		return LayerDefault
	}
	if ls, ok := src.(LayeredValueSource); ok {
		return ls.Layer()
	}
	if es, ok := src.(EnvValueSource); ok && es.IsFromEnv() {
		return LayerEnvironment
	}
	return LayerFile
}

// validateSourceOrder verifies that the declared sources of a chain do
// not place a higher-precedence layer after a lower-precedence one.
// The command line layer is not part of a flag's Sources chain and is
// always implicitly first. An out-of-order chain fails closed with an
// error naming both offending layers so that a misconfiguration cannot
// be silently installed.
func validateSourceOrder(chain []ValueSource) error {
	prev := -1
	var prevSrc ValueSource
	for _, src := range chain {
		layer := sourceLayer(src)
		cur, ok := layerOrder[layer]
		if !ok {
			return fmt.Errorf(
				"unknown value source layer %q for %s",
				layer, src,
			)
		}
		if prev >= 0 && cur < prev {
			return fmt.Errorf(
				"value source order violation: %s (layer %q) cannot be overridden by earlier %s (layer %q); expected precedence command-line > environment > file > default",
				prevSrc, layerName(prev),
				src, layer,
			)
		}
		prev = cur
		prevSrc = src
	}
	return nil
}

func layerName(layerIndex int) ValueSourceLayer {
	for layer, idx := range layerOrder {
		if idx == layerIndex {
			return layer
		}
	}
	return ValueSourceLayer("unknown")
}

// ValidateSourceOrder reports an error if the configured [ValueSource]
// chain violates the locked precedence
// (command-line > environment > file > default). It is invoked while
// flags are installed, so an invalid chain prevents the command from
// running rather than producing an undefined merge.
func (vsc *ValueSourceChain) ValidateSourceOrder() error {
	return validateSourceOrder(vsc.Chain)
}

// FlagValueSource records the winning origin of a flag value after the
// full merge chain has been resolved.
type FlagValueSource struct {
	// Layer is the merge layer that supplied the final value.
	Layer ValueSourceLayer
	// Source is the concrete [ValueSource] that supplied the value, or
	// nil when the value is the code default.
	Source ValueSource
	// FlagName is the flag the resolved value belongs to.
	FlagName string
}

func (fvs FlagValueSource) String() string {
	switch fvs.Layer {
	case LayerCommandLine:
		return fmt.Sprintf("command line flag %[1]q", fvs.FlagName)
	case LayerDefault:
		return fmt.Sprintf("code default for flag %[1]q", fvs.FlagName)
	default:
		if fvs.Source != nil {
			return fmt.Sprintf("%[1]s for flag %[2]q", fvs.Source, fvs.FlagName)
		}
		return fmt.Sprintf("%[1]s layer for flag %[2]q", fvs.Layer, fvs.FlagName)
	}
}

// SourceTracker is implemented by flags that can report the layer and
// concrete source which supplied their final merged value.
type SourceTracker interface {
	// ValueSource returns the winning origin after parsing, or false
	// when the flag is not part of an applied flag set.
	ValueSourceOrigin() (FlagValueSource, bool)
}

// cliValueSource marks a value supplied directly on the command line.
type cliValueSource struct {
	flagName string
}

func (c *cliValueSource) Lookup() (string, bool) { return "", false }
func (c *cliValueSource) String() string {
	return fmt.Sprintf("command line flag %[1]q", c.flagName)
}

func (c *cliValueSource) GoString() string {
	return fmt.Sprintf("&cliValueSource{flagName:%[1]q}", c.flagName)
}
func (c *cliValueSource) Layer() ValueSourceLayer { return LayerCommandLine }

// defaultCodeSource marks the code default declared on a flag.
type defaultCodeSource struct {
	flagName string
}

func (d *defaultCodeSource) Lookup() (string, bool) { return "", false }
func (d *defaultCodeSource) String() string {
	return fmt.Sprintf("code default for flag %[1]q", d.flagName)
}

func (d *defaultCodeSource) GoString() string {
	return fmt.Sprintf("&defaultCodeSource{flagName:%[1]q}", d.flagName)
}
func (d *defaultCodeSource) Layer() ValueSourceLayer { return LayerDefault }

var _ LayeredValueSource = (*defaultCodeSource)(nil)

func NewValueSourceChain(src ...ValueSource) ValueSourceChain {
	return ValueSourceChain{
		Chain: src,
	}
}

func (vsc *ValueSourceChain) Append(other ValueSourceChain) {
	vsc.Chain = append(vsc.Chain, other.Chain...)
}

func (vsc *ValueSourceChain) EnvKeys() []string {
	vals := []string{}

	for _, src := range vsc.Chain {
		if v, ok := src.(EnvValueSource); ok && v.IsFromEnv() {
			vals = append(vals, v.Key())
		}
	}

	return vals
}

func (vsc *ValueSourceChain) String() string {
	s := []string{}

	for _, vs := range vsc.Chain {
		s = append(s, vs.String())
	}

	return strings.Join(s, ",")
}

func (vsc *ValueSourceChain) GoString() string {
	s := []string{}

	for _, vs := range vsc.Chain {
		s = append(s, vs.GoString())
	}

	return fmt.Sprintf("&ValueSourceChain{Chain:{%[1]s}}", strings.Join(s, ","))
}

func (vsc *ValueSourceChain) Lookup() (string, bool) {
	s, _, ok := vsc.LookupWithSource()
	return s, ok
}

func (vsc *ValueSourceChain) LookupWithSource() (string, ValueSource, bool) {
	for _, src := range vsc.Chain {
		if value, found := src.Lookup(); found {
			return value, src, true
		}
	}

	return "", nil, false
}

// envVarValueSource encapsulates a ValueSource from an environment variable
type envVarValueSource struct {
	key string
}

func (e *envVarValueSource) Lookup() (string, bool) {
	return os.LookupEnv(strings.TrimSpace(string(e.key)))
}

func (e *envVarValueSource) IsFromEnv() bool {
	return true
}

func (e *envVarValueSource) Key() string {
	return e.key
}

func (e *envVarValueSource) Layer() ValueSourceLayer { return LayerEnvironment }

func (e *envVarValueSource) String() string { return fmt.Sprintf("environment variable %[1]q", e.key) }

func (e *envVarValueSource) GoString() string {
	return fmt.Sprintf("&envVarValueSource{Key:%[1]q}", e.key)
}

func EnvVar(key string) ValueSource {
	return &envVarValueSource{
		key: key,
	}
}

// EnvVars is a helper function to encapsulate a number of
// envVarValueSource together as a ValueSourceChain
func EnvVars(keys ...string) ValueSourceChain {
	vsc := ValueSourceChain{Chain: []ValueSource{}}

	for _, key := range keys {
		vsc.Chain = append(vsc.Chain, EnvVar(key))
	}

	return vsc
}

// fileValueSource encapsulates a ValueSource from a file
type fileValueSource struct {
	Path string
}

func (f *fileValueSource) Lookup() (string, bool) {
	data, err := os.ReadFile(f.Path)
	return string(data), err == nil
}

func (f *fileValueSource) String() string { return fmt.Sprintf("file %[1]q", f.Path) }
func (f *fileValueSource) GoString() string {
	return fmt.Sprintf("&fileValueSource{Path:%[1]q}", f.Path)
}
func (f *fileValueSource) Layer() ValueSourceLayer { return LayerFile }

func File(path string) ValueSource {
	return &fileValueSource{Path: path}
}

// Files is a helper function to encapsulate a number of
// fileValueSource together as a ValueSourceChain
func Files(paths ...string) ValueSourceChain {
	vsc := ValueSourceChain{Chain: []ValueSource{}}

	for _, path := range paths {
		vsc.Chain = append(vsc.Chain, File(path))
	}

	return vsc
}

type mapSource struct {
	name string
	m    map[any]any
}

func NewMapSource(name string, m map[any]any) MapSource {
	return &mapSource{
		name: name,
		m:    m,
	}
}

func (ms *mapSource) String() string { return fmt.Sprintf("map source %[1]q", ms.name) }
func (ms *mapSource) GoString() string {
	return fmt.Sprintf("&mapSource{name:%[1]q}", ms.name)
}

// Lookup returns a value from the map source. The lookup name may be a dot-separated path into the map.
// If that is the case, it will recursively traverse the map based on the '.' delimited sections to find
// a nested value for the key.
func (ms *mapSource) Lookup(name string) (any, bool) {
	sections := strings.Split(name, ".")
	if name == "" || len(sections) == 0 {
		return nil, false
	}

	node := ms.m

	// traverse into the map based on the dot-separated sections
	if len(sections) >= 2 { // the last section is the value we want, we will return it directly at the end
		for _, section := range sections[:len(sections)-1] {
			child, ok := node[section]
			if !ok {
				return nil, false
			}

			switch child := child.(type) {
			case map[string]any:
				node = make(map[any]any, len(child))
				for k, v := range child {
					node[k] = v
				}
			case map[any]any:
				node = child
			default:
				return nil, false
			}
		}
	}

	if val, ok := node[sections[len(sections)-1]]; ok {
		return val, true
	}
	return nil, false
}

type mapValueSource struct {
	key string
	ms  MapSource
}

func NewMapValueSource(key string, ms MapSource) ValueSource {
	return &mapValueSource{
		key: key,
		ms:  ms,
	}
}

func (mvs *mapValueSource) String() string {
	return fmt.Sprintf("key %[1]q from %[2]s", mvs.key, mvs.ms.String())
}

func (mvs *mapValueSource) GoString() string {
	return fmt.Sprintf("&mapValueSource{key:%[1]q, src:%[2]s}", mvs.key, mvs.ms.GoString())
}

func (mvs *mapValueSource) Lookup() (string, bool) {
	if v, ok := mvs.ms.Lookup(mvs.key); !ok {
		return "", false
	} else {
		return fmt.Sprintf("%+v", v), true
	}
}
