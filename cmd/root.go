package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aborroy/alfresco-cli/httpclient"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Format string

const (
	Json    Format = "json"
	Id      Format = "id"
	Table   Format = "table"
	Default Format = "default"
)

var cfgFile string
var OutputParam string
var HttpTimeoutParam time.Duration
var HttpRetriesParam int
var HttpRetryWaitParam time.Duration
var RootCmd = &cobra.Command{
	Use:   "alfresco",
	Short: "A Command Line Interface for Alfresco Content Services",
	Long: `Alfresco CLI provides access to Alfresco REST API services via command line.
A running ACS server is required to use this program (commonly available in http://localhost:8080/alfresco).`,
	Version: "0.0.4",
	PersistentPreRunE: func(command *cobra.Command, args []string) error {
		return httpclient.Configure(HttpTimeoutParam, HttpRetriesParam, HttpRetryWaitParam)
	},
}

var UsernameParam string
var PasswordParam string

type CLIError struct {
	CmdID string
	Cause error
}

func (e *CLIError) Error() string {
	if e == nil {
		return "unknown cli error"
	}
	return "ERROR " + e.CmdID + " " + e.Cause.Error()
}

func Execute() {
	defer func() {
		if recovered := recover(); recovered != nil {
			switch err := recovered.(type) {
			case *CLIError:
				fmt.Fprintln(os.Stderr, err.Error())
				log.Println(err.Error())
				os.Exit(1)
			case error:
				panic(err)
			default:
				panic(recovered)
			}
		}
	}()

	err := RootCmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		log.Println(err.Error())
		os.Exit(1)
	}
}

func ExitWithError(CmdId string, err error) {
	panic(&CLIError{
		CmdID: CmdId,
		Cause: err,
	})
}

func isStdoutTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func normalizeOutputFormat(format string) (string, error) {
	switch strings.ToLower(format) {
	case string(Json):
		return string(Json), nil
	case string(Id):
		return string(Id), nil
	case string(Default):
		return string(Table), nil
	case string(Table):
		return string(Table), nil
	default:
		return "", fmt.Errorf("format '%s' is not an option, allowed values are 'table', 'json' or 'id'", format)
	}
}

func ResolveOutputFormat(command *cobra.Command) (string, error) {
	outputFormat, _ := command.Flags().GetString("format")
	normalizedFormat, err := normalizeOutputFormat(outputFormat)
	if err != nil {
		return "", err
	}
	if command.Flags().Changed("format") || command.Flags().Changed("output") {
		return normalizedFormat, nil
	}
	if !isStdoutTTY() && normalizedFormat == string(Table) {
		return string(Json), nil
	}
	return normalizedFormat, nil
}

func init() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".alfresco")
	}
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		os.WriteFile(".alfresco", nil, 0644)
	}

	RootCmd.PersistentFlags().StringVar(&UsernameParam, "username", "", "Alfresco Username (overrides default stored config value)")
	RootCmd.PersistentFlags().StringVar(&PasswordParam, "password", "", "Alfresco Password for the Username (overrides default stored config value)")
	RootCmd.MarkFlagsRequiredTogether("username", "password")
	RootCmd.PersistentFlags().StringVarP(&OutputParam, "format", "o", "table", "Output format. E.g.: 'table', 'json' or 'id'.")
	RootCmd.PersistentFlags().StringVar(&OutputParam, "output", "table", "Output format alias for backward compatibility. E.g.: 'default', 'table', 'json' or 'id'.")
	RootCmd.PersistentFlags().DurationVar(&HttpTimeoutParam, "http-timeout", 30*time.Second, "HTTP client timeout")
	RootCmd.PersistentFlags().IntVar(&HttpRetriesParam, "http-retries", 2, "Number of retries for retry-safe operations")
	RootCmd.PersistentFlags().DurationVar(&HttpRetryWaitParam, "http-retry-wait", 500*time.Millisecond, "Wait time between retries")
}
