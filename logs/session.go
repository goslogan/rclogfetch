package logs

import (
	"io"
	"os"
	"slices"
	"time"

	"github.com/spf13/viper"
)

type SessionLogEntry struct {
	Id        string    `json:"id" csv:"id"`
	TimeStamp time.Time `json:"time" csv:"time"`
	User      string    `json:"user" csv:"user"`
	UserAgent string    `json:"userAgent" csv:"userAgent"`
	IpAddress string    `json:"ipAddress" csv:"ipAddress"`
	UserRole  string    `json:"userRole" csv:"userRole"`
	Type      string    `json:"type" csv:"type"`
	Action    string    `json:"action" csv:"action"`
}

type SessionLogs struct {
	lastId  string            // lastId is the last id that was fetched, used to filter the logs
	Entries []SessionLogEntry `json:"entries"`
}

// merge appends the contents of a SessionLogEntries value into the current one, returning
// e new value with the content of both. This identifies entries that overlap by looking for a
// matching Id field. See filter for why we can't do a binary search here
func (logs *SessionLogs) merge(second *SessionLogs) {

	// simple cases
	if len(logs.Entries) == 0 {
		logs.Entries = second.Entries
		return
	}

	if len(second.Entries) == 0 {
		return
	}

	for n := 0; n < len(logs.Entries); n++ {
		if logs.Entries[n].Id == second.Entries[0].Id {
			logs.Entries = append(logs.Entries[0:n], second.Entries...)
			return
		}
	}
}

// Sort returns the SystemLogEntries value in the required order (sorts in place by time)
func (logs *SessionLogs) Sort(asAsc bool) {

	if asAsc {
		slices.SortFunc(logs.Entries, func(a SessionLogEntry, b SessionLogEntry) int {
			if a.TimeStamp.After(b.TimeStamp) {
				return 1
			} else if b.TimeStamp.After(a.TimeStamp) {
				return -1
			} else {
				return 0
			}
		})
	}
}

// filter a log response so that only responses newer than the previous limit are contained.
// Remember -logs are returned in time order, reversed. We can just walk through and look
// for the id. But we can't use a binary search in the session log because we are looking for
// a GUID.
// Returns whether or not the stopId was found in the response
func (logs *SessionLogs) filter() bool {

	for n := 0; n < len(logs.Entries); n++ {
		if logs.Entries[n].Id == logs.lastId {
			logs.Entries = logs.Entries[0:n]
			return true
		}
	}

	return false
}

// serializeResults writes the retrieved logs to output either as a JSON array if JSON output is requested
// or as CSV if that was requested
func (logs *SessionLogs) Serialize(out *os.File, asJson bool) error {
	if len(logs.Entries) == 0 {
		return nil
	}
	return serialize(out, logs.Entries, asJson)
}

// NewFromBody creates a new SessionLogEntries from the body of a response
func (logs *SessionLogs) FromBody(body io.Reader) (*SessionLogs, error) {
	newLogs := &SessionLogs{}
	err := parseBody(body, newLogs)
	newLogs.lastId = logs.lastId

	return newLogs, err
}

// SetStopId sets the stopId for the logs, which is used to filter the logs.
func (logs *SessionLogs) SetStopId(stopId any) {
	logs.lastId = stopId.(string)
}

// GetStopId returns the current stop id
func (logs *SessionLogs) GetStopId() any {
	if logs.Size() > 0 {
		return logs.Entries[0].Id
	}
	return logs.lastId
}

// Size returns the length of the entries
func (logs *SessionLogs) Size() int {
	return len(logs.Entries)
}

// FetchLogs retrieves the logs from the API and inserts them into  a SessionLogs value.
func (logs *SessionLogs) FetchLogs(config *viper.Viper) error {

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

		offset += count

	}
}
