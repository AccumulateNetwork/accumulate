package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	. "github.com/russross/blackfriday/v2"
	"github.com/spf13/cobra"
	"github.com/traefik/yaegi/interp"
	"gitlab.com/accumulatenetwork/accumulate/cmd/play-accumulate/pkg"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	testing "gitlab.com/accumulatenetwork/accumulate/internal/testing"
)

var Flag = struct {
	Network string
}{}

var Command = &cobra.Command{
	Use:   "play-accumulate [files...]",
	Short: "Run Accumulate playbooks",
	Run:   run,
}

func init() {
	Command.Flags().StringVarP(&Flag.Network, "network", "n", "", "Run the test against a network")
}

var reYamlDoc = regexp.MustCompile("(?m)^---$")
var reCodeFence = regexp.MustCompile("^([^\\s\\{]*)(\\{[^\\n]*\\})?")

func run(_ *cobra.Command, filenames []string) {
	testing.EnableDebugFeatures()

	documents := make([]*Node, len(filenames))
	for i, filename := range filenames {
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
		documents[i] = parser.Parse(contents)
	}

	var C *client.Client
	var err error
	if Flag.Network != "" {
		C, err = client.New(Flag.Network)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating client %q: %v\n", Flag.Network, err)
			os.Exit(1)
		}
	}

	for i, document := range documents {
		S := &pkg.Session{
			Filename: filepath.Base(filenames[i]),
			Output: func(o ...pkg.Output) {
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
		if C == nil {
			S.UseSimulator(3)
		} else {
			S.SetStartTime(time.Now())
			S.UseNetwork(C)
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

			_, err := I.Eval(string(node.Literal))
			if err == nil {
				return GoToNext
			}

			var panic interp.Panic
			if !errors.As(err, &panic) {
				fmt.Fprintf(os.Stderr, "Failed(%q): %v\n", S.Filename, err)
				return Terminate
			}

			abort, ok := panic.Value.(pkg.Abort)
			if !ok {
				fmt.Fprintf(os.Stderr, "Panicked(%q): %v\n%s", S.Filename, panic.Value, panic.Stack)
				return Terminate
			}

			fmt.Fprintf(os.Stderr, "Aborted(%q): %v\n", S.Filename, abort.Value)
			return Terminate
		})
	}
}
