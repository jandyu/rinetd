// Package strd implements github.com/go-logr/logr.Logger in terms of
// Go's standard log package.
// copy from "github.com/go-logr/stdr" with string not enscaped html
package stdr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"sort"
	"strconv"

	"github.com/go-logr/logr"
)

// The global verbosity level.  See SetVerbosity().
var globalVerbosity int = 0

// SetVerbosity sets the global level against which all info logs will be
// compared.  If this is greater than or equal to the "V" of the logger, the
// message will be logged.  A higher value here means more logs will be written.
// The previous verbosity value is returned.  This is not concurrent-safe -
// callers must be sure to call it from only one goroutine.
func SetVerbosity(v int) int {
	old := globalVerbosity
	globalVerbosity = v
	return old
}

// New returns a logr.Logger which is implemented by Go's standard log package,
// or something like it.  If std is nil, this will call functions in the log
// package instead.
//
// Example: stdr.New(log.New(os.Stderr, "", log.LstdFlags)))
func New(std StdLogger) logr.Logger {
	return NewWithOptions(std, Options{})
}

// NewWithOptions returns a logr.Logger which is implemented by Go's standard
// log package, or something like it.  See New for details.
func NewWithOptions(std StdLogger, opts Options) logr.Logger {
	if opts.Depth < 0 {
		opts.Depth = 0
	}

	return logger{
		std:    std,
		level:  0,
		prefix: "",
		values: nil,
		depth:  opts.Depth,
	}
}

type Options struct {
	// DepthOffset biases the assumed number of call frames to the "true"
	// caller.  This is useful when the calling code calls a function which then
	// calls glogr (e.g. a logging shim to another API).  Values less than zero
	// will be treated as zero.
	Depth int
}

// StdLogger is the subset of the Go stdlib log.Logger API that is needed for
// this adapter.
type StdLogger interface {
	// Output is the same as log.Output and log.Logger.Output.
	Output(calldepth int, logline string) error
}

type logger struct {
	std    StdLogger
	level  int
	prefix string
	values []interface{}
	depth  int
}

func (l logger) clone() logger {
	out := l
	l.values = copySlice(l.values)
	return out
}

func copySlice(in []interface{}) []interface{} {
	out := make([]interface{}, len(in))
	copy(out, in)
	return out
}

// Magic string for intermediate frames that we should ignore.
const autogeneratedFrameName = "<autogenerated>"

// Discover how many frames we need to climb to find the caller. This approach
// was suggested by Ian Lance Taylor of the Go team, so it *should* be safe
// enough (famous last words).
func framesToCaller() int {
	// 1 is the immediate caller.  3 should be too many.
	for i := 1; i < 3; i++ {
		_, file, _, _ := runtime.Caller(i + 1) // +1 for this function's frame
		if file != autogeneratedFrameName {
			return i
		}
	}
	return 1 // something went wrong, this is safe
}

func flatten(kvList ...interface{}) string {
	keys := make([]string, 0, len(kvList))
	vals := make(map[string]interface{}, len(kvList))
	for i := 0; i < len(kvList); i += 2 {
		k, ok := kvList[i].(string)
		if !ok {
			panic(fmt.Sprintf("key is not a string: %s", pretty(kvList[i])))
		}
		var v interface{}
		if i+1 < len(kvList) {
			v = kvList[i+1]
		}
		keys = append(keys, k)
		vals[k] = v
	}
	sort.Strings(keys)
	buf := bytes.Buffer{}
	for i, k := range keys {
		v := vals[k]
		if i > 0 {
			buf.WriteRune(' ')
		}
		buf.WriteString(pretty(k))
		buf.WriteString("=")
		buf.WriteString(pretty(v))
	}
	return buf.String()
}

// JSONMarshal same with json.Marshal but different is
// JSONMarshal will add '\n' at the tail of value
func JSONMarshal(t interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(true)
	err := encoder.Encode(t)
	return buffer.Bytes(), err
}

func pretty(value interface{}) string {
	switch value.(type) {
	case string:
		return value.(string)
	case int:
		return strconv.Itoa(value.(int))
	default:
		b, _ := json.Marshal(value)
		return string(b)
	}
}

func (l logger) Info(msg string, kvList ...interface{}) {
	if l.Enabled() {
		lvlStr := flatten("level", l.level)
		msgStr := flatten("msg", msg)
		fixedStr := flatten(l.values...)
		userStr := flatten(kvList...)
		l.output(framesToCaller()+l.depth, fmt.Sprintln(l.prefix, lvlStr, msgStr, fixedStr, userStr))
	}
}

func (l logger) Enabled() bool {
	return globalVerbosity >= l.level
}

func (l logger) Error(err error, msg string, kvList ...interface{}) {
	msgStr := flatten("msg", msg)
	var loggableErr interface{}
	if err != nil {
		loggableErr = err.Error()
	}
	errStr := flatten("error", loggableErr)
	fixedStr := flatten(l.values...)
	userStr := flatten(kvList...)
	l.output(framesToCaller()+l.depth, fmt.Sprintln(l.prefix, errStr, msgStr, fixedStr, userStr))
}

func (l logger) output(calldepth int, s string) {
	depth := calldepth + 2 // offset for this adapter

	// ignore errors - what can we really do about them?
	if l.std != nil {
		_ = l.std.Output(depth, s)
	} else {
		_ = log.Output(depth, s)
	}
}

func (l logger) V(level int) logr.Logger {
	new1 := l.clone()
	new1.level += level
	return new1
}

// WithName returns a new logr.Logger with the specified name appended.  stdr
// uses '/' characters to separate name elements.  Callers should not pass '/'
// in the provided name string, but this library does not actually enforce that.
func (l logger) WithName(name string) logr.Logger {
	new1 := l.clone()
	if len(l.prefix) > 0 {
		new1.prefix = l.prefix + "/"
	}
	new1.prefix += name
	return new1
}

func (l logger) WithValues(kvList ...interface{}) logr.Logger {
	new1 := l.clone()
	new1.values = append(new1.values, kvList...)
	return new1
}

var _ logr.Logger = logger{}
