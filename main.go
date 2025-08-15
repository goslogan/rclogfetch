package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/goslogan/rclogfetch/logs"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var config, stateConfig *viper.Viper

func main() {

	// parse command line flags
	pflag.Parse()
	if err := validateFlags(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		pflag.Usage()
		os.Exit(1)
	}

	// load the state file into the config if found (not finding it is not an error)
	if err := loadState(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Unable to load state: %v\n", err)
		os.Exit(1)
	}

	// Get the output writer
	output, err := getOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "unable to open output file %s: %v", config.GetString("output"), err)
		os.Exit(1)
	}

	var handler logs.LogHandler

	if config.GetBool("system") {
		handler = &logs.SystemLogs{}
		handler.SetStopId(stateConfig.GetUint32("system"))
	} else {
		handler = &logs.SessionLogs{}
		handler.SetStopId(stateConfig.GetString("session"))
	}

	err = handler.FetchLogs(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// need to call before sorting.
	finalId := handler.GetStopId()

	handler.Sort(config.GetBool("asc"))
	err = handler.Serialize(output, config.GetBool("json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Error serializing logs: %v\n", err)
		os.Exit(1)
	}

	if handler.Size() > 0 {
		err = saveState(finalId)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "Error saving state: %v\n", err)
			os.Exit(1)
		}
	}

}

// saveState writes the state file back again after a succesful log fetch
func saveState(finalId any) error {
	if config.GetBool("system") {
		stateConfig.Set("system", finalId)
	} else {
		stateConfig.Set("session", finalId)
	}

	return stateConfig.WriteConfig()
}

// loadState loads the state file into the viper config if found, checking the sanity of the
// values found in the file. If not found, the defaults are used (or those values passed on the command line).
func loadState() error {

	stateFileName := config.GetString("statefile")
	stateConfig = viper.New()
	stateConfig.SetConfigType("yaml")
	stateConfig.SetConfigFile(stateFileName)

	if err := stateConfig.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		} else if os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	}

	// If overridden on the command line, update here
	if config.IsSet("last-id") {
		if config.GetBool("system") {
			val, err := strconv.ParseUint(config.GetString("last-id"), 10, 32)
			if err != nil {
				return fmt.Errorf("invalid last-id value %s: %v", config.GetString("last-id"), err)
			}
			stateConfig.Set("system", val)
		} else {
			stateConfig.Set("session", config.GetString("last-id"))
		}
	}

	return nil
}

// validateFlags makes sure the flags are sane and usable
func validateFlags() error {

	if config.GetString("api-key") == "" {
		return errors.New("API Key is required")
	}
	if config.GetString("secret-key") == "" {
		return errors.New("secret key is required")
	}
	if config.GetUint16("count") > 1000 {
		return fmt.Errorf("count cannot be greater than 1000, got %d", config.GetUint16("count"))
	}
	if config.GetUint16("count") == 0 {
		fmt.Fprintf(os.Stderr, "Count is set to 0, changing to 100\n")
		config.Set("count", 100)
	}

	if !(config.GetBool("csv") || config.GetBool("json")) {
		return errors.New("either csv or json output must be specified")
	}
	if !(config.GetBool("system") || config.GetBool("session")) {
		return errors.New("either system or session log must be specified")
	}

	if !config.GetBool("asc") && !config.GetBool("desc") {
		return errors.New("either asc or desc must be specified")
	}

	return nil
}

// getOutput returns the io.writer that the output should be written to.
// By default this is stdout. If another file is specified, it will be created/truncated unless
// the --append flag has been set.
func getOutput() (*os.File, error) {

	if config.GetString("output") == "" {
		return os.Stdout, nil
	}

	var output *os.File
	var err error

	if config.GetBool("append") {
		output, err = os.OpenFile(config.GetString("output"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	} else {
		output, err = os.Create(config.GetString("output"))
	}

	return output, err
}

func init() {

	pflag.String("api-key", "", "Redis Cloud API Key")
	pflag.String("secret-key", "", "Redis Cloud Secret Key")

	pflag.Bool("system", true, "fetch the system log")
	pflag.Bool("session", false, "fetch the sesssion log")

	pflag.Bool("append", false, "append to the output file (if not standard output)")
	pflag.String("output", "", "output file (default is standard output)")

	pflag.String("last-id", "", "id of the last recored received (used to resume fetching logs)")

	pflag.String("statefile", ".rc-log-fetch-state.yaml", "state file to store the last fetched log line id")

	pflag.Bool("asc", true, "sort the log in ascending order")
	pflag.Bool("desc", false, "sort the log in descending order")

	pflag.Bool("csv", false, "output in CSV format")
	pflag.Bool("json", true, "output in JSON format")

	config = viper.New()

	config.BindPFlags(pflag.CommandLine)
	config.SetEnvPrefix("RCLOGFETCH")
	config.AutomaticEnv()
}
