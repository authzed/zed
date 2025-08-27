package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ccoveille/go-safecast"
	"github.com/charmbracelet/lipgloss"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	composable "github.com/authzed/spicedb/pkg/composableschemadsl/compiler"
	"github.com/authzed/spicedb/pkg/development"
	core "github.com/authzed/spicedb/pkg/proto/core/v1"
	devinterface "github.com/authzed/spicedb/pkg/proto/developer/v1"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/spiceerrors"
	"github.com/authzed/spicedb/pkg/validationfile"

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
	validateCmd.Flags().Bool("fail-on-warn", false, "treat warnings as errors during validation")
	validateCmd.Flags().String("schema-type", "", "force validation according to specific schema syntax (\"\", \"composable\", \"standard\")")
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
	Args:              commands.ValidationWrapper(cobra.MinimumNArgs(1)),
	ValidArgsFunction: commands.FileExtensionCompletions("zed", "yaml", "zaml"),
	PreRunE:           validatePreRunE,
	RunE: func(cmd *cobra.Command, filenames []string) error {
		result, shouldExit, err := validateCmdFunc(cmd, filenames)
		if err != nil {
			return err
		}
		console.Print(result)
		if shouldExit {
			os.Exit(1)
		}
		return nil
	},

	// A schema that causes the parser/compiler to error will halt execution
	// of this command with an error. In that case, we want to just display the error,
	// rather than showing usage for this command.
	SilenceUsage: true,
}

var validSchemaTypes = []string{"", "standard", "composable"}

func validatePreRunE(cmd *cobra.Command, _ []string) error {
	// Override lipgloss's autodetection of whether it's in a terminal environment
	// and display things in color anyway. This can be nice in CI environments that
	// support it.
	setForceColor := cobrautil.MustGetBool(cmd, "force-color")
	if setForceColor {
		lipgloss.SetColorProfile(termenv.ANSI256)
	}

	schemaType := cobrautil.MustGetString(cmd, "schema-type")
	schemaTypeValid := false
	for _, validType := range validSchemaTypes {
		if schemaType == validType {
			schemaTypeValid = true
		}
	}
	if !schemaTypeValid {
		return fmt.Errorf("schema-type must be one of \"\", \"standard\", \"composable\". received: %s", schemaType)
	}

	return nil
}

