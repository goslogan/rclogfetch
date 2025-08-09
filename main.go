package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sort"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type SystemLogEntry struct {
	Id          uint32    `json:"id" csv:"id"`
	TimeStamp   time.Time `json:"time" csv:"time"`
	Originator  string    `json:"originator,omitempty" csv:"originator"`
	ApiKeyName  string    `json:"apiKeyName,omitempty" csv:"apiKeyName"`
	Resource    string    `json:"resource,omitempty" csv:"resource"`
	Type        string    `json:"type,omitempty" csv:"type"`
	Description string    `json:"description" csv:"description"`
}

type LogResponse struct {
	Entries []SystemLogEntry `json:"entries"`
}

var config, stateConfig *viper.Viper

const baseURL = "https://api.redislabs.com/v1/%s"

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
	output, err := getOUtput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "unable to open output file %s: %v", config.GetString("output"), err)
		os.Exit(1)
	}

	// finalId will contain the ID of the last log entry fetched, which will be used to update the state file
	results, err := fetchLogs(stateConfig.GetUint32("id"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Error fetching logs: %v\n", err)
		os.Exit(1)
	}

	if len(results) > 0 {
		stateConfig.Set("id", results[0].Id)
		if err := stateConfig.WriteConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "Error writing new final state id (%d): %v\n", results[0].Id, err)
			os.Exit(1)
		}
		if err := serializeResults(output, sortResponse(results)); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "Error serializing logs: %v\n", err)
			os.Exit(1)
		}
	}

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
	if config.IsSet("id") {
		stateConfig.Set("id", config.GetUint32("id"))
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
func getOUtput() (*os.File, error) {

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

// fetchLogs call the Redis Cloud API to fetch the logs, starting from the given offset until a log
// containg stopId is found or the end of the log is reached.
func fetchLogs(stopId uint32) ([]SystemLogEntry, error) {

	var url string
	responses := []SystemLogEntry{}

	if config.GetBool("system") {
		url = fmt.Sprintf(baseURL, "logs")
	} else {
		url = fmt.Sprintf(baseURL, "session-logs")
	}

	offset := uint32(0)
	count := config.GetUint32("count")
	for {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", config.GetString("api-key"))
		req.Header.Set("x-api-secret-key", config.GetString("secret-key"))

		q := req.URL.Query()
		q.Set("offset", fmt.Sprintf("%d", offset))
		q.Set("count", fmt.Sprintf("%d", count))
		req.URL.RawQuery = q.Encode()

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("unauthorized: check your API key and secret key")
		} else if resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("forbidden: check your API key and secret key")
		} else if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code %d from API", resp.StatusCode)
		}

		logResponse, err := parseBody(resp.Body)
		if err != nil {
			return nil, err
		}

		logResponse, found := filterResponse(logResponse, stopId)
		responses = mergeResponses(responses, logResponse)

		if found || len(logResponse) == 0 {
			return responses, nil
		} else {
			offset += count
		}

	}
}

// mergeResponses takes two SystemLogEntry slices and merges them such that no
// duplicate entries are found. It's assumed that the first slice contains higher
// (in the sense of higher Id value) entries than the second.
func mergeResponses(first []SystemLogEntry, second []SystemLogEntry) []SystemLogEntry {

	// simple cases
	if len(first) == 0 {
		return second
	}

	if len(second) == 0 {
		return first
	}

	if first[len(first)-1].Id > second[0].Id {
		return append(first, second...)
	}

	n := sort.Search(len(first), func(x int) bool { return first[x].Id <= second[0].Id })
	return append(first[0:n], second...)
}

// sortResponse ensures the response returned is in the correct order
func sortResponse(response []SystemLogEntry) []SystemLogEntry {

	if viper.GetBool("asc") {
		slices.SortFunc(response, func(a SystemLogEntry, b SystemLogEntry) int {
			if a.Id > b.Id {
				return 1
			} else if b.Id > a.Id {
				return -1
			} else {
				return 0
			}
		})
	}
	return response
}

// parse the body and return any records where the id is greater than the one passed in.
func parseBody(body io.Reader) ([]SystemLogEntry, error) {

	var logResponse LogResponse

	if content, err := io.ReadAll(body); err != nil {
		return nil, err
	} else if err := json.Unmarshal(content, &logResponse); err != nil {
		return nil, err
	} else {
		return logResponse.Entries, nil
	}

}

// filter a log response so that only responses newer than the previous limit are contained.
// Returns whether or not the stopId was found in the response and a new filtered response.
func filterResponse(response []SystemLogEntry, stopId uint32) ([]SystemLogEntry, bool) {

	n := sort.Search(len(response), func(x int) bool { return response[x].Id <= stopId })
	if n == len(response) {
		return response, false
	} else {
		return response[0:n], true
	}

}

// serializeResults writes the retrieved logs to output either as a JSON array if JSON output is requested
// or as CSV if that was requested
func serializeResults(out *os.File, results []SystemLogEntry) error {
	if config.GetBool("json") {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
	} else {
		return gocsv.MarshalFile(results, out) // csv
	}
}

func init() {

	pflag.String("api-key", "", "Redis Cloud API Key")
	pflag.String("secret-key", "", "Redis Cloud Secret Key")

	pflag.Bool("system", true, "fetch the system log")
	pflag.Bool("session", false, "fetch the sesssion log")

	pflag.Bool("append", false, "append to the output file (if not standard output)")
	pflag.String("output", "", "output file (default is standard output)")

	pflag.Uint32("id", 0, "id of the last recored received (used to resume fetching logs)")
	pflag.Uint32("count", 1000, "number of lines to fetch from the log (maximum 1000)")

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
