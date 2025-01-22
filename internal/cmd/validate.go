package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ccoveille/go-safecast"
	"github.com/spf13/cobra"

	"github.com/authzed/spicedb/pkg/development"
	core "github.com/authzed/spicedb/pkg/proto/core/v1"
	devinterface "github.com/authzed/spicedb/pkg/proto/developer/v1"
	"github.com/authzed/spicedb/pkg/spiceerrors"
	"github.com/authzed/spicedb/pkg/validationfile"
	"github.com/charmbracelet/lipgloss"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/muesli/termenv"

	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/console"
	"github.com/authzed/zed/internal/decode"
	"github.com/authzed/zed/internal/printers"
)

var (
	// NOTE: these need to be set *after* the renderer has been set, otherwise
	// the forceColor setting can't work, hence the thunking.
	success = func() string {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render("Success!")
	}
	complete      = func() string { return lipgloss.NewStyle().Bold(true).Render("complete") }
	errorPrefix   = func() string { return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Render("error: ") }
	warningPrefix = func() string {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")).Render("warning: ")
	}
	errorMessageStyle      = func() lipgloss.Style { return lipgloss.NewStyle().Bold(true).Width(80) }
	linePrefixStyle        = func() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("12")) }
	highlightedSourceStyle = func() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("9")) }
	highlightedLineStyle   = func() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("9")) }
	codeStyle              = func() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("8")) }
	highlightedCodeStyle   = func() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("15")) }
	traceStyle             = func() lipgloss.Style { return lipgloss.NewStyle().Bold(true) }
)

func registerValidateCmd(cmd *cobra.Command) {
	validateCmd.Flags().Bool("force-color", false, "force color code output even in non-tty environments")
	cmd.AddCommand(validateCmd)
}

var validateCmd = &cobra.Command{
	Use:   "validate <validation_file_or_schema_file>",
	Short: "Validates the given validation file (.yaml, .zaml) or schema file (.zed)",
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
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: commands.FileExtensionCompletions("zed", "yaml", "zaml"),
	PreRunE:           validatePreRunE,
	RunE:              validateCmdFunc,

	// A schema that causes the parser/compiler to error will halt execution
	// of this command with an error. In that case, we want to just display the error,
	// rather than showing usage for this command.
	SilenceUsage: true,
}

func validatePreRunE(cmd *cobra.Command, _ []string) error {
	// Override lipgloss's autodetection of whether it's in a terminal environment
	// and display things in color anyway. This can be nice in CI environments that
	// support it.
	setForceColor := cobrautil.MustGetBool(cmd, "force-color")
	if setForceColor {
		lipgloss.SetColorProfile(termenv.ANSI256)
	}

	return nil
}

func validateCmdFunc(cmd *cobra.Command, filenames []string) error {
	// Initialize variables for multiple files
	var (
		totalFiles                 = len(filenames)
		successfullyValidatedFiles = 0
	)

	for _, filename := range filenames {
		// If we're running over multiple files, print the filename for context/debugging purposes
		if totalFiles > 1 {
			console.Println(filename)
		}

		u, err := url.Parse(filename)
		if err != nil {
			return err
		}

		decoder, err := decode.DecoderForURL(u)
		if err != nil {
			return err
		}

		var parsed validationfile.ValidationFile
		validateContents, isOnlySchema, err := decoder(&parsed)
		if err != nil {
			var errWithSource spiceerrors.WithSourceError
			if errors.As(err, &errWithSource) {
				ouputErrorWithSource(validateContents, errWithSource)
			}
			return err
		}

		tuples := make([]*core.RelationTuple, 0)
		totalAssertions := 0
		totalRelationsValidated := 0

		for _, rel := range parsed.Relationships.Relationships {
			tuples = append(tuples, rel.ToCoreTuple())
		}

		// Create the development context for each run
		ctx := cmd.Context()
		devCtx, devErrs, err := development.NewDevContext(ctx, &devinterface.RequestContext{
			Schema:        parsed.Schema.Schema,
			Relationships: tuples,
		})
		if err != nil {
			return err
		}
		if devErrs != nil {
			schemaOffset := 1 /* for the 'schema:' */
			if isOnlySchema {
				schemaOffset = 0
			}

			// Output errors
			outputDeveloperErrorsWithLineOffset(validateContents, devErrs.InputErrors, schemaOffset)
		}
		// Run assertions
		adevErrs, aerr := development.RunAllAssertions(devCtx, &parsed.Assertions)
		if aerr != nil {
			return aerr
		}
		if adevErrs != nil {
			outputDeveloperErrors(validateContents, adevErrs)
		}
		successfullyValidatedFiles++

		// Run expected relations for all parsed files
		_, erDevErrs, rerr := development.RunValidation(devCtx, &parsed.ExpectedRelations)
		if rerr != nil {
			return rerr
		}
		if erDevErrs != nil {
			outputDeveloperErrors(validateContents, erDevErrs)
		}
		// Print out any warnings for all files
		warnings, err := development.GetWarnings(ctx, devCtx)
		if err != nil {
			return err
		}
		if len(warnings) > 0 {
			for _, warning := range warnings {
				console.Printf("%s%s\n", warningPrefix(), warning.Message)
				outputForLine(validateContents, uint64(warning.Line), warning.SourceCode, uint64(warning.Column)) // warning.LineNumber is 1-indexed
				console.Printf("\n")
			}

			console.Print(complete())
		} else {
			console.Print(success())
		}
		totalAssertions += len(parsed.Assertions.AssertTrue) + len(parsed.Assertions.AssertFalse)
		totalRelationsValidated += len(parsed.ExpectedRelations.ValidationMap)

		console.Printf(" - %d relationships loaded, %d assertions run, %d expected relations validated\n",
			len(tuples),
			totalAssertions,
			totalRelationsValidated,
		)
	}

	if totalFiles > 1 {
		console.Printf("total files: %d, successfully validated files: %d\n", totalFiles, successfullyValidatedFiles)
	}
	return nil
}