// validateCmdFunc returns the string to print to the user, whether to return a non-zero status code, and any errors.
func validateCmdFunc(cmd *cobra.Command, filenames []string) (string, bool, error) {
	// Initialize variables for multiple files
	var (
		totalFiles                 = len(filenames)
		successfullyValidatedFiles = 0
		shouldExit                 = false
		toPrint                    = &strings.Builder{}
		schemaType                 = cobrautil.MustGetString(cmd, "schema-type")
		failOnWarn                 = cobrautil.MustGetBool(cmd, "fail-on-warn")
	)

	for _, filename := range filenames {
		// If we're running over multiple files, print the filename for context/debugging purposes
		if totalFiles > 1 {
			toPrint.WriteString(filename + "\n")
		}

		u, err := url.Parse(filename)
		if err != nil {
			return "", false, err
		}

		decoder, err := decode.DecoderForURL(u)
		if err != nil {
			return "", false, err
		}

		var parsed validationfile.ValidationFile
		validateContents, isOnlySchema, err := decoder(&parsed)
		standardErrors, composableErrs, otherErrs := classifyErrors(err)

		switch schemaType {
		case "standard":
			if standardErrors != nil {
				var errWithSource spiceerrors.WithSourceError
				if errors.As(standardErrors, &errWithSource) {
					outputErrorWithSource(toPrint, validateContents, errWithSource)
					shouldExit = true
				}
				return "", shouldExit, standardErrors
			}
		case "composable":
			if composableErrs != nil {
				var errWithSource spiceerrors.WithSourceError
				if errors.As(composableErrs, &errWithSource) {
					outputErrorWithSource(toPrint, validateContents, errWithSource)
					shouldExit = true
				}
				return "", shouldExit, composableErrs
			}
		default:
			// By default, validate will attempt to validate a schema first according to composable schema rules,
			// then standard schema rules,
			// and if both fail it will show the errors from composable schema.
			if composableErrs != nil && standardErrors != nil {
				var errWithSource spiceerrors.WithSourceError
				if errors.As(composableErrs, &errWithSource) {
					outputErrorWithSource(toPrint, validateContents, errWithSource)
					shouldExit = true
				}
				return "", shouldExit, composableErrs
			}
		}

		if otherErrs != nil {
			return "", false, otherErrs
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
			return "", false, err
		}
		if devErrs != nil {
			schemaOffset := parsed.Schema.SourcePosition.LineNumber
			if isOnlySchema {
				schemaOffset = 0
			}

			// Output errors
			outputDeveloperErrorsWithLineOffset(toPrint, validateContents, devErrs.InputErrors, schemaOffset)
			return toPrint.String(), true, nil
		}
		// Run assertions
		adevErrs, aerr := development.RunAllAssertions(devCtx, &parsed.Assertions)
		if aerr != nil {
			return "", false, aerr
		}
		if adevErrs != nil {
			outputDeveloperErrors(toPrint, validateContents, adevErrs)
			return toPrint.String(), true, nil
		}
		successfullyValidatedFiles++

		// Run expected relations for file
		_, erDevErrs, rerr := development.RunValidation(devCtx, &parsed.ExpectedRelations)
		if rerr != nil {
			return "", false, rerr
		}
		if erDevErrs != nil {
			outputDeveloperErrors(toPrint, validateContents, erDevErrs)
			return toPrint.String(), true, nil
		}
		// Print out any warnings for file
		warnings, err := development.GetWarnings(ctx, devCtx)
		if err != nil {
			return "", false, err
		}

		if len(warnings) > 0 {
			for _, warning := range warnings {
				fmt.Fprintf(toPrint, "%s%s\n", warningPrefix(), warning.Message)
				outputForLine(toPrint, validateContents, uint64(warning.Line), warning.SourceCode, uint64(warning.Column)) // warning.LineNumber is 1-indexed
				toPrint.WriteString("\n")
			}

			toPrint.WriteString(complete())
			// If we have warnings, we use the failOnWarn flag's value
			// to decide whether to exit with an error.
			shouldExit = failOnWarn
		} else {
			toPrint.WriteString(success())
		}
		totalAssertions += len(parsed.Assertions.AssertTrue) + len(parsed.Assertions.AssertFalse)
		totalRelationsValidated += len(parsed.ExpectedRelations.ValidationMap)

		fmt.Fprintf(toPrint, " - %d relationships loaded, %d assertions run, %d expected relations validated\n",
			len(tuples),
			totalAssertions,
			totalRelationsValidated)
	}

	if totalFiles > 1 {
		fmt.Fprintf(toPrint, "total files: %d, successfully validated files: %d\n", totalFiles, successfullyValidatedFiles)
	}

	return toPrint.String(), shouldExit, nil
}

func outputErrorWithSource(sb *strings.Builder, validateContents []byte, errWithSource spiceerrors.WithSourceError) {
	fmt.Fprintf(sb, "%s%s\n", errorPrefix(), errorMessageStyle().Render(errWithSource.Error()))
	outputForLine(sb, validateContents, errWithSource.LineNumber, errWithSource.SourceCodeString, 0) // errWithSource.LineNumber is 1-indexed
}

func outputForLine(sb *strings.Builder, validateContents []byte, oneIndexedLineNumber uint64, sourceCodeString string, oneIndexedColumnPosition uint64) {
	lines := strings.Split(string(validateContents), "\n")
	// These should be fine to be zero if the cast fails.
	intLineNumber, _ := safecast.ToInt(oneIndexedLineNumber)
	intColumnPosition, _ := safecast.ToInt(oneIndexedColumnPosition)
	errorLineNumber := intLineNumber - 1
	for i := errorLineNumber - 3; i < errorLineNumber+3; i++ {
		if i == errorLineNumber {
			renderLine(sb, lines, i, sourceCodeString, errorLineNumber, intColumnPosition-1)
		} else {
			renderLine(sb, lines, i, "", errorLineNumber, -1)
		}
	}
}

