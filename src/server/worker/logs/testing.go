package logs

import (
	"fmt"
	"io"

	"github.com/pachyderm/pachyderm/src/client/pfs"
	"github.com/pachyderm/pachyderm/src/server/worker/common"
)

// MockLogger is an implementation of the TaggedLogger interface for use in
// tests.  Loggers are often passed to callbacks, so you can check that the
// logger has been configured with the right tags in these cases.  In addition,
// you can set the Writer field so that log statements go directly to stdout
// (or some other location) for debugging purposes.
type MockLogger struct {
	// These fields are exposed so that tests can fuck around with them or make assertions
	Writer   io.Writer
	Job      string
	Data     []*common.Input
	UserCode bool
}

// Not used - forces a compile-time error in this file if MockLogger does not
// implement TaggedLogger
var _ TaggedLogger = &MockLogger{}

// NewMockLogger constructs a MockLogger object for use by tests.
func NewMockLogger() *MockLogger {
	return &MockLogger{}
}

// Write fulfills the io.Writer interface for TaggedLogger, and will optionally
// write to the configured ml.Writer, otherwise it pretends that it succeeded.
func (ml *MockLogger) Write(p []byte) (_ int, retErr error) {
	if ml.Writer != nil {
		return ml.Writer.Write(p)
	}
	return len(p), nil
}

// Logf optionally logs a statement using string formatting
func (ml *MockLogger) Logf(formatString string, args ...interface{}) {
	if ml.Writer != nil {
		str := fmt.Sprintf("Logf: "+formatString, args...)
		ml.Writer.Write([]byte(str))
	}
}

// Errf optionally logs an error statement using string formatting
func (ml *MockLogger) Errf(formatString string, args ...interface{}) {
	if ml.Writer != nil {
		str := fmt.Sprintf("Errf: "+formatString, args...)
		ml.Writer.Write([]byte(str))
	}
}

// clone is used by the With* member functions to duplicate the current logger.
func (ml *MockLogger) clone() *MockLogger {
	result := &MockLogger{}
	*result = *ml
	return result
}

// WithJob duplicates the MockLogger and returns a new one tagged with the given
// job ID.
func (ml *MockLogger) WithJob(jobID string) TaggedLogger {
	result := ml.clone()
	result.Job = jobID
	return result
}

// WithData duplicates the MockLogger and returns a new one tagged with the
// given input data.
func (ml *MockLogger) WithData(data []*common.Input) TaggedLogger {
	result := ml.clone()
	result.Data = data
	return result
}

// WithUserCode duplicates the MockLogger and returns a new one tagged to
// indicate that the log statements came from user code.
func (ml *MockLogger) WithUserCode() TaggedLogger {
	result := ml.clone()
	result.UserCode = true
	return result
}

// JobID returns the currently tagged job ID for the logger.  This is redundant
// for MockLogger, as you can access ml.Job directly, but it is needed for the
// TaggedLogger interface.
func (ml *MockLogger) JobID() string {
	return ml.Job
}

// Close is meant to be called to flush logs to object storage and return the
// generated object, but this behavior is not implemented in MockLogger.
func (ml *MockLogger) Close() (*pfs.Object, int64, error) {
	// If you need an actual pfs.Object here, inherit and shadow this function
	return nil, 0, nil
}
