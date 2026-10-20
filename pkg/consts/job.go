package consts

import "fmt"

// JobKind prefixes every async job identifier.
type JobKind string

const (
	JobDeploy    JobKind = "deploy"
	JobProvision JobKind = "provision"
	JobProject   JobKind = "project"
)

// JobID builds the canonical job identifier for a kind and subject.
func JobID(kind JobKind, subject string) string {
	return fmt.Sprintf("%s-%s", kind, subject)
}

// JobIDFromInt builds the canonical job identifier for an integer subject.
func JobIDFromInt(kind JobKind, id int64) string {
	return fmt.Sprintf("%s-%d", kind, id)
}