func outputDeveloperErrors(sb *strings.Builder, validateContents []byte, devErrors []*devinterface.DeveloperError) {
	outputDeveloperErrorsWithLineOffset(sb, validateContents, devErrors, 0)
}

func outputDeveloperErrorsWithLineOffset(sb *strings.Builder, validateContents []byte, devErrors []*devinterface.DeveloperError, lineOffset int) {
	lines := strings.Split(string(validateContents), "\n")

	for _, devErr := range devErrors {
		outputDeveloperError(sb, devErr, lines, lineOffset)
	}
}

func outputDeveloperError(sb *strings.Builder, devError *devinterface.DeveloperError, lines []string, lineOffset int) {
	lineContext := fmt.Sprintf("parse error in `%s`, line %d, column %d:", devError.Context, devError.Line, devError.Column)
	fmt.Fprintf(sb, "%s%s %s\n", errorPrefix(), lineContext, errorMessageStyle().Render(devError.Message))
	errorLineNumber := int(devError.Line) - 1 + lineOffset // devError.Line is 1-indexed
	for i := errorLineNumber - 3; i < errorLineNumber+3; i++ {
		if i == errorLineNumber {
			renderLine(sb, lines, i, devError.Context, errorLineNumber, -1)
		} else {
			renderLine(sb, lines, i, "", errorLineNumber, -1)
		}
	}

	if devError.CheckResolvedDebugInformation != nil && devError.CheckResolvedDebugInformation.Check != nil {
		fmt.Fprintf(sb, "\n  %s\n", traceStyle().Render("Explanation:"))
		tp := printers.NewTreePrinter()
		printers.DisplayCheckTrace(devError.CheckResolvedDebugInformation.Check, tp, true)
		sb.WriteString(tp.Indented())
	}

	sb.WriteString("\n\n")
}

func renderLine(sb *strings.Builder, lines []string, index int, highlight string, highlightLineIndex int, highlightStartingColumnIndex int) {
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
		fmt.Fprintf(sb, " %s%s %s %s\n", lineNumberSpacer, lineNumberStyle().Render(lineNumberStr), lineDelimiter, lineContentsStyle().Render(lineContents))
	} else {
		fmt.Fprintf(sb, " %s%s %s %s%s%s\n",
			lineNumberSpacer,
			lineNumberStyle().Render(lineNumberStr),
			lineDelimiter,
			lineContentsStyle().Render(lineContents[0:highlightColumnIndex]),
			highlightedSourceStyle().Render(highlight),
			lineContentsStyle().Render(lineContents[highlightColumnIndex+len(highlight):]))

		fmt.Fprintf(sb, " %s %s %s%s%s\n",
			noNumberSpaces,
			lineDelimiter,
			strings.Repeat(" ", highlightColumnIndex),
			highlightedSourceStyle().Render("^"),
			highlightedSourceStyle().Render(strings.Repeat("~", highlightLength)))
	}
}

// classifyErrors returns errors from the composable DSL, the standard DSL, and any other parsing errors.
func classifyErrors(err error) (error, error, error) {
	if err == nil {
		return nil, nil, nil
	}
	var standardErr compiler.BaseCompilerError
	var composableErr composable.BaseCompilerError
	var retStandard, retComposable, allOthers error

	ok := errors.As(err, &standardErr)
	if ok {
		retStandard = standardErr
	}
	ok = errors.As(err, &composableErr)
	if ok {
		retComposable = composableErr
	}

	if retStandard == nil && retComposable == nil {
		allOthers = err
	}

	return retStandard, retComposable, allOthers
}
