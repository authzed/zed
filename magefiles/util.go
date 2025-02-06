//go:build mage
// +build mage

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type byName []*cobra.Command

type CommandContent struct {
	Name    string
	Content string
}

func (s byName) Len() int           { return len(s) }
func (s byName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byName) Less(i, j int) bool { return s[i].Name() < s[j].Name() }

func GenCustomMarkdownTree(cmd *cobra.Command, dir string) error {
	basename := strings.ReplaceAll(cmd.CommandPath(), " ", "_") + ".md"
	filename := filepath.Join(dir, basename)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return genMarkdownTreeCustom(cmd, f)
}

func genMarkdownTreeCustom(cmd *cobra.Command, f *os.File) error {
	var commandContents []CommandContent

	collectCommandContent(cmd, &commandContents)

	// for sorting commands and their content
	sort.Slice(commandContents, func(i, j int) bool {
		return commandContents[i].Name < commandContents[j].Name
	})

	for _, cc := range commandContents {
		_, err := f.WriteString(cc.Content)
		if err != nil {
			return err
		}
	}

	return nil
}

func collectCommandContent(cmd *cobra.Command, commandContents *[]CommandContent) {
	buf := new(bytes.Buffer)
	name := cmd.CommandPath()

	buf.WriteString("## " + name + "\n\n")
	buf.WriteString(cmd.Short + "\n\n")
	if len(cmd.Long) > 0 {
		buf.WriteString("### Synopsis\n\n")
		buf.WriteString(cmd.Long + "\n\n")
	}

	if cmd.Runnable() {
		buf.WriteString(fmt.Sprintf("```\n%s\n```\n\n", cmd.UseLine()))
	}

	if len(cmd.Example) > 0 {
		buf.WriteString("### Examples\n\n")
		buf.WriteString(fmt.Sprintf("```\n%s\n```\n\n", cmd.Example))
	}

	if err := printOptions(buf, cmd); err != nil {
		fmt.Println("Error printing options:", err)
	}

	if hasSeeAlso(cmd) {
		buf.WriteString("### SEE ALSO\n\n")
		if cmd.HasParent() {
			parent := cmd.Parent()
			pname := parent.CommandPath()
			pname = strings.ReplaceAll(strings.ReplaceAll(pname, "_", "-"), " ", "-")

			buf.WriteString(fmt.Sprintf("* [%s](#%s)\t - %s\n", pname, pname, parent.Short))
		}

		children := cmd.Commands()
		sort.Sort(byName(children))

		for _, child := range children {
			if !child.IsAvailableCommand() || child.IsAdditionalHelpTopicCommand() {
				continue
			}
			cname := name + " " + child.Name()
			link := strings.ReplaceAll(strings.ReplaceAll(cname, "_", "-"), " ", "-")
			buf.WriteString(fmt.Sprintf("* [%s](#%s)\t - %s\n", cname, link, child.Short))
		}
		buf.WriteString("\n\n")
	}

	*commandContents = append(*commandContents, CommandContent{
		Name:    name,
		Content: buf.String(),
	})

	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		collectCommandContent(c, commandContents)
	}
}

func hasSeeAlso(cmd *cobra.Command) bool {
	if cmd.HasParent() {
		return true
	}
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		return true
	}
	return false
}

func printOptions(buf *bytes.Buffer, cmd *cobra.Command) error {
	flags := cmd.NonInheritedFlags()
	flags.SetOutput(buf)

	if flags.HasAvailableFlags() {
		buf.WriteString("### Options\n\n```\n")
		flags.PrintDefaults()
		buf.WriteString("```\n\n")
	}

	parentFlags := cmd.InheritedFlags()
	parentFlags.SetOutput(buf)

	if parentFlags.HasAvailableFlags() {
		buf.WriteString("### Options Inherited From Parent Flags\n\n```\n")
		parentFlags.PrintDefaults()
		buf.WriteString("```\n\n")
	}

	return nil
}
