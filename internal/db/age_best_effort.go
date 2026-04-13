package db

import "log/slog"

// bestEffortAGEDualWriteError logs AGE dual-write errors at warn level and
// returns nil. This prevents AGE availability issues from failing the primary
// write path.
func bestEffortAGEDualWriteError(kind string, err error) error {
	if err == nil {
		return nil
	}
	slog.Warn("db: age dual-write failed; continuing with relational write",
		"kind", kind,
		"error", err,
	)
	return nil
}

// BestEffortAGEDualWriteError is the exported version for use by the compat subpackage.
func BestEffortAGEDualWriteError(kind string, err error) error {
	return bestEffortAGEDualWriteError(kind, err)
}
