package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/authzed/spicedb/pkg/development"
	core "github.com/authzed/spicedb/pkg/proto/core/v1"
	devinterface "github.com/authzed/spicedb/pkg/proto/developer/v1"
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
	validateCmd := &cobra.Command{
		Use:   "validate <validation_files_or_schema_files>",
		Short: "Validates the given validation files (.yaml, .zaml) or schema files (.zed)",
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
		zed validate https://pastebin.com/8qU45rVK`,
		Args:              commands.ValidationWrapper(cobra.MinimumNArgs(1)),
		ValidArgsFunction: commands.FileExtensionCompletions("zed", "yaml", "yml", "zaml"),
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

	validateCmd.Flags().Bool("force-color", false, "force color code output even in non-tty environments")
	validateCmd.Flags().Bool("fail-on-warn", false, "treat warnings as errors during validation")
	validateCmd.Flags().String("type", "", "the type of the validated file. use this when zed cannot auto-detect the format of the file properly. valid options are \"zed\" and \"yaml\"")
	cmd.AddCommand(validateCmd)
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

// validateCmdFunc returns the string to print to the user, whether to return a non-zero status code, and any errors.
func validateCmdFunc(cmd *cobra.Command, filenames []string) (string, bool, error) {
	// Initialize variables for multiple files
	var (
		totalFiles                 = len(filenames)
		successfullyValidatedFiles = 0
		shouldExit                 = false
		toPrint                    = &strings.Builder{}
		failOnWarn                 = cobrautil.MustGetBool(cmd, "fail-on-warn")
		fileTypeArg                = cobrautil.MustGetString(cmd, "type")
		fileType                   = decode.FileTypeUnknown
	)

	for _, filename := range filenames {
		// Make a guess as to the filetype based on file extension
		ext := filepath.Ext(filename)
		switch ext {
		case ".zed":
			fileType = decode.FileTypeZed
		case ".yml", ".yaml":
			fileType = decode.FileTypeYaml
		}

		// Use the filetype arg if present
		if fileTypeArg != "" {
			switch fileTypeArg {
			case "yaml":
				fileType = decode.FileTypeYaml
			case "zed":
				fileType = decode.FileTypeZed
			default:
				return "", true, fmt.Errorf("invalid value \"%s\" for --type. valid options are \"zed\" and \"yaml\"", fileTypeArg)
			}
		}

		// If we're running over multiple files, print the filename for context/debugging purposes
		if totalFiles > 1 {
			toPrint.WriteString(filename + "\n")
		}

		u, err := url.Parse(filename)
		if err != nil {
			return "", true, err
		}

		d, err := decode.DecoderFromURL(u)
		if err != nil {
			return "", true, err
		}
		validateContents := d.Contents

		// Root the filesystem at the directory containing the schema file, so
		// that relative imports resolve correctly.
		fileDir := filepath.Dir(filename)
		if fileDir == "" {
			fileDir = "."
		}
		filesystem := os.DirFS(fileDir)

		var parsed *validationfile.ValidationFile
		switch fileType {
		case decode.FileTypeYaml:
			parsed, err = d.UnmarshalYAMLValidationFile()
		case decode.FileTypeZed:
			if decode.LooksLikeYAMLValidationFile(string(d.Contents)) {
				fmt.Fprintf(toPrint, "%sfile %q has a .zed extension but appears to be a YAML validation file.\n"+
					"  Rename the file to use a .yaml extension, or use --type yaml to override:\n"+
					"    zed validate %s --type yaml\n\n",
					errorPrefix(), filename, filename,
				)
				shouldExit = true
				continue
			}
			parsed = d.UnmarshalSchemaValidationFile()
		default:
			parsed, err = d.UnmarshalAsYAMLOrSchema()
		}
		// This block handles the error regardless of which case statement is hit
		if err != nil {
			return "", true, err
		}

		// Ensure that either schema or schemaFile is present
		if parsed.Schema.Schema == "" && parsed.SchemaFile == "" {
			return "", false, errors.New("either schema or schemaFile must be present")
		}

		// This logic will use the zero value of the struct, so we don't need
		// to do it conditionally.
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
		}, development.WithSourceFS(filesystem), development.WithRootFileName(filepath.Base(filename)))
		if err != nil {
			return "", false, err
		}
		if devErrs != nil {
			schemaOffset := parsed.Schema.SourcePosition.LineNumber

			// Output errors
			outputDeveloperErrorsWithLineOffset(toPrint, validateContents, devErrs.InputErrors, schemaOffset, filesystem)
			return toPrint.String(), true, nil
		}
		// Run assertions
		adevErrs, aerr := development.RunAllAssertions(devCtx, &parsed.Assertions)
		if aerr != nil {
			return "", false, aerr
		}
		if adevErrs != nil {
			outputDeveloperErrors(toPrint, validateContents, adevErrs, filesystem)
			return toPrint.String(), true, nil
		}
		successfullyValidatedFiles++

		// Run expected relations for file
		_, erDevErrs, rerr := development.RunValidation(devCtx, &parsed.ExpectedRelations)
		if rerr != nil {
			return "", false, rerr
		}
		if erDevErrs != nil {
			outputDeveloperErrors(toPrint, validateContents, erDevErrs, filesystem)
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

func outputForLine(sb *strings.Builder, validateContents []byte, oneIndexedLineNumber uint64, sourceCodeString string, oneIndexedColumnPosition uint64) {
	lines := strings.Split(string(validateContents), "\n")
	intLineNumber, err := safecast.Convert[int](oneIndexedLineNumber)
	if err != nil {
		// It's fine to be zero if the cast fails.
		intLineNumber = 0
	}
	intColumnPosition, err := safecast.Convert[int](oneIndexedColumnPosition)
	if err != nil {
		// It's fine to be zero if the cast fails.
		intColumnPosition = 0
	}
	errorLineNumber := intLineNumber - 1
	for i := errorLineNumber - 3; i < errorLineNumber+3; i++ {
		if i == errorLineNumber {
			renderLine(sb, lines, i, sourceCodeString, errorLineNumber, intColumnPosition-1)
		} else {
			renderLine(sb, lines, i, "", errorLineNumber, -1)
		}
	}
}

func outputDeveloperErrors(sb *strings.Builder, validateContents []byte, devErrors []*devinterface.DeveloperError, sourceFS fs.FS) {
	outputDeveloperErrorsWithLineOffset(sb, validateContents, devErrors, 0, sourceFS)
}

func outputDeveloperErrorsWithLineOffset(sb *strings.Builder, validateContents []byte, devErrors []*devinterface.DeveloperError, lineOffset int, sourceFS fs.FS) {
	for _, devErr := range devErrors {
		// If the error has a Path, read the contents from that file instead.
		if len(devErr.Path) > 0 {
			fileContents, err := fs.ReadFile(sourceFS, devErr.Path[0])
			if err == nil {
				validateContents = fileContents
			}
		}
		lines := strings.Split(string(validateContents), "\n")
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

	lineNumberLength := len(strconv.Itoa(len(lines)))
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

	lineNumberStr := strconv.Itoa(index + 1)
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
