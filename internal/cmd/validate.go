package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/authzed/spicedb/pkg/development"
	core "github.com/authzed/spicedb/pkg/proto/core/v1"
	devinterface "github.com/authzed/spicedb/pkg/proto/developer/v1"
	"github.com/authzed/spicedb/pkg/spiceerrors"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/authzed/spicedb/pkg/validationfile"
	"github.com/charmbracelet/lipgloss"

	"github.com/authzed/zed/internal/console"
	"github.com/authzed/zed/internal/decode"
)

var (
	success                = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render("Success!")
	errorPrefix            = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Render("error: ")
	errorMessageStyle      = lipgloss.NewStyle().Bold(true).Width(80)
	linePrefixStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	highlightedSourceStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	highlightedLineStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	codeStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	highlightedCodeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
)

func registerValidateCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(validateCmd)
}

var validateCmd = &cobra.Command{
	Use:   "validate <validation_file>",
	Short: "validate the given validation file",
	Example: `
	From a local file (with prefix):
		zed validate file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	From a local file (no prefix):
		zed validate authzed-x7izWU8_2Gw3.yaml

	From a gist:
		zed validate https://gist.github.com/ecordell/8e3b613a677e3c844742cf24421c08b6

	From a playground link:
		zed validate https://play.authzed.com/s/iksdFvCtvnkR/schema

	From pastebin:
		zed validate https://pastebin.com/8qU45rVK

	From a devtools instance:
		zed validate https://localhost:8443/download`,
	Args: cobra.ExactArgs(1),
	RunE: validateCmdFunc,
}

func validateCmdFunc(cmd *cobra.Command, args []string) error {
	// Parse the URL of the validation document to import.
	u, err := url.Parse(args[0])
	if err != nil {
		return err
	}

	decoder, err := decode.DecoderForURL(u)
	if err != nil {
		return err
	}

	// Decode the validation document.
	var parsed validationfile.ValidationFile
	validateContents, err := decoder(&parsed)
	if err != nil {
		var errWithSource spiceerrors.ErrorWithSource
		if errors.As(err, &errWithSource) {
			ouputErrorWithSource(validateContents, errWithSource)
		}

		return err
	}

	// Create the development context.
	ctx := cmd.Context()
	tuples := make([]*core.RelationTuple, 0, len(parsed.Relationships.Relationships))
	for _, rel := range parsed.Relationships.Relationships {
		tuples = append(tuples, tuple.MustFromRelationship(rel))
	}
	devCtx, devErrs, err := development.NewDevContext(ctx, &devinterface.RequestContext{
		Schema:        parsed.Schema.Schema,
		Relationships: tuples,
	})
	if err != nil {
		return err
	}
	if devErrs != nil {
		outputDeveloperErrors(validateContents, devErrs.InputErrors)
	}

	// Run assertions.
	adevErrs, aerr := development.RunAllAssertions(devCtx, &parsed.Assertions)
	if aerr != nil {
		return aerr
	}
	if adevErrs != nil {
		outputDeveloperErrors(validateContents, adevErrs)
	}

	// Run expected relations.
	_, erDevErrs, rerr := development.RunValidation(devCtx, &parsed.ExpectedRelations)
	if rerr != nil {
		return rerr
	}
	if erDevErrs != nil {
		outputDeveloperErrors(validateContents, erDevErrs)
	}

	fmt.Print(success)
	console.Printf(" - %d relationships loaded, %d assertions run, %d expected relations validated\n",
		len(tuples),
		len(parsed.Assertions.AssertTrue)+len(parsed.Assertions.AssertFalse),
		len(parsed.ExpectedRelations.ValidationMap),
	)
	return nil
}

func ouputErrorWithSource(validateContents []byte, errWithSource spiceerrors.ErrorWithSource) {
	lines := strings.Split(string(validateContents), "\n")

	console.Printf("%s%s\n", errorPrefix, errorMessageStyle.Render(errWithSource.Error()))
	errorLineNumber := int(errWithSource.LineNumber) - 1 // errWithSource.LineNumber is 1-indexed
	for i := errorLineNumber - 3; i < errorLineNumber+3; i++ {
		if i == errorLineNumber {
			renderLine(lines, i, errWithSource.SourceCodeString, errorLineNumber)
		} else {
			renderLine(lines, i, "", errorLineNumber)
		}
	}
	os.Exit(1)
}

func outputDeveloperErrors(validateContents []byte, devErrors []*devinterface.DeveloperError) {
	lines := strings.Split(string(validateContents), "\n")

	for _, devErr := range devErrors {
		outputDeveloperError(devErr, lines)
	}

	os.Exit(1)
}

func outputDeveloperError(devError *devinterface.DeveloperError, lines []string) {
	console.Printf("%s %s\n", errorPrefix, errorMessageStyle.Render(devError.Message))
	errorLineNumber := int(devError.Line) - 1 // devError.Line is 1-indexed
	for i := errorLineNumber - 3; i < errorLineNumber+3; i++ {
		if i == errorLineNumber {
			renderLine(lines, i, devError.Context, errorLineNumber)
		} else {
			renderLine(lines, i, "", errorLineNumber)
		}
	}

	console.Printf("\n\n")
}

func renderLine(lines []string, index int, highlight string, highlightLineIndex int) {
	if index < 0 || index >= len(lines) {
		return
	}

	lineNumberLength := len(fmt.Sprintf("%d", len(lines)))
	lineContents := lines[index]
	highlightIndex := strings.Index(lineContents, highlight)
	lineNumberStr := fmt.Sprintf("%d", index+1)
	spacer := strings.Repeat(" ", lineNumberLength)

	lineNumberStyle := linePrefixStyle
	lineContentsStyle := codeStyle
	if index == highlightLineIndex {
		lineNumberStyle = highlightedLineStyle
		lineContentsStyle = highlightedCodeStyle
	}

	if highlightIndex < 0 || len(highlight) == 0 {
		console.Printf(" %s | %s\n", lineNumberStyle.Render(lineNumberStr), lineContentsStyle.Render(lineContents))
	} else {
		console.Printf(" %s | %s%s%s\n",
			lineNumberStyle.Render(lineNumberStr),
			lineContentsStyle.Render(lineContents[0:highlightIndex]),
			highlightedSourceStyle.Render(highlight),
			lineContentsStyle.Render(lineContents[highlightIndex+len(highlight):]),
		)
		console.Printf(" %s | %s%s%s\n",
			lineNumberStyle.Render(spacer),
			strings.Repeat(" ", highlightIndex),
			highlightedSourceStyle.Render("^"),
			highlightedSourceStyle.Render(strings.Repeat("~", len(highlight)-1)),
		)
	}
}
