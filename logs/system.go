package logs

import (
	"io"
	"os"
	"slices"
	"sort"
	"time"

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

type SystemLogs struct {
	lastId  uint32
	Entries []SystemLogEntry `json:"entries"`
}

// merge appends the contents of a SystemLogEntries value into the current one, updating the first
func (logs *SystemLogs) merge(second *SystemLogs) {

	// simple cases
	if len(logs.Entries) == 0 {
		logs.Entries = second.Entries
		return
	}

	if len(second.Entries) == 0 {
		return
	}

	if logs.Entries[len(logs.Entries)-1].Id > second.Entries[0].Id {
		logs.Entries = append(logs.Entries, second.Entries...)
	}

	// Deal with any potential overlap in log fetching.
	n := sort.Search(len(logs.Entries), func(x int) bool { return logs.Entries[x].Id <= second.Entries[0].Id })
	logs.Entries = append(logs.Entries[0:n], second.Entries...)
}

// Sort returns the SystemLogEntries value in the required order (sorts in place)
func (logs *SystemLogs) Sort(asAsc bool) {

	if asAsc {
		slices.SortFunc(logs.Entries, func(a SystemLogEntry, b SystemLogEntry) int {
			if a.Id > b.Id {
				return 1
			} else if b.Id > a.Id {
				return -1
			} else {
				return 0
			}
		})
	}
}

// filter a log response so that only responses newer than the previous limit are contained.
// Returns whether or not the stopId was found in the response
func (logs *SystemLogs) filter() bool {

	l := logs.lastId

	n := sort.Search(len(logs.Entries), func(x int) bool { return logs.Entries[x].Id <= l })
	if n == len(logs.Entries) {
		return false
	} else {
		logs.Entries = logs.Entries[0:n]
		return true
	}
}

// serializeResults writes the retrieved logs to output either as a JSON array if JSON output is requested
// or as CSV if that was requested
func (logs *SystemLogs) Serialize(out *os.File, asJson bool) error {
	if len(logs.Entries) == 0 {
		return nil
	}
	return serialize(out, logs.Entries, asJson)
}

// SetStopId sets the stopId for the logs, which is used to filter the logs.
func (logs *SystemLogs) SetStopId(stopId any) {
	logs.lastId = stopId.(uint32)
}

// GetStopId returns the current stop id
func (logs *SystemLogs) GetStopId() any {
	if logs.Size() > 0 {
		return logs.Entries[0].Id
	}
	return logs.lastId
}

// Size returns the length of the entries
func (logs *SystemLogs) Size() int {
	return len(logs.Entries)
}

// NewFromBody creates a new SessionLogEntries from the body of a response
func (logs *SystemLogs) FromBody(body io.Reader) (*SystemLogs, error) {
	newLogs := &SystemLogs{}
	err := parseBody(body, newLogs)
	newLogs.lastId = logs.lastId
	return newLogs, err
}

// FetchLogs retrieves the logs from the API and inserts them into  a SessionLogs value.
func (logs *SystemLogs) FetchLogs(config *viper.Viper) error {

	offset := uint32(0)

	for {
		resp, err := makeRequest(config, offset, count)
		if err != nil {
			return err
		}

		defer resp.Body.Close()

		nextResponse, err := logs.FromBody(resp.Body)
		if err != nil {
			return err
		}

		found := nextResponse.filter()
		logs.merge(nextResponse)

		if found || len(nextResponse.Entries) == 0 {
			return nil
		}

		// Increase by less than the offset to handle potential log overlap if new entries
		// added while fetching logs (faster than we get them)
		offset += count
	}
}
