package accumulated

const unknownVersion = "version unknown"

var Version = unknownVersion

func IsVersionKnown() bool {
	return Version != unknownVersion
}
