package runrunc

import (
	"fmt"
	"strings"

	"github.com/kr/logfmt"
	"github.com/pivotal-golang/lager"
)

func forwardRuncLogsToLager(log lager.Logger, buff []byte) {
	parsedLogLine := struct{ Msg string }{}
	for _, logLine := range strings.Split(string(buff), "\n") {
		if err := logfmt.Unmarshal([]byte(logLine), &parsedLogLine); err == nil {
			log.Debug("runc", lager.Data{
				"message": parsedLogLine.Msg,
			})
		}
	}
}

func wrapWithErrorFromRuncLog(log lager.Logger, originalError error, buff []byte) error {
	parsedLogLine := struct{ Msg string }{}
	if err := logfmt.Unmarshal(buff, &parsedLogLine); err != nil {
		return originalError
	}

	if parsedLogLine.Msg == "" {
		return originalError
	}

	return fmt.Errorf("%s: %s", originalError, parsedLogLine.Msg)
}