func ouputErrorWithSource(validateContents []byte, errWithSource spiceerrors.WithSourceError) {
	console.Printf("%s%s\n", errorPrefix(), errorMessageStyle().Render(errWithSource.Error()))
	outputForLine(validateContents, errWithSource.LineNumber, errWithSource.SourceCodeString, 0) // errWithSource.LineNumber is 1-indexed
	os.Exit(1)
}

func outputForLine(validateContents []byte, oneIndexedLineNumber uint64, sourceCodeString string, oneIndexedColumnPosition uint64) {
	lines := strings.Split(string(validateContents), "\n")
	// These should be fine to be zero if the cast fails.
	intLineNumber, _ := safecast.ToInt(oneIndexedLineNumber)
	intColumnPosition, _ := safecast.ToInt(oneIndexedColumnPosition)
	errorLineNumber := intLineNumber - 1
	for i := errorLineNumber - 3; i < errorLineNumber+3; i++ {
		if i == errorLineNumber {
			renderLine(lines, i, sourceCodeString, errorLineNumber, intColumnPosition-1)
		} else {
			renderLine(lines, i, "", errorLineNumber, -1)
		}
	}
}

func outputDeveloperErrors(validateContents []byte, devErrors []*devinterface.DeveloperError) {
	outputDeveloperErrorsWithLineOffset(validateContents, devErrors, 0)
}

func outputDeveloperErrorsWithLineOffset(validateContents []byte, devErrors []*devinterface.DeveloperError, lineOffset int) {
	lines := strings.Split(string(validateContents), "\n")

	for _, devErr := range devErrors {
		outputDeveloperError(devErr, lines, lineOffset)
	}

	os.Exit(1)
}

func outputDeveloperError(devError *devinterface.DeveloperError, lines []string, lineOffset int) {
	console.Printf("%s %s\n", errorPrefix(), errorMessageStyle().Render(devError.Message))
	errorLineNumber := int(devError.Line) - 1 + lineOffset // devError.Line is 1-indexed
	for i := errorLineNumber - 3; i < errorLineNumber+3; i++ {
		if i == errorLineNumber {
			renderLine(lines, i, devError.Context, errorLineNumber, -1)
		} else {
			renderLine(lines, i, "", errorLineNumber, -1)
		}
	}

	if devError.CheckResolvedDebugInformation != nil && devError.CheckResolvedDebugInformation.Check != nil {
		console.Printf("\n  %s\n", traceStyle().Render("Explanation:"))
		tp := printers.NewTreePrinter()
		printers.DisplayCheckTrace(devError.CheckResolvedDebugInformation.Check, tp, true)
		tp.PrintIndented()
	}

	console.Printf("\n\n")
}

func renderLine(lines []string, index int, highlight string, highlightLineIndex int, highlightStartingColumnIndex int) {
	if index < 0 || index >= len(lines) {
		return
	}

	lineNumberLength := len(fmt.Sprintf("%d", len(lines)))
	lineContents := strings.ReplaceAll(lines[index], "\t", " ")
	lineDelimiter := "|"

	highlightLength := max(0, len(highlight)-1)
	highlightColumnIndex := -1

	// If the highlight string was provided, then we need to find the index of the highlight
	// string in the line contents to determine where to place the caret.
	if len(highlight) > 0 {
		offset := 0
		for {
			foundRelativeIndex := strings.Index(lineContents[offset:], highlight)
			foundIndex := foundRelativeIndex + offset
			if foundIndex >= highlightStartingColumnIndex {
				highlightColumnIndex = foundIndex
				break
			}

			offset = foundIndex + 1
			if foundRelativeIndex < 0 || foundIndex > len(lineContents) {
				break
			}
		}
	} else if highlightStartingColumnIndex >= 0 {
		// Otherwise, just show a caret at the specified starting column, if any.
		highlightColumnIndex = highlightStartingColumnIndex
	}

	lineNumberStr := fmt.Sprintf("%d", index+1)
	noNumberSpaces := strings.Repeat(" ", lineNumberLength)

	lineNumberStyle := linePrefixStyle
	lineContentsStyle := codeStyle
	if index == highlightLineIndex {
		lineNumberStyle = highlightedLineStyle
		lineContentsStyle = highlightedCodeStyle
		lineDelimiter = ">"
	}

	lineNumberSpacer := strings.Repeat(" ", lineNumberLength-len(lineNumberStr))

	if highlightColumnIndex < 0 {
		console.Printf(" %s%s %s %s\n", lineNumberSpacer, lineNumberStyle().Render(lineNumberStr), lineDelimiter, lineContentsStyle().Render(lineContents))
	} else {
		console.Printf(" %s%s %s %s%s%s\n",
			lineNumberSpacer,
			lineNumberStyle().Render(lineNumberStr),
			lineDelimiter,
			lineContentsStyle().Render(lineContents[0:highlightColumnIndex]),
			highlightedSourceStyle().Render(highlight),
			lineContentsStyle().Render(lineContents[highlightColumnIndex+len(highlight):]),
		)

		console.Printf(" %s %s %s%s%s\n",
			noNumberSpaces,
			lineDelimiter,
			strings.Repeat(" ", highlightColumnIndex),
			highlightedSourceStyle().Render("^"),
			highlightedSourceStyle().Render(strings.Repeat("~", highlightLength)),
		)
	}
}
