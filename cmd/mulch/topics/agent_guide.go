package topics

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

//go:embed agent_guide.md
var agentGuideText string

// agentGuideCmd represents the "agent-guide" command
var agentGuideCmd = &cobra.Command{
	Use:   "agent-guide",
	Short: "Display a usage guide for AI agents / LLMs",
	Long: `Display a Markdown guide for AI agents and LLMs driving this client:
concepts, safety rules, output handling and a full command reference.

This command does not require any configuration file.`,
	Aliases:     []string{"llm"},
	Hidden:      true,
	Args:        cobra.NoArgs,
	Annotations: map[string]string{annotationNoConfig: "true"},
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Print(agentGuideText)
		fmt.Print(agentGuideReference())
	},
}

func init() {
	rootCmd.AddCommand(agentGuideCmd)
}

// agentGuideReference generates a Markdown command reference from the
// cobra command tree, so it can't diverge from the real client
func agentGuideReference() string {
	var sb strings.Builder

	sb.WriteString("\n## Command reference (generated)\n\n")
	sb.WriteString("Global flags (usable with any command):\n\n")
	agentGuideFlags(&sb, rootCmd.PersistentFlags())

	sb.WriteString("\nCommands (`command <args> — description [flag: description] (aliases)`).\nThis is an index: run `<command> --help` for details before using a command.\n\n```text\n")
	for _, cmd := range rootCmd.Commands() {
		agentGuideCommand(&sb, cmd)
	}
	sb.WriteString("```\n")
	return sb.String()
}

// agentGuideCommand writes one line per runnable command, recursively
func agentGuideCommand(sb *strings.Builder, cmd *cobra.Command) {
	if cmd.Hidden || cmd.Name() == "help" || cmd.Name() == "completion" || cmd.Annotations[annotationNoConfig] == "true" {
		return
	}

	if cmd.Runnable() {
		fmt.Fprintf(sb, "%s %s — %s", cmd.Parent().CommandPath(), cmd.Use, cmd.Short)
		cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden || f.Name == "help" {
				return
			}
			_, usage := pflag.UnquoteUsage(f)
			fmt.Fprintf(sb, " [%s: %s]", agentGuideFlagName(f, "/"), usage)
		})
		if len(cmd.Aliases) > 0 {
			fmt.Fprintf(sb, " (%s)", strings.Join(cmd.Aliases, ", "))
		}
		sb.WriteString("\n")
	}

	for _, sub := range cmd.Commands() {
		agentGuideCommand(sb, sub)
	}
}

// agentGuideFlagName returns "-f<sep>--flag <type>"
func agentGuideFlagName(f *pflag.Flag, sep string) string {
	name := "--" + f.Name
	if f.Shorthand != "" {
		name = "-" + f.Shorthand + sep + name
	}
	varname, _ := pflag.UnquoteUsage(f)
	if varname != "" {
		name += " <" + varname + ">"
	}
	return name
}

func agentGuideFlags(sb *strings.Builder, flags *pflag.FlagSet) {
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		_, usage := pflag.UnquoteUsage(f)
		fmt.Fprintf(sb, "- `%s`: %s\n", agentGuideFlagName(f, ", "), usage)
	})
}
