package qq

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type color string

const (
	bold     color = "\033[1m"
	yellow   color = "\033[33m"
	cyan     color = "\033[36m"
	endColor color = "\033[0m" // ANSI escape code for "reset everything"
)

var (
	// LogFile is the full path to the qq.log file.
	LogFile = filepath.Join(os.TempDir(), "qq.log")

	// set logger to output to stderr on init, but it will be replaced with
	// qq.log file when Log() is called. we have to open/close qq.log inside
	// every Log() call because it's the only way ensure qq.log is properly
	// closed.
	logger = log.New(os.Stderr, "", 0)

	// for grouping log messages by time of write
	timer = time.NewTimer(0)
)

// TODO: function comment here
func Log(a ...interface{}) {
	// get info about the func calling qq.Log()
	pc, file, line, ok := runtime.Caller(1)
	if ok {
		names, err := argNames(file, line)
		if err == nil {
			a = formatArgs(names, a)
		}

		logger.SetPrefix(prefix(pc, file, line))
	}

	// line break if more than 2s since last write (groups logs together)
	if wasRunning := timer.Reset(2 * time.Second); !wasRunning {
		logger.SetPrefix("\n" + logger.Prefix())
	}

	f := openLog()
	defer f.Close()
	logger.SetOutput(f)
	logger.Println(a...)
}

// argNames finds the qq.Log() call at the given filename/line number and
// returns its arguments as a slice of strings. If the argument is a literal,
// argNames will return an empty string at the index position of that argument.
// For example, qq.Log(ip, port, 5432) would return []string{"ip", "port", ""}.
// err will be non-nil if the source text cannot be parsed.
func argNames(filename string, line int) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}

	var names []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, is := n.(*ast.CallExpr)
		if !is {
			return true // visit next node
		}

		// is a function call, but on wrong line
		if fset.Position(call.End()).Line != line {
			return true
		}

		// is a function call on correct line, but not a qq function
		if !qqCall(call) {
			return true
		}

		for _, arg := range call.Args {
			names = append(names, argName(arg))
		}
		return true
	})

	return names, nil
}

// qqCall returns true if the given function call expression is for a qq
// function, e.g. qq.Log().
func qqCall(n *ast.CallExpr) bool {
	sel, is := n.Fun.(*ast.SelectorExpr) // example of SelectorExpr: a.B()
	if !is {
		return false
	}

	ident, is := sel.X.(*ast.Ident) // sel.X is
	if !is {
		return false
	}

	return ident.Name == "qq"
}

// argName returns the source text of the given argument if it's a variable or
// an expression. If the argument is something else, like a literal, argName
// returns an empty string.
func argName(arg ast.Expr) string {
	var name string
	switch a := arg.(type) {
	case *ast.Ident:
		if a.Obj.Kind == ast.Var {
			name = a.Obj.Name
		}
	case *ast.BinaryExpr,
		*ast.CallExpr,
		*ast.IndexExpr,
		*ast.KeyValueExpr,
		*ast.ParenExpr,
		*ast.SliceExpr,
		*ast.TypeAssertExpr,
		*ast.UnaryExpr:
		name = exprToString(arg)
	}
	return name
}

// exprToString returns the source text underlying the given ast.Expr.
func exprToString(arg ast.Expr) string {
	var buf bytes.Buffer
	fset := token.NewFileSet()
	printer.Fprint(&buf, fset, arg)
	return buf.String() // returns empty string if printer fails
}

// TODO: scrap this prefix and just use the timer value
func prefix(pc uintptr, file string, line int) string {
	t := time.Now().Format("15:04:05")
	shortFile := filepath.Base(file)
	callerName := runtime.FuncForPC(pc).Name()

	return fmt.Sprintf("[%s %s:%d %s] ", t, shortFile, line, callerName)
}

// openLog returns a file descriptor for the qq.log file.
func openLog() *os.File {
	fd, err := os.OpenFile(LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	return fd
}

// formatArgs turns a slice of arguments into pretty-printed strings. If the
// argument is a variable or an expression, it will be returned as a
// name=value string, e.g. "port=443", "3+2=5". Variable names, expressions, and
// values are colorized using ANSI escape codes.
func formatArgs(names []string, values []interface{}) []interface{} {
	formatted := make([]interface{}, len(values))
	for i := 0; i < len(values); i++ {
		val := fmt.Sprintf("%#v", values[i])
		val = colorize(val, cyan)

		if names[i] == "" {
			// arg is a literal
			formatted[i] = val
		} else {
			name := colorize(names[i], bold)
			formatted[i] = fmt.Sprintf("%s=%s", name, val)
		}
	}
	return formatted
}

// colorize returns the given text encapsulated in ANSI escape codes that
// give the text a color in the terminal.
func colorize(text string, c color) string {
	return string(c) + text + string(endColor)
}