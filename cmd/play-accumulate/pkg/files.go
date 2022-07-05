package pkg

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	. "github.com/russross/blackfriday/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/ethan.reesor/vscode-notebooks/yaegi/interp"
	"gitlab.com/ethan.reesor/vscode-notebooks/yaegi/stdlib"
)

var reYamlDoc = regexp.MustCompile("(?m)^---$")
var reCodeFence = regexp.MustCompile("^([^\\s\\{]*)(\\{[^\\n]*\\})?")

func ExecuteFile(ctx context.Context, filename string, simBvns int, client *client.Client) error {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %q: %v\n", filename, err)
		os.Exit(1)
	}

	// Extract frontmatter
	ranges := reYamlDoc.FindAllIndex(contents, 2)
	if len(ranges) == 2 {
		contents = contents[ranges[1][1]:]
	}

	// Create a new parser for each document, because that's what works
	parser := New(WithExtensions(FencedCode))
	document := parser.Parse(contents)

	S := &Session{
		Filename: filepath.Base(filename),
		Output: func(o ...Output) {
			if len(o) == 0 {
				return
			}
			switch o[0].Mime {
			case "stderr":
				_, _ = os.Stderr.Write(o[0].Value)
			default:
				_, _ = os.Stdout.Write(o[0].Value)
			}
		},
	}
	if client == nil {
		S.UseSimulator(simBvns)
	} else {
		S.SetStartTime(time.Now())
		S.UseNetwork(client)
	}
	I := interp.New(interp.Options{})
	InterpUseSession(S, I)

	var level int
	var heading string
	document.Walk(func(node *Node, entering bool) WalkStatus {
		switch node.Type {
		case Heading:
			if entering {
				level, heading = node.Level, ""
			} else {
				color.Blue("\n%s %s\n\n", strings.Repeat("#", level), heading)
				level = 0
			}
			return GoToNext

		case CodeBlock:
			m := reCodeFence.FindSubmatch(node.Info)
			if len(m) < 2 || string(m[1]) != "go" {
				return GoToNext
			}
			// Continue

		default:
			if level > 0 {
				heading += string(node.Literal)
			}
			return GoToNext
		}

		_, err = I.EvalWithContext(ctx, string(node.Literal))
		if err == nil {
			return GoToNext
		}

		var panic interp.Panic
		if !errors.As(err, &panic) {
			fmt.Fprintf(os.Stderr, "Failed(%q): %v\n", S.Filename, err)
			return Terminate
		}

		abort, ok := panic.Value.(Abort)
		if !ok {
			fmt.Fprintf(os.Stderr, "Panicked(%q): %v\n%s", S.Filename, panic.Value, panic.Stack)
			return Terminate
		}

		fmt.Fprintf(os.Stderr, "Aborted(%q): %v\n", S.Filename, abort.Value)
		return Terminate
	})
	return err
}

func InterpUseSession(s *Session, I interface {
	Use(values interp.Exports) error
	Eval(src string) (res reflect.Value, err error)
}) {
	err := I.Use(stdlib.Symbols)
	if err != nil {
		panic(err)
	}

	pkg := map[string]reflect.Value{
		"ACME": reflect.ValueOf(protocol.AcmeUrl()),
	}

	sv := reflect.ValueOf(s)
	st := sv.Type()
	for i, n := 0, st.NumMethod(); i < n; i++ {
		m := st.Method(i)
		if !m.IsExported() {
			continue
		}

		pkg[m.Name] = sv.Method(i)
	}

	err = I.Use(interp.Exports{
		"accumulate/accumulate": pkg,
		"protocol/protocol": map[string]reflect.Value{
			// Transactions
			"CreateIdentity":     reflect.ValueOf((*protocol.CreateIdentity)(nil)),
			"CreateTokenAccount": reflect.ValueOf((*protocol.CreateTokenAccount)(nil)),
			"SendTokens":         reflect.ValueOf((*protocol.SendTokens)(nil)),
			"CreateDataAccount":  reflect.ValueOf((*protocol.CreateDataAccount)(nil)),
			"WriteData":          reflect.ValueOf((*protocol.WriteData)(nil)),
			"WriteDataTo":        reflect.ValueOf((*protocol.WriteDataTo)(nil)),
			"AcmeFaucet":         reflect.ValueOf((*protocol.AcmeFaucet)(nil)),
			"CreateToken":        reflect.ValueOf((*protocol.CreateToken)(nil)),
			"IssueTokens":        reflect.ValueOf((*protocol.IssueTokens)(nil)),
			"BurnTokens":         reflect.ValueOf((*protocol.BurnTokens)(nil)),
			"CreateKeyPage":      reflect.ValueOf((*protocol.CreateKeyPage)(nil)),
			"CreateKeyBook":      reflect.ValueOf((*protocol.CreateKeyBook)(nil)),
			"AddCredits":         reflect.ValueOf((*protocol.AddCredits)(nil)),
			"UpdateKeyPage":      reflect.ValueOf((*protocol.UpdateKeyPage)(nil)),
			"UpdateAccountAuth":  reflect.ValueOf((*protocol.UpdateAccountAuth)(nil)),
			"UpdateKey":          reflect.ValueOf((*protocol.UpdateKey)(nil)),

			// General
			"TokenRecipient": reflect.ValueOf((*protocol.TokenRecipient)(nil)),
		},
	})
	if err != nil {
		panic(err)
	}

	_, err = I.Eval(`
		import (
			. "accumulate"
			"protocol"
		)
	`)
	if err != nil {
		panic(err)
	}
}
