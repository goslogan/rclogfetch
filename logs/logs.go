package logs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gocarina/gocsv"
	"github.com/spf13/viper"
)

const baseURL = "https://api.redislabs.com/v1/%s"
const count = 100

type LogHandler interface {
	Sort(asAsc bool)
	Serialize(out *os.File, asJson bool) error
	SetStopId(stopId any)
	FetchLogs(config *viper.Viper) error
	GetStopId() any
	Size() int
}

// Parse into a LogResponse and return the entries of the response.
func parseBody(body io.Reader, results LogHandler) error {

	if content, err := io.ReadAll(body); err != nil {
		return err
	} else if err := json.Unmarshal(content, results); err != nil {
		return err
	} else {
		return nil
	}
}

func serialize(out *os.File, entries any, asJson bool) error {

	if asJson {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(entries)
	} else {
		return gocsv.MarshalFile(entries, out) // csv
	}
}

func makeRequest(config *viper.Viper, offset uint32, count uint32) (*http.Response, error) {

	url := ""

	if config.GetBool("system") {
		url = fmt.Sprintf(baseURL, "logs")
	} else {
		url = fmt.Sprintf(baseURL, "session-logs")
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", config.GetString("api-key"))
	req.Header.Set("x-api-secret-key", config.GetString("secret-key"))

	q := req.URL.Query()
	q.Set("offset", fmt.Sprintf("%d", offset))
	q.Set("limit", fmt.Sprintf("%d", count))
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)

	if err == nil && resp.StatusCode == http.StatusOK {
		return resp, nil
	} else {
		defer resp.Body.Close() // Close the body if we got a response
		if err == nil {
			if resp.StatusCode == http.StatusUnauthorized {
				err = fmt.Errorf("unauthorized: check your API key and secret key")
			} else if resp.StatusCode == http.StatusForbidden {
				err = fmt.Errorf("forbidden: check your API key and secret key")
			} else if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("unexpected status code %d from API", resp.StatusCode)
			}
		}

		return resp, err
	}

}
